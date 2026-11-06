package botcontroller

import (
	tb "gopkg.in/tucnak/telebot.v3"
	"strings"
	"testing"
	"tgRssBot/aggregatorservice"
)

// The complete callback payload must fit Telegram's 64-byte limit even for long URLs.
func TestSettingsCallbackSize(t *testing.T) {
	urls := []string{"https://example.com/rss", "https://example.com/a/very/long/feed/path?filter=some-long-query-value&another=parameter"}
	for _, url := range urls {
		key := settingsKey(url)
		if len("\fsettings|maximum:"+key) > 64 {
			t.Fatal("callback exceeds Telegram limit")
		}
		if key != settingsKey(url) {
			t.Fatal("unstable feed key")
		}
	}
	if settingsKey(urls[0]) == settingsKey(urls[1]) {
		t.Fatal("feeds share callback key")
	}
}

type settingsContext struct {
	tb.Context
	message      *tb.Message
	sender       tb.Recipient
	callback     *tb.Callback
	sent         string
	markup       *tb.ReplyMarkup
	acknowledged bool
}

func (c *settingsContext) Message() *tb.Message   { return c.message }
func (c *settingsContext) Sender() tb.Recipient   { return c.sender }
func (c *settingsContext) Callback() *tb.Callback { return c.callback }
func (c *settingsContext) Send(v interface{}, opts ...interface{}) error {
	c.sent = v.(string)
	if len(opts) > 0 {
		c.markup = opts[0].(*tb.ReplyMarkup)
	}
	return nil
}
func (c *settingsContext) Respond(...*tb.CallbackResponse) error { c.acknowledged = true; return nil }
func TestSettingsPrivateAndEmptyMenu(t *testing.T) {
	c := &CommandSettings{AggregatorService: &aggregatorservice.AggregatorService{}, pending: make(map[int64]settingsInput)}
	ctx := &settingsContext{message: &tb.Message{Chat: &tb.Chat{ID: -10}}, sender: &tb.User{ID: 1}}
	if err := c.Handler(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.markup != nil || !strings.Contains(ctx.sent, "private chat") {
		t.Fatal("group exposed settings")
	}
	ctx.message.Chat.ID = 1
	c.pending[1] = settingsInput{Field: "keyword"}
	if err := c.Handler(ctx); err != nil {
		t.Fatal(err)
	}
	if len(c.pending) != 0 || ctx.markup == nil || len(ctx.markup.InlineKeyboard) != 1 {
		t.Fatal("empty menu or pending reset failed")
	}
	ctx.callback = &tb.Callback{Data: "add"}
	if err := c.callback(ctx); err != nil {
		t.Fatal(err)
	}
	if !ctx.acknowledged || c.pending[1].Field != "add" {
		t.Fatal("add callback not handled")
	}
	ctx.callback.Data = "feed:missing"
	if err := c.callback(ctx); err != nil {
		t.Fatal(err)
	}
	if len(c.pending) != 0 || !strings.Contains(ctx.sent, "No subscriptions") {
		t.Fatal("stale callback not handled")
	}
}
