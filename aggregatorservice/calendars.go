package aggregatorservice

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/purell"
	ical "github.com/arran4/golang-ical"
	log "github.com/sirupsen/logrus"
	"tgRssBot/storage"
	"tgRssBot/types"
)

func calendarURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "webcal://") {
		raw = "https://" + strings.TrimPrefix(raw, "webcal://")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", errors.New("enter an HTTP(S) or webcal calendar URL")
	}
	return u.String(), nil
}

func parseCalendar(r io.Reader) (types.Calendar, error) {
	c, err := ical.ParseCalendar(r)
	result := types.Calendar{Updated: time.Now().UTC()}
	if err != nil {
		return result, err
	}
	for _, e := range c.Events() {
		if status := e.GetProperty(ical.ComponentPropertyStatus); status != nil && status.Value == "CANCELLED" {
			continue
		}
		// Explicitly reject recurrence rather than silently missing future pauses.
		if e.GetProperty(ical.ComponentPropertyRrule) != nil {
			return result, errors.New("recurring events are not supported; use an expanded calendar feed")
		}
		start, err := e.GetStartAt()
		if err != nil {
			return result, fmt.Errorf("invalid event start: %w", err)
		}
		result.Starts = append(result.Starts, start.UTC())
	}
	sort.Slice(result.Starts, func(i, j int) bool { return result.Starts[i].Before(result.Starts[j]) })
	return result, nil
}

func fetchCalendar(link string) (types.Calendar, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(link)
	if err != nil {
		return types.Calendar{}, errors.New("calendar download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return types.Calendar{}, fmt.Errorf("calendar returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return types.Calendar{}, err
	}
	if len(data) > 4*1024*1024 {
		return types.Calendar{}, errors.New("calendar exceeds 4 MiB")
	}
	return parseCalendar(strings.NewReader(string(data)))
}

// CalendarCommand exposes chat-owned calendars and subscription controls.
func (as *AggregatorService) CalendarCommand(id int64, args []string) (string, error) {
	usage := "Usage: /calendar list | add/delete/resume/advance <calendar URL> | link <feed URL> <calendar URL> | unlink/next <feed URL>"
	if len(args) == 0 {
		return usage, nil
	}
	action := args[0]
	if (action == "unlink" || action == "next") && len(args) != 2 {
		return usage, nil
	}
	if action == "list" && len(args) == 1 {
		as.mu.Lock()
		defer as.mu.Unlock()
		chat := as.chats[id]
		if chat == nil {
			return "", errors.New("unknown chat")
		}
		var links []string
		for link, c := range chat.Calendars {
			links = append(links, fmt.Sprintf("%s (%d events; refreshed %s)", link, len(c.Starts), c.Updated.Format(time.RFC3339)))
		}
		sort.Strings(links)
		if len(links) == 0 {
			return "No calendars", nil
		}
		return strings.Join(links, "\n"), nil
	}
	if len(args) < 2 {
		return usage, nil
	}
	var downloaded types.Calendar
	var err error
	link := ""
	if action == "link" {
		if len(args) != 3 {
			return usage, nil
		}
		link, err = calendarURL(args[2])
	} else if action != "unlink" && action != "next" {
		if len(args) != 2 {
			return usage, nil
		}
		link, err = calendarURL(args[1])
	}
	if err != nil {
		return "", err
	}
	if action == "add" {
		downloaded, err = fetchCalendar(link)
		if err != nil {
			return "", err
		}
	}
	as.mu.Lock()
	defer as.mu.Unlock()
	chat := as.chats[id]
	if chat == nil {
		return "", errors.New("unknown chat")
	}
	if action == "add" || action == "delete" {
		calendars := make(map[string]types.Calendar)
		for k, v := range chat.Calendars {
			calendars[k] = v
		}
		if action == "add" {
			calendars[link] = downloaded
		} else {
			if _, ok := calendars[link]; !ok {
				return "", errors.New("calendar missing")
			}
			for feed, r := range as.feeds {
				if r.Options.Chats[id] {
					s := r.Options.ForChat(id)
					if s.Calendar == link {
						s.Calendar = ""
						s.CalendarAfter = time.Time{}
						if err := as.updateLocked(id, feed, s); err != nil {
							return "", err
						}
					}
				}
			}
			delete(calendars, link)
		}
		updated := *chat
		updated.Calendars = calendars
		if err := as.storage.SaveChat(id, &updated); err != nil {
			return "", err
		}
		as.chats[id] = &updated
		as.signalDelivery()
		return "Calendar updated", nil
	}
	if action == "link" || action == "unlink" || action == "next" {
		feed, err := purell.NormalizeURLString(args[1], purell.FlagsUsuallySafeGreedy)
		if err != nil {
			return "", err
		}
		r := as.feeds[feed]
		if r == nil || !r.Options.Chats[id] {
			return "", errors.New("subscription missing")
		}
		s := r.Options.ForChat(id)
		switch action {
		case "link":
			if _, ok := chat.Calendars[link]; !ok {
				return "", errors.New("add the calendar first")
			}
			s.Calendar = link
			s.CalendarAfter = time.Now().UTC()
		case "unlink":
			s.Calendar = ""
			s.CalendarAfter = time.Time{}
		case "next":
			if err := advance(chat, &s); err != nil {
				return "", err
			}
		}
		return "Subscription updated", as.updateLocked(id, feed, s)
	}
	if action == "resume" || action == "advance" {
		if _, ok := chat.Calendars[link]; !ok {
			return "", errors.New("calendar missing")
		}
		updates := map[string]types.SubscriptionSettings{}
		for feed, r := range as.feeds {
			if r.Options.Chats[id] {
				s := r.Options.ForChat(id)
				if s.Calendar == link {
					if action == "resume" {
						s.Paused = false
						s.CalendarAfter = time.Now().UTC()
					} else if err := advance(chat, &s); err != nil {
						return "", err
					}
					updates[feed] = s
				}
			}
		}
		for feed, s := range updates {
			if err := as.updateLocked(id, feed, s); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("Updated %d subscriptions", len(updates)), nil
	}
	return usage, nil
}

func nextStart(c types.Calendar, after time.Time) time.Time {
	for _, t := range c.Starts {
		if t.After(after) {
			return t
		}
	}
	return time.Time{}
}
func advance(chat *types.Chat, s *types.SubscriptionSettings) error {
	c, ok := chat.Calendars[s.Calendar]
	if !ok {
		return errors.New("subscription has no calendar")
	}
	next := nextStart(c, s.CalendarAfter)
	if next.IsZero() {
		return errors.New("no next calendar event")
	}
	s.CalendarAfter = next
	s.Paused = false
	return nil
}
func (as *AggregatorService) calendarBlocks(sub storage.Subscription, m types.Message) bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.calendarBlocksLocked(sub, m)
}
func (as *AggregatorService) calendarBlocksLocked(sub storage.Subscription, m types.Message) bool {
	r := as.feeds[sub.Feed]
	if r == nil {
		return true
	}
	s := r.Options.ForChat(sub.Chat)
	if s.Calendar == "" {
		return false
	}
	chat := as.chats[sub.Chat]
	if chat == nil {
		return true
	}
	c, ok := chat.Calendars[s.Calendar]
	if !ok {
		return true
	}
	boundary := nextStart(c, s.CalendarAfter)
	if boundary.IsZero() {
		return false
	}
	// Old queue records have no timestamp: fail closed once the boundary is reached.
	return !time.Now().Before(boundary) && (m.ReceivedAt.IsZero() || !m.ReceivedAt.Before(boundary))
}
func (as *AggregatorService) refreshCalendars() {
	defer as.workers.Done()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		as.mu.Lock()
		links := map[string]bool{}
		for _, chat := range as.chats {
			for link := range chat.Calendars {
				links[link] = true
			}
		}
		as.mu.Unlock()
		for link := range links {
			select {
			case <-as.done:
				return
			default:
			}
			c, err := fetchCalendar(link)
			if err != nil {
				log.Warn("Calendar refresh failed: ", err)
				continue
			}
			as.mu.Lock()
			for id, chat := range as.chats {
				if _, ok := chat.Calendars[link]; ok {
					old := chat.Calendars[link]
					chat.Calendars[link] = c
					if err := as.storage.SaveChat(id, chat); err != nil {
						chat.Calendars[link] = old
						log.Warn(err)
					}
				}
			}
			as.mu.Unlock()
		}
		as.signalDelivery()
		select {
		case <-as.done:
			return
		case <-ticker.C:
		}
	}
}
