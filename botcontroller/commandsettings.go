package botcontroller

import tb "gopkg.in/tucnak/telebot.v3"

type CommandSettings struct {
	BotController *BotController
	URL           string
}

func (c CommandSettings) GetCommand() string     { return "/settings" }
func (c CommandSettings) GetDescription() string { return "Manage subscriptions" }
func (c CommandSettings) Handler(ctx tb.Context) error {
	if ctx.Message().Chat.ID != int64(ctx.Message().Sender.ID) {
		return ctx.Send("Open /settings in a private chat with me.")
	}
	_, err := c.BotController.bot.Raw("sendMessage", map[string]interface{}{
		"chat_id": ctx.Message().Sender.ID, "text": "Manage your subscriptions",
		"reply_markup": map[string]interface{}{"inline_keyboard": [][]interface{}{{map[string]interface{}{"text": "Open settings", "web_app": map[string]string{"url": c.URL}}}}},
	})
	return err
}
