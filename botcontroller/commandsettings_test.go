package botcontroller

import (
	"fmt"
	tb "gopkg.in/tucnak/telebot.v3"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"tgRssBot/aggregatorservice"
	"tgRssBot/storage"
	"tgRssBot/types"
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
	if len(c.pending) != 0 || ctx.markup == nil || len(ctx.markup.InlineKeyboard) != 2 {
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

func TestSettingsCalendarWorkflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/calendar" {
			fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:race\r\nDTSTART:20990101T120000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
			return
		}
		fmt.Fprint(w, "<rss version=\"2.0\"><channel><title>Test</title></channel></rss>")
	}))
	defer server.Close()
	db, err := storage.NewStorage(storage.Options{Path: filepath.Join(t.TempDir(), "db")})
	if err != nil {
		t.Fatal(err)
	}
	as, err := aggregatorservice.NewAggregatorService(func(types.Message) error { return nil }, db)
	if err != nil {
		t.Fatal(err)
	}
	defer as.Close()
	if _, err := as.Start(1); err != nil {
		t.Fatal(err)
	}
	feed, link := server.URL+"/rss", server.URL+"/calendar"
	if err := as.TrySubscribe(1, feed); err != nil {
		t.Fatal(err)
	}
	c := &CommandSettings{AggregatorService: as, pending: make(map[int64]settingsInput)}
	ctx := &settingsContext{message: &tb.Message{Chat: &tb.Chat{ID: 1}}, sender: &tb.User{ID: 1}, callback: &tb.Callback{}}
	click := func(data string) {
		t.Helper()
		ctx.callback.Data = data
		if err := c.callback(ctx); err != nil {
			t.Fatal(err)
		}
		for _, row := range ctx.markup.InlineKeyboard {
			for _, button := range row {
				if len(button.Data) > 64 {
					t.Fatal("callback too long", button.Data)
				}
			}
		}
	}
	click("calendars")
	click("caladd")
	ctx.message.Text = link
	if err := c.input(ctx); err != nil {
		t.Fatal(err)
	}
	if len(as.CalendarLinks(1)) != 1 || len(c.pending) != 0 {
		t.Fatal("add calendar failed")
	}
	click("calpick:" + settingsKey(feed))
	click("calbind:" + settingsKey(feed) + ":" + settingsKey(link))
	if as.Subscriptions(1)[feed].Calendar != link {
		t.Fatal("link failed")
	}
	click("calnext:" + settingsKey(feed))
	if as.Subscriptions(1)[feed].CalendarAfter.Year() != 2099 {
		t.Fatal("advance failed")
	}
	click("calresume:" + settingsKey(link))
	if as.Subscriptions(1)[feed].CalendarAfter.Year() == 2099 {
		t.Fatal("resume failed")
	}
	click("calunlink:" + settingsKey(feed))
	if as.Subscriptions(1)[feed].Calendar != "" {
		t.Fatal("unlink failed")
	}
	click("calbind:" + settingsKey(feed) + ":" + settingsKey(link))
	click("caladvance:" + settingsKey(link))
	if as.Subscriptions(1)[feed].CalendarAfter.Year() != 2099 {
		t.Fatal("bulk advance failed")
	}
	click("calremove:" + settingsKey(link))
	if len(as.CalendarLinks(1)) != 1 {
		t.Fatal("deleted before confirmation")
	}
	click("caldelete:" + settingsKey(link))
	if len(as.CalendarLinks(1)) != 0 || as.Subscriptions(1)[feed].Calendar != "" {
		t.Fatal("delete failed")
	}
	click("calview:" + settingsKey(link)) // A stale button must be harmless.
	if strings.Contains(ctx.sent, link) {
		t.Fatal("stale calendar exposed")
	}
}
