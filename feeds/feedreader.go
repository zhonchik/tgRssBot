package feeds

import (
	"context"
	"github.com/mmcdole/gofeed"
	log "github.com/sirupsen/logrus"
	"sort"
	"strings"
	"sync"
	"tgRssBot/types"
	"time"
)

type (
	FeedReader struct {
		Options   *types.FeedOptions
		Mu        sync.Mutex
		IsRunning bool
		items     []string
	}
)

func NewFeedReader(options *types.FeedOptions) *FeedReader {
	return &FeedReader{Options: options}
}

func (reader *FeedReader) Run(ch chan<- types.Message) {
	last := make(map[int64]time.Time)
	for {
		reader.Mu.Lock()
		if len(reader.Options.Chats) == 0 {
			reader.IsRunning = false
			reader.Mu.Unlock()
			return
		}
		reader.Mu.Unlock()
		items, err := reader.getFeedItems()
		if err == nil {
			var messages []types.Message
			reader.Mu.Lock()
			reader.items = nil
			for _, item := range items {
				reader.items = append(reader.items, item.Link)
			}
			for id := range reader.Options.Chats {
				s := reader.Options.ForChat(id)
				if time.Since(last[id]) < time.Duration(s.Minutes)*time.Minute {
					continue
				}
				last[id] = time.Now()
				var batch []types.Message
				for _, item := range items {
					m := articleMessage(item)
					if !strings.Contains(strings.ToLower(m.Title+" "+m.Text), strings.ToLower(s.Keyword)) {
						continue
					}
					m.ChatID = id
					m.FeedURL = reader.Options.FeedUrl
					batch = append(batch, m)
				}
				if s.MaxItems > 0 && len(batch) > s.MaxItems {
					batch = batch[len(batch)-s.MaxItems:]
				}
				messages = append(messages, batch...)
			}
			reader.Mu.Unlock()
			for _, m := range messages {
				ch <- m
			}
		}
		time.Sleep(time.Minute)
	}
}

func (reader *FeedReader) GetItems() ([]string, error) {
	if reader.items != nil {
		return reader.items, nil
	}
	feedItems, err := reader.getFeedItems()
	if err != nil {
		return nil, err
	}

	var items []string
	for _, item := range feedItems {
		items = append(items, item.Link)
	}
	return items, nil
}

func (reader *FeedReader) getFeedItems() ([]*gofeed.Item, error) {
	log.Printf("Reading feed %s", reader.Options.FeedUrl)
	parser := gofeed.NewParser()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	feed, err := parser.ParseURLWithContext(reader.Options.FeedUrl, ctx)
	if err != nil {
		log.Printf("Failed to update feed at %s, %s", reader.Options.FeedUrl, err)
		return nil, err
	}

	items := feed.Items[:]
	sort.SliceStable(items, func(i, j int) bool {
		return publicationTime(items[i]).Before(publicationTime(items[j]))
	})

	return items, nil
}

// Use parsed instants, including timezone offsets, before applying item limits.
// Undated items sort first, preserving their relative order.
func publicationTime(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed
	}
	return time.Time{}
}
