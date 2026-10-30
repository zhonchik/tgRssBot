package botcontroller

import (
	"errors"
	tb "gopkg.in/tucnak/telebot.v3"
	"html"
	"net/url"
	"strings"
	"tgRssBot/types"
	"unicode/utf16"
)

// SendArticle bypasses automatic link previews and uses Telegram's expandable
// blockquote support, which is newer than the pinned telebot wrapper.
func (bc *BotController) SendArticle(message types.Message) error {
	return sendArticle(bc.bot.Raw, message)
}

type articleAPI func(string, interface{}) ([]byte, error)

func sendArticle(call articleAPI, message types.Message) error {
	firstLimit := 3200
	if message.ImageURL != "" {
		title := html.UnescapeString(strings.TrimSuffix(strings.TrimPrefix(articleTitle(message.Title), "<b>"), "</b>"))
		// Reserve the visible title, source link and paragraph separators.
		firstLimit = 1024 - len(utf16.Encode([]rune(title))) - len("Original article") - 12
	}
	pages := articlePagesWithLimit(message, firstLimit)
	sendText := func(text string) error {
		_, err := call("sendMessage", map[string]interface{}{
			"chat_id": message.ChatID, "text": text, "parse_mode": "HTML",
			"link_preview_options":     map[string]bool{"is_disabled": true},
			"disable_web_page_preview": true,
		})
		return err
	}
	sendPages := func(pages []string) error {
		for _, page := range pages {
			if err := sendText(page); err != nil {
				return err
			}
		}
		return nil
	}
	if message.ImageURL == "" {
		return sendPages(pages)
	}
	_, err := call("sendPhoto", map[string]interface{}{
		"chat_id": message.ChatID, "photo": message.ImageURL,
		"caption": pages[0], "parse_mode": "HTML",
	})
	if err == nil {
		return sendPages(pages[1:])
	}
	// A rejected image must not suppress the article. Retry ambiguous transport
	// failures on the next feed poll rather than marking the article delivered.
	if err != nil {
		var apiErr *tb.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 400 {
			return err
		}
	}
	return sendPages(articlePages(message))
}

// Keep all feed text, splitting only when Telegram's message limit requires it.
func articlePages(message types.Message) []string {
	return articlePagesWithLimit(message, 3200)
}

func articlePagesWithLimit(message types.Message, firstLimit int) []string {
	remaining := strings.TrimSpace(message.Text)
	first, remaining := takeText(remaining, firstLimit)
	message.Text = first
	pages := []string{articleHTML(message)}
	for remaining != "" {
		var chunk string
		chunk, remaining = takeText(remaining, 3200)
		pages = append(pages, "<blockquote expandable>"+html.EscapeString(chunk)+"</blockquote>"+articleLink(message.URL))
	}
	return pages
}

func takeText(text string, limit int) (string, string) {
	units := 0
	for i, r := range text {
		size := 1
		if r > 0xffff {
			size = 2
		}
		if units+size > limit {
			return text[:i], text[i:]
		}
		units += size
	}
	return text, ""
}

func articleTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "News"
	}
	return "<b>" + html.EscapeString(clip(title, 256)) + "</b>"
}

func articleHTML(message types.Message) string {
	title := strings.TrimSpace(message.Title)
	if title == "" {
		title = "News"
	}
	body := strings.TrimSpace(message.Text)
	lead, rest := body, ""
	if i := strings.Index(body, "\n\n"); i >= 0 {
		lead, rest = body[:i], strings.TrimSpace(body[i+2:])
	}
	// A long single paragraph also needs a useful visible excerpt.
	if len([]rune(lead)) > 350 {
		runes := []rune(lead)
		cut := 350
		for i := 350; i >= 200; i-- {
			if runes[i] == ' ' {
				cut = i
				break
			}
		}
		tail := strings.TrimSpace(string(runes[cut:]))
		lead = strings.TrimSpace(string(runes[:cut]))
		if rest != "" {
			tail += "\n\n" + rest
		}
		rest = tail
	}
	text := articleTitle(title)
	if lead != "" {
		text += "\n\n" + html.EscapeString(lead)
	}
	if rest != "" {
		text += "\n\n<blockquote expandable>" + html.EscapeString(rest) + "</blockquote>"
	}
	return text + articleLink(message.URL)
}

func articleLink(address string) string {
	u, err := url.Parse(address)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return "\n\n<a href=\"" + html.EscapeString(address) + "\">Original article</a>"
	}
	return ""
}

// clip counts UTF-16 units, as required by Telegram, and never cuts a rune.
func clip(text string, limit int) string {
	units := 0
	for i, r := range text {
		size := 1
		if r > 0xffff {
			size = 2
		}
		if units+size > limit-1 {
			return text[:i] + "…"
		}
		units += size
	}
	return text
}
