package feeds

import (
	"github.com/mmcdole/gofeed"
	log "github.com/sirupsen/logrus"
	"sort"
	"tgRssBot/types"
	"time"
)

type (
	FeedReader struct {
		Options   *types.FeedOptions
		IsRunning bool
		items     []string
	}
)

func NewFeedReader(options *types.FeedOptions) *FeedReader {
	return &FeedReader{Options: options}
}

func (reader *FeedReader) Run(ch chan<- types.Message) {
	reader.IsRunning = true
	for {
		if len(reader.Options.Chats) == 0 {
			log.Infof("No more subscribed chats for %s. Update finished", reader.Options.FeedUrl)
			break
		}

		feedItems, err := reader.getFeedItems()
		if err != nil {
			time.Sleep(reader.Options.CheckInterval)
			continue
		}

		var items []string
		for _, item := range feedItems {
			for chatID := range reader.Options.Chats {
				ch <- types.Message{
					ChatID: chatID,
					URL:    item.Link,
				}
			}
			items = append(items, item.Link)
		}
		reader.items = items

		time.Sleep(reader.Options.CheckInterval)
	}
	reader.IsRunning = false
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
	feed, err := parser.ParseURL(reader.Options.FeedUrl)
	if err != nil {
		log.Printf("Failed to update feed at %s, %s", reader.Options.FeedUrl, err)
		return nil, err
	}

	items := feed.Items[:]
	sort.Slice(items, func(i, j int) bool {
		return items[i].Published < items[j].Published
	})

	return items, nil
}
