package botcontroller

import (
	"fmt"
	tb "gopkg.in/tucnak/telebot.v3"
	"strings"
)

func (c *CommandSettings) calendarMenu(ctx tb.Context, calendarKey, feedKey string) error {
	menu := &tb.ReplyMarkup{}
	var rows [][]tb.Btn
	text := "Calendars — choose a calendar."
	links := c.AggregatorService.CalendarLinks(ctx.Message().Chat.ID)
	if calendarKey == "" {
		if len(links) == 0 {
			text = "No calendars yet. Add an iCalendar subscription URL below."
		}
		if feedKey != "" {
			text = "Choose a calendar to link to this subscription."
		}
		for _, link := range links {
			action := "calview:" + settingsKey(link)
			if feedKey != "" {
				action = "calbind:" + feedKey + ":" + settingsKey(link)
			}
			rows = append(rows, menu.Row(menu.Data(link, "settings", action)))
		}
		rows = append(rows, menu.Row(menu.Data("Add calendar", "settings", "caladd")))
	} else {
		found := false
		for _, link := range links {
			if settingsKey(link) != calendarKey {
				continue
			}
			found = true
			var feeds []string
			for feed, s := range c.AggregatorService.Subscriptions(ctx.Message().Chat.ID) {
				if s.Calendar == link {
					feeds = append(feeds, feed)
				}
			}
			text = fmt.Sprintf("Calendar: %s\nLinked subscriptions: %d", link, len(feeds))
			for _, v := range []struct{ label, action string }{{"Resume all linked subscriptions", "calresume"}, {"Advance all to next event", "caladvance"}, {"Delete calendar", "calremove"}} {
				rows = append(rows, menu.Row(menu.Data(v.label, "settings", v.action+":"+calendarKey)))
			}
		}
		if !found {
			text = "Calendar no longer exists."
		}
	}
	back := "calendars"
	if calendarKey == "" {
		back = "back"
	}
	if feedKey != "" {
		back = "feed:" + feedKey
	}
	rows = append(rows, menu.Row(menu.Data("Back", "settings", back)))
	for _, row := range rows {
		var buttons []tb.InlineButton
		for _, button := range row {
			buttons = append(buttons, *button.Inline())
		}
		menu.InlineKeyboard = append(menu.InlineKeyboard, buttons)
	}
	return ctx.Send(text, menu)
}

func (c *CommandSettings) calendarCallback(ctx tb.Context, data string) (bool, error) {
	if !strings.HasPrefix(data, "cal") {
		return false, nil
	}
	id := ctx.Message().Chat.ID
	parts := strings.Split(data, ":")
	action := parts[0]
	if action == "calendars" {
		return true, c.calendarMenu(ctx, "", "")
	}
	if action == "caladd" {
		c.mu.Lock()
		c.pending[id] = settingsInput{Field: "calendar-add"}
		c.mu.Unlock()
		return true, ctx.Send("Send an HTTP(S) or webcal iCalendar subscription URL, or /cancel.")
	}
	if len(parts) < 2 {
		return true, c.calendarMenu(ctx, "", "")
	}
	key := parts[1]
	if action == "calpick" || action == "calunlink" || action == "calnext" || action == "calbind" {
		for feed := range c.AggregatorService.Subscriptions(id) {
			if settingsKey(feed) != key {
				continue
			}
			if action == "calpick" {
				return true, c.calendarMenu(ctx, "", key)
			}
			args := []string{"unlink", feed}
			if action == "calnext" {
				args[0] = "next"
			}
			if action == "calbind" {
				if len(parts) != 3 {
					return true, c.menu(ctx, key)
				}
				link := ""
				for _, candidate := range c.AggregatorService.CalendarLinks(id) {
					if settingsKey(candidate) == parts[2] {
						link = candidate
					}
				}
				if link == "" {
					return true, ctx.Send("Calendar no longer exists. Reopen /settings.")
				}
				args = []string{"link", feed, link}
			}
			if _, err := c.AggregatorService.CalendarCommand(id, args); err != nil {
				return true, ctx.Send(err.Error())
			}
			return true, c.menu(ctx, key)
		}
		return true, c.menu(ctx, "")
	}
	for _, link := range c.AggregatorService.CalendarLinks(id) {
		if settingsKey(link) != key {
			continue
		}
		switch action {
		case "calview":
			return true, c.calendarMenu(ctx, key, "")
		case "calremove":
			menu := &tb.ReplyMarkup{}
			menu.Inline(menu.Row(menu.Data("Confirm deletion", "settings", "caldelete:"+key), menu.Data("Cancel", "settings", "calview:"+key)))
			return true, ctx.Send("Delete "+link+"? All linked subscriptions will be disconnected. Queued articles are kept and may resume delivery.", menu)
		case "calresume", "caladvance", "caldelete":
			result, err := c.AggregatorService.CalendarCommand(id, []string{strings.TrimPrefix(action, "cal"), link})
			if err != nil {
				return true, ctx.Send(err.Error())
			}
			if err := ctx.Send(result); err != nil {
				return true, err
			}
			if action == "caldelete" {
				return true, c.calendarMenu(ctx, "", "")
			}
			return true, c.calendarMenu(ctx, key, "")
		}
	}
	return true, c.calendarMenu(ctx, "", "")
}
