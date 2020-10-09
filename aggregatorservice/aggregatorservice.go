package aggregatorservice

import (
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/tucnak/telebot.v3"
	"strconv"
	"strings"
	"tgRssBot/botcontroller"
	"tgRssBot/feeds"
	"tgRssBot/storage"
	"tgRssBot/types"
	"time"
)

type (
	AggregatorOptions struct {
		BotOptions     botcontroller.BotOptions
		StorageOptions storage.Options
	}

	AggregatorService struct {
		options       AggregatorOptions
		botController *botcontroller.BotController
		storage       *storage.Storage
		feeds         map[string]*types.FeedOptions
		messageQueue  chan types.Message
	}
)

func NewAggregatorService(options AggregatorOptions) (*AggregatorService, error) {
	bc := botcontroller.NewBotController(options.BotOptions)
	log.Info("Bot created")

	s, err := storage.NewStorage(options.StorageOptions)
	if err != nil {
		return nil, err
	}

	messageQueue := make(chan types.Message)

	as := &AggregatorService{
		options:       options,
		botController: bc,
		storage:       s,
		feeds:         make(map[string]*types.FeedOptions),
		messageQueue:  messageQueue,
	}

	feedList, err := s.GetFeeds()
	if err != nil {
		return nil, err
	}
	for _, feed := range feedList {
		as.addFeed(feed)
	}

	go as.processMessages()

	bc.AddHandler("/subscribe", as.handleSubscribeCommand)
	bc.AddHandler("/unsubscribe", as.handleUnsubscribeCommand)

	return as, nil
}

func (as *AggregatorService) Close() error {
	log.Info("Closing service")
	return as.storage.Close()
}

func (as *AggregatorService) addFeed(feedOptions *types.FeedOptions) {
	fr := feeds.NewFeedReader(feedOptions)
	go fr.Run(as.messageQueue)
	as.feeds[feedOptions.FeedUrl] = feedOptions
}

func (as *AggregatorService) processMessages() {
	for message := range as.messageQueue {
		if as.storage.IsProcessed(message) {
			continue
		}
		log.Infof("New message %+v", message)
		err := as.botController.SendTextMessage(message.ChatID, message.URL)
		if err == nil {
			_ = as.storage.MarkAsProcessed(message)
		}
	}
}

func (as *AggregatorService) handleSubscribeCommand(ctx tb.Context) error {
	return as.handleMultiArgCommand(ctx, func(chatID int64, arg string) error {
		feed := as.feeds[arg]
		needStartReader := false
		if feed == nil {
			feed = &types.FeedOptions{
				FeedUrl:       arg,
				Chats:         map[int64]bool{chatID: true},
				CheckInterval: time.Minute,
			}
			needStartReader = true
		}
		needStartReader = needStartReader || !feed.Chats[chatID]
		feed.Chats[chatID] = true
		err := as.storage.SaveFeed(feed)
		if err != nil {
			return err
		}
		if needStartReader {
			as.addFeed(feed)
		}
		return nil
	})
}

func (as *AggregatorService) handleUnsubscribeCommand(ctx tb.Context) error {
	return as.handleMultiArgCommand(ctx, func(chatID int64, arg string) error {
		feed := as.feeds[arg]
		if feed == nil || !feed.Chats[chatID] {
			return errors.New("was not subscribed")
		}
		delete(feed.Chats, chatID)
		err := as.storage.SaveFeed(feed)
		if err != nil {
			return err
		}
		return nil
	})
}

func (as *AggregatorService) handleMultiArgCommand(ctx tb.Context, handler func(chatID int64, arg string) error) error {
	sender := ctx.Sender()
	log.Infof("%s command received from %+v", ctx.Message().Text, sender)
	chatID, err := as.getChatID(sender)
	if err != nil {
		return err
	}
	var lines []string
	for _, arg := range ctx.Args() {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		err := handler(chatID, arg)
		var line string
		if err == nil {
			line = fmt.Sprintf("✅ %s", arg)
		} else {
			line = fmt.Sprintf("⚠️ %s %s", arg, err)
		}
		lines = append(lines, line)
	}
	return as.botController.SendTextMessage(chatID, strings.Join(lines, "\n"))
}

func (as *AggregatorService) getChatID(sender tb.Recipient) (int64, error) {
	chatID, err := strconv.ParseInt(sender.Recipient(), 10, 64)
	if err != nil {
		log.Warnf("Failed to get chat id: %s", err)
		return 0, err
	}
	return chatID, nil
}
