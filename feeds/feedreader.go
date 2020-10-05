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
		options *types.FeedOptions
	}
)

func NewFeedReader(options *types.FeedOptions) *FeedReader {
	return &FeedReader{options}
}

func (reader *FeedReader) Run(ch chan<- types.Message) {
	for {
		items, err := reader.getFeedItems()
		if err != nil {
			time.Sleep(reader.options.CheckInterval)
			continue
		}

		for _, item := range items {
			for chatID := range reader.options.Chats {
				ch <- types.Message{
					ChatID: chatID,
					URL:    item.Link,
				}
			}
		}

		if len(reader.options.Chats) == 0 {
			log.Infof("No more subscribed chats for %s. Update finished", reader.options.FeedUrl)
			return
		}

		time.Sleep(reader.options.CheckInterval)
	}
}

func (reader *FeedReader) CheckFeed() error {
	_, err := reader.getFeedItems()
	return err
}

func (reader *FeedReader) getFeedItems() ([]*gofeed.Item, error) {
	log.Printf("Reading feed %s", reader.options.FeedUrl)
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(reader.options.FeedUrl)
	if err != nil {
		log.Printf("Failed to update feed at %s", reader.options.FeedUrl)
		return nil, err
	}

	items := feed.Items[:]
	sort.Slice(items, func(i, j int) bool {
		return items[i].Published < items[j].Published
	})

	return items, nil
}
