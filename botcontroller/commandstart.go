package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"tgRssBot/aggregatorservice"
)

type CommandStart struct {
	AggregatorService *aggregatorservice.AggregatorService
	BotController     *BotController
}

func (c CommandStart) GetCommand() string {
	return "/start"
}

func (c CommandStart) GetDescription() string {
	return "List subscribed feeds and suggestions"
}

func (c CommandStart) Handler(ctx tb.Context) error {
	return c.BotController.HandleNoArgCommand(ctx, nil, c.AggregatorService.Start)
}
