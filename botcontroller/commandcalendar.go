package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"tgRssBot/aggregatorservice"
)

type CommandCalendar struct {
	AggregatorService *aggregatorservice.AggregatorService
	BotController     *BotController
}

func (c CommandCalendar) GetCommand() string { return "/calendar" }
func (c CommandCalendar) GetDescription() string {
	return "list, add, delete, link, unlink, resume, advance or next — calendar pause controls"
}
func (c CommandCalendar) Handler(ctx tb.Context) error {
	return c.BotController.HandleNoArgCommand(ctx, c.AggregatorService.CheckChat, func(id int64) (string, error) { return c.AggregatorService.CalendarCommand(id, ctx.Args()) })
}
