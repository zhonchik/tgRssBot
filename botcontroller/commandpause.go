package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"tgRssBot/aggregatorservice"
)

type CommandPause struct {
	AggregatorService *aggregatorservice.AggregatorService
	BotController     *BotController
	Paused            bool
}

func (c CommandPause) GetCommand() string {
	if c.Paused {
		return "/pause"
	}
	return "/resume"
}

func (c CommandPause) GetDescription() string {
	if c.Paused {
		return "<feed URL> — store new items for later delivery"
	}
	return "<feed URL> — deliver queued items and resume updates"
}

func (c CommandPause) Handler(ctx tb.Context) error {
	return c.BotController.HandleMultiArgCommand(ctx, c.AggregatorService.CheckChat, func(id int64, url string) error {
		return c.AggregatorService.SetPaused(id, url, c.Paused)
	})
}
