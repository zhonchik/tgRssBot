package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"tgRssBot/aggregatorservice"
)

type CommandList struct {
	AggregatorService *aggregatorservice.AggregatorService
	BotController     *BotController
}

func (c CommandList) GetCommand() string {
	return "/list"
}

func (c CommandList) GetDescription() string {
	return "List subscribed feeds and suggestions"
}

func (c CommandList) Handler(ctx tb.Context) error {
	return c.BotController.HandleNoArgCommand(ctx, c.AggregatorService.GetFeedList)
}
