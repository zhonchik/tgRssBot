package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"tgRssBot/aggregatorservice"
)

type CommandSubscribe struct {
	AggregatorService *aggregatorservice.AggregatorService
	BotController     *BotController
}

func (c CommandSubscribe) GetCommand() string {
	return "/subscribe"
}

func (c CommandSubscribe) GetDescription() string {
	return "Start getting feed messages (multiple urls space separated are supported)"
}

func (c CommandSubscribe) Handler(ctx tb.Context) error {
	return c.BotController.HandleMultiArgCommand(ctx, c.AggregatorService.TrySubscribe)
}
