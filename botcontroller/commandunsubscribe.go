package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"tgRssBot/aggregatorservice"
)

type CommandUnsubscribe struct {
	AggregatorService *aggregatorservice.AggregatorService
	BotController     *BotController
}

func (c CommandUnsubscribe) GetCommand() string {
	return "/unsubscribe"
}

func (c CommandUnsubscribe) GetDescription() string {
	return "Stop getting feed messages (multiple urls space separated are supported)"
}

func (c CommandUnsubscribe) Handler(ctx tb.Context) error {
	return c.BotController.HandleMultiArgCommand(ctx, c.AggregatorService.TryUnsubscribe)
}
