package aggregatorservice

import (
	"errors"
	"github.com/PuerkitoBio/purell"
	log "github.com/sirupsen/logrus"
	"strings"
	"tgRssBot/feeds"
	"tgRssBot/storage"
	"tgRssBot/types"
	"time"
)

type (
	AggregatorService struct {
		sendMessage  func(chatID int64, message string) error
		storage      *storage.Storage
		chats        map[int64]*types.Chat
		feeds        map[string]*feeds.FeedReader
		messageQueue chan types.Message
	}
)

func NewAggregatorService(
	messageSender func(chatID int64, message string) error,
	st *storage.Storage,
) (*AggregatorService, error) {
	messageQueue := make(chan types.Message)

	as := &AggregatorService{
		sendMessage:  messageSender,
		storage:      st,
		chats:        make(map[int64]*types.Chat),
		feeds:        make(map[string]*feeds.FeedReader),
		messageQueue: messageQueue,
	}

	feedList, err := st.GetFeeds()
	if err != nil {
		return nil, err
	}
	for _, feed := range feedList {
		feedUrl, err := purell.NormalizeURLString(feed.FeedUrl, purell.FlagsUsuallySafeGreedy)
		if err != nil {
			continue
		}
		feed.FeedUrl = feedUrl
		as.addReader(feed)
	}
	as.chats, err = st.GetChats()
	if err != nil {
		log.Warnf("Failed to read chats: %s", err)
	}

	go as.processMessages()

	return as, nil
}

func (as *AggregatorService) Close() error {
	log.Info("Closing service")
	return as.storage.Close()
}

func (as *AggregatorService) Start(chatID int64) (string, error) {
	chat := as.chats[chatID]
	if chat != nil {
		return "", errors.New("i know you")
	}
	chat = &types.Chat{}
	as.chats[chatID] = chat
	err := as.storage.SaveChat(chatID, chat)
	if err != nil {
		return "", err
	}
	return "you're welcome", nil
}

func (as *AggregatorService) TrySubscribe(chatID int64, feedUrl string) error {
	err := as.checkChat(chatID)
	if err != nil {
		return err
	}
	feedUrl, err = purell.NormalizeURLString(feedUrl, purell.FlagsUsuallySafeGreedy)
	if err != nil {
		return errors.New("wrong url")
	}
	reader := as.feeds[feedUrl]
	if reader == nil {
		feedOptions := &types.FeedOptions{
			FeedUrl:       feedUrl,
			Chats:         map[int64]bool{chatID: true},
			CheckInterval: time.Minute,
		}
		reader = feeds.NewFeedReader(feedOptions)
		as.feeds[feedOptions.FeedUrl] = reader
	}

	items, err := reader.GetItems()
	if err == nil {
		for _, item := range items {
			message := types.Message{ChatID: chatID, URL: item}
			_ = as.storage.MarkAsProcessed(message)
		}
	} else {
		return errors.New("unsupported feed format")
	}

	reader.Options.Chats[chatID] = true
	err = as.storage.SaveFeed(reader.Options)
	if err != nil {
		return err
	}
	if !reader.IsRunning {
		go reader.Run(as.messageQueue)
	}
	return nil
}

func (as *AggregatorService) TryUnsubscribe(chatID int64, feedUrl string) error {
	err := as.checkChat(chatID)
	if err != nil {
		return err
	}
	feedUrl, err = purell.NormalizeURLString(feedUrl, purell.FlagsUsuallySafeGreedy)
	if err != nil {
		return errors.New("wrong url")
	}
	reader := as.feeds[feedUrl]
	if reader == nil || !reader.Options.Chats[chatID] {
		return errors.New("was not subscribed")
	}
	delete(reader.Options.Chats, chatID)
	err = as.storage.SaveFeed(reader.Options)
	if err != nil {
		return err
	}
	return nil
}

func (as *AggregatorService) GetFeedList(chatID int64) (string, error) {
	err := as.checkChat(chatID)
	if err != nil {
		return "", err
	}
	var lines []string
	var subscribed []string
	var suggestions []string

	for feedUrl, reader := range as.feeds {
		if reader.Options.Chats[chatID] {
			subscribed = append(subscribed, feedUrl)
		} else {
			suggestions = append(suggestions, feedUrl)
		}
	}

	if len(subscribed) > 0 {
		lines = append(lines, "Subscribed:")
		lines = append(lines, subscribed...)
	}

	if len(suggestions) > 0 {
		lines = append(lines, "Suggestions:")
		lines = append(lines, suggestions...)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "No subscriptions yet. Try to /subscribe"
	}
	return text, nil
}

func (as *AggregatorService) checkChat(chatID int64) error {
	if as.chats[chatID] == nil {
		return errors.New("i don't know you")
	}
	return nil
}

func (as *AggregatorService) addReader(feedOptions *types.FeedOptions) {
	fr := feeds.NewFeedReader(feedOptions)
	go fr.Run(as.messageQueue)
	as.feeds[feedOptions.FeedUrl] = fr
}

func (as *AggregatorService) processMessages() {
	for message := range as.messageQueue {
		if as.storage.IsProcessed(message) {
			continue
		}
		log.Infof("New message %+v", message)
		err := as.sendMessage(message.ChatID, message.URL)
		if err == nil {
			_ = as.storage.MarkAsProcessed(message)
		}
	}
}
