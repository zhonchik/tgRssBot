package botcontroller

import (
	"encoding/json"
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
	sendText := func(text string) error {
		_, err := call("sendMessage", map[string]interface{}{
			"chat_id": message.ChatID, "text": text, "parse_mode": "HTML",
			"link_preview_options":     map[string]bool{"is_disabled": true},
			"disable_web_page_preview": true,
		})
		return err
	}
	if message.ImageURL == "" {
		return sendText(articleWithLimit(message, 4096))
	}
	response, err := call("sendPhoto", map[string]interface{}{
		"chat_id": message.ChatID, "photo": message.ImageURL,
		"caption": articleWithLimit(message, 1024), "parse_mode": "HTML",
	})
	// Read the response directly: the pinned library only types known errors
	// and its legacy parser can miss errors when JSON fields are reordered.
	var result struct {
		OK          *bool  `json:"ok"`
		Code        int    `json:"error_code"`
		Description string `json:"description"`
	}
	explicitFailure := json.Unmarshal(response, &result) == nil && result.OK != nil && !*result.OK
	if explicitFailure && err == nil {
		err = tb.NewAPIError(result.Code, result.Description)
	}
	if err == nil {
		return nil
	}
	// Fall back only after an explicit 400 rejection; ambiguous transport,
	// rate-limit and server failures retain the queued article for retry.
	var apiErr *tb.APIError
	rejected := errors.As(err, &apiErr) && apiErr.Code == 400
	if explicitFailure && result.Code == 400 {
		rejected = true
	}
	if !rejected {
		return err
	}
	return sendText(articleWithLimit(message, 4096))
}

// Reserve room for the headline, Source link, and paragraph separators before
// clipping plain text. HTML escaping happens afterwards, keeping markup intact.
func articleWithLimit(message types.Message, limit int) string {
	title := html.UnescapeString(strings.TrimSuffix(strings.TrimPrefix(articleTitle(message.Title), "<b>"), "</b>"))
	bodyLimit := limit - len(utf16.Encode([]rune(title))) - len("Source") - 12
	message.Text = clip(strings.TrimSpace(message.Text), bodyLimit)
	return articleHTML(message)
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
		return "\n\n<a href=\"" + html.EscapeString(address) + "\">Source</a>"
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
