package botcontroller

import (
	"crypto/sha256"
	"fmt"
	tb "gopkg.in/tucnak/telebot.v3"
	"sort"
	"strconv"
	"strings"
	"sync"
	"tgRssBot/aggregatorservice"
)

type settingsInput struct{ URL, Field string }
type CommandSettings struct {
	BotController     *BotController
	AggregatorService *aggregatorservice.AggregatorService
	URL               string
	mu                sync.Mutex
	pending           map[int64]settingsInput
}

func (c *CommandSettings) GetCommand() string     { return "/settings" }
func (c *CommandSettings) GetDescription() string { return "Manage feed settings" }
func settingsKey(url string) string               { return fmt.Sprintf("%x", sha256.Sum256([]byte(url)))[:16] }
func (c *CommandSettings) private(ctx tb.Context) bool {
	return ctx.Message() != nil && ctx.Message().Chat != nil && ctx.Sender() != nil && ctx.Sender().Recipient() == strconv.FormatInt(ctx.Message().Chat.ID, 10)
}
func (c *CommandSettings) Register() {
	c.pending = make(map[int64]settingsInput)
	c.BotController.bot.Handle(&tb.Btn{Unique: "settings"}, c.callback)
	c.BotController.bot.Handle(tb.OnText, c.input)
	c.BotController.bot.Handle("/cancel", func(ctx tb.Context) error {
		if !c.private(ctx) {
			return nil
		}
		c.clear(ctx.Message().Chat.ID)
		return c.menu(ctx, "")
	})
	c.BotController.AddHandler(c)
}
func (c *CommandSettings) clear(id int64) { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }
func (c *CommandSettings) Handler(ctx tb.Context) error {
	if !c.private(ctx) {
		return ctx.Send("Open /settings in a private chat with me.")
	}
	c.clear(ctx.Message().Chat.ID)
	return c.menu(ctx, "")
}
func (c *CommandSettings) menu(ctx tb.Context, key string) error {
	subs := c.AggregatorService.Subscriptions(ctx.Message().Chat.ID)
	menu := &tb.ReplyMarkup{}
	var rows [][]tb.Btn
	text := "Settings — choose a subscription."
	if key == "" {
		urls := make([]string, 0, len(subs))
		for u := range subs {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		for _, u := range urls {
			rows = append(rows, menu.Row(menu.Data(u, "settings", "feed:"+settingsKey(u))))
		}
		if len(urls) == 0 {
			text = "No subscriptions yet. Add a feed below."
		}
		rows = append(rows, menu.Row(menu.Data("Add subscription", "settings", "add")))
		if c.URL != "" {
			rows = append(rows, menu.Row(menu.URL("Web settings", c.URL)))
		}
	} else {
		found := false
		for u, s := range subs {
			if settingsKey(u) != key {
				continue
			}
			found = true
			text = fmt.Sprintf("%s\nPaused: %t\nInterval: %d minutes\nItems per check: %d (0 = all)\nKeyword: %s", u, s.Paused, s.Minutes, s.MaxItems, s.Keyword)
			label := "Pause delivery"
			if s.Paused {
				label = "Resume delivery"
			}
			for _, v := range []struct{ label, action string }{{label, "pause"}, {"Change interval", "minutes"}, {"Change item limit", "maximum"}, {"Change keyword", "keyword"}, {"Clear keyword", "clear"}, {"Remove subscription", "remove"}} {
				rows = append(rows, menu.Row(menu.Data(v.label, "settings", v.action+":"+key)))
			}
		}
		if !found {
			text = "Subscription no longer exists."
		}
		rows = append(rows, menu.Row(menu.Data("Back", "settings", "back")))
	}
	for _, row := range rows {
		var buttons []tb.InlineButton
		for _, button := range row {
			buttons = append(buttons, *button.Inline())
		}
		menu.InlineKeyboard = append(menu.InlineKeyboard, buttons)
	}
	return ctx.Send(text, menu)
}
func (c *CommandSettings) callback(ctx tb.Context) error {
	if !c.private(ctx) {
		return ctx.Respond()
	}
	if err := ctx.Respond(); err != nil {
		return err
	}
	c.clear(ctx.Message().Chat.ID)
	parts := strings.SplitN(ctx.Callback().Data, ":", 2)
	action := parts[0]
	if action == "back" {
		return c.menu(ctx, "")
	}
	if action == "add" {
		c.mu.Lock()
		c.pending[ctx.Message().Chat.ID] = settingsInput{Field: "add"}
		c.mu.Unlock()
		return ctx.Send("Send the feed URL, or /cancel.")
	}
	if len(parts) != 2 {
		return c.menu(ctx, "")
	}
	key := parts[1]
	for u, s := range c.AggregatorService.Subscriptions(ctx.Message().Chat.ID) {
		if settingsKey(u) != key {
			continue
		}
		switch action {
		case "feed":
			return c.menu(ctx, key)
		case "pause":
			s.Paused = !s.Paused
		case "clear":
			s.Keyword = ""
		case "remove":
			menu := &tb.ReplyMarkup{}
			menu.Inline(menu.Row(menu.Data("Confirm removal", "settings", "delete:"+key), menu.Data("Cancel", "settings", "feed:"+key)))
			return ctx.Send("Remove "+u+" and its queued articles?", menu)
		case "delete":
			if err := c.AggregatorService.TryUnsubscribe(ctx.Message().Chat.ID, u); err != nil {
				return ctx.Send(err.Error())
			}
			return c.menu(ctx, "")
		case "minutes", "maximum", "keyword":
			c.mu.Lock()
			c.pending[ctx.Message().Chat.ID] = settingsInput{u, action}
			c.mu.Unlock()
			prompts := map[string]string{"minutes": "Send an interval from 1 to 1440 minutes.", "maximum": "Send an item limit from 0 to 100 (0 = all).", "keyword": "Send a keyword filter, up to 120 characters."}
			return ctx.Send(prompts[action] + " Or /cancel.")
		default:
			return c.menu(ctx, "")
		}
		if err := c.AggregatorService.Update(ctx.Message().Chat.ID, u, s); err != nil {
			return ctx.Send(err.Error())
		}
		return c.menu(ctx, key)
	}
	return c.menu(ctx, "")
}
func (c *CommandSettings) input(ctx tb.Context) error {
	if !c.private(ctx) {
		return nil
	}
	id := ctx.Message().Chat.ID
	c.mu.Lock()
	p, ok := c.pending[id]
	c.mu.Unlock()
	if !ok {
		return nil
	}
	value := strings.TrimSpace(ctx.Message().Text)
	if p.Field == "add" {
		if err := c.AggregatorService.TrySubscribe(id, value); err != nil {
			return ctx.Send(err.Error() + " Try again or /cancel.")
		}
		c.clear(id)
		return c.menu(ctx, "")
	}
	s, ok := c.AggregatorService.Subscriptions(id)[p.URL]
	if !ok {
		c.clear(id)
		return c.menu(ctx, "")
	}
	switch p.Field {
	case "keyword":
		s.Keyword = value
	case "minutes", "maximum":
		n, err := strconv.Atoi(value)
		if err != nil {
			return ctx.Send("Enter a whole number, or /cancel.")
		}
		if p.Field == "minutes" {
			s.Minutes = n
		} else {
			s.MaxItems = n
		}
	}
	if err := c.AggregatorService.Update(id, p.URL, s); err != nil {
		return ctx.Send(err.Error() + " Try again or /cancel.")
	}
	c.clear(id)
	return c.menu(ctx, settingsKey(p.URL))
}
