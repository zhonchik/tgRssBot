package aggregatorservice

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"tgRssBot/feeds"
	"tgRssBot/storage"
	"tgRssBot/types"
	"time"
)

func TestCalendarParsing(t *testing.T) {
	c, err := parseCalendar(strings.NewReader("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:a\r\nDTSTART;TZID=Europe/London:20260705T150000\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:b\r\nDTSTART;VALUE=DATE:20260101\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
	if err != nil || len(c.Starts) != 2 {
		t.Fatal(c, err)
	}
	if c.Starts[1].Format(time.RFC3339) != "2026-07-05T14:00:00Z" {
		t.Fatal(c.Starts)
	}
	if _, err := parseCalendar(strings.NewReader("not a calendar")); err == nil {
		t.Fatal("accepted invalid calendar")
	}
}

func TestCalendarControlsAndPersistence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"))
	}))
	defer server.Close()
	db, err := storage.NewStorage(storage.Options{Path: filepath.Join(t.TempDir(), "db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := &AggregatorService{storage: db, chats: map[int64]*types.Chat{1: {}, 2: {}}, feeds: map[string]*feeds.FeedReader{}, wake: make(chan struct{}, 1)}
	for _, feed := range []string{"https://example.com/a", "https://example.com/b"} {
		as.feeds[feed] = feeds.NewFeedReader(&types.FeedOptions{FeedUrl: feed, Chats: map[int64]bool{1: true, 2: true}})
	}
	command := func(id int64, args ...string) {
		t.Helper()
		if _, err := as.CalendarCommand(id, args); err != nil {
			t.Fatal(err)
		}
	}
	command(1, "add", server.URL)
	for feed := range as.feeds {
		command(1, "link", feed, server.URL)
	}
	now := time.Now().UTC()
	a := now.Add(-2 * time.Hour)
	b := now.Add(-time.Hour)
	c := now.Add(time.Hour)
	as.chats[1].Calendars[server.URL] = types.Calendar{Starts: []time.Time{a, b, c}}
	for feed, r := range as.feeds {
		s := r.Options.ForChat(1)
		s.CalendarAfter = a.Add(-time.Second)
		if err := as.Update(1, feed, s); err != nil {
			t.Fatal(err)
		}
		r.Options.Settings[1] = s
	}
	sub := storage.Subscription{Feed: "https://example.com/a", Chat: 1}
	msg := types.Message{ReceivedAt: a.Add(time.Minute)}
	if !as.calendarBlocks(sub, msg) {
		t.Fatal("did not pause at first event")
	}
	command(1, "next", sub.Feed)
	if as.calendarBlocks(sub, msg) {
		t.Fatal("advance did not release earlier item")
	}
	msg.ReceivedAt = b
	if !as.calendarBlocks(sub, msg) {
		t.Fatal("boundary article leaked")
	}
	command(1, "resume", server.URL)
	if as.calendarBlocks(sub, msg) {
		t.Fatal("resume did not release backlog")
	}
	if as.feeds[sub.Feed].Options.ForChat(2).Calendar != "" {
		t.Fatal("changed another user")
	}
	saved, err := db.GetFeeds()
	if err != nil || len(saved) != 2 {
		t.Fatal(saved, err)
	}
	chats, err := db.GetChats()
	if err != nil || len(chats[1].Calendars) != 1 {
		t.Fatal(chats, err)
	}
	if _, err := as.CalendarCommand(2, []string{"link", sub.Feed, server.URL}); err == nil {
		t.Fatal("cross-user calendar access")
	}
	command(1, "delete", server.URL)
	for _, r := range as.feeds {
		if r.Options.ForChat(1).Calendar != "" {
			t.Fatal("dangling link")
		}
	}
}
