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
	"tgRssBot/types"
	"time"
)

type (
	AggregatorOptions struct {
		BotOptions botcontroller.BotOptions
	}

	AggregatorService struct {
		options           AggregatorOptions
		botController     *botcontroller.BotController
		feeds             map[string]*types.FeedOptions
		messageQueue      chan types.Message
		processedMessages map[types.Message]bool
	}
)

func NewAggregatorService(options AggregatorOptions) *AggregatorService {
	bc := botcontroller.NewBotController(options.BotOptions)
	log.Info("Bot created")

	messageQueue := make(chan types.Message)

	as := &AggregatorService{
		options:           options,
		botController:     bc,
		feeds:             make(map[string]*types.FeedOptions),
		messageQueue:      messageQueue,
		processedMessages: make(map[types.Message]bool),
	}

	go as.processMessages()

	bc.AddHandler("/subscribe", as.handleSubscribeCommand)
	bc.AddHandler("/unsubscribe", as.handleUnsubscribeCommand)

	return as
}

func (as *AggregatorService) addFeed(feedOptions *types.FeedOptions) {
	fr := feeds.NewFeedReader(feedOptions)
	go fr.Run(as.messageQueue)
	as.feeds[feedOptions.FeedUrl] = feedOptions
}

func (as *AggregatorService) processMessages() {
	for message := range as.messageQueue {
		var processedMessages []types.Message
		if as.processedMessages[message] {
			continue
		}
		log.Infof("New message %+v", message)
		err := as.botController.SendTextMessage(message.ChatID, message.URL)
		if err == nil {
			processedMessages = append(processedMessages, message)
		}
		as.markAsProcessed(processedMessages)
	}
}

func (as *AggregatorService) markAsProcessed(messages []types.Message) {
	for _, message := range messages {
		as.processedMessages[message] = true
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
