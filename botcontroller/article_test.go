package botcontroller

import (
	"encoding/json"
	"errors"
	"fmt"
	xhtml "golang.org/x/net/html"
	tb "gopkg.in/tucnak/telebot.v3"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"tgRssBot/types"
	"unicode/utf16"
)

func TestArticleHTML(t *testing.T) {
	m := types.Message{Title: "A <headline>", URL: "https://example.com/?a=1&b=2", Text: "Visible <lead>.\n\nHidden & remainder."}
	s := articleHTML(m)
	for _, want := range []string{"<b>A &lt;headline&gt;</b>", "Visible &lt;lead&gt;.", "<blockquote expandable>Hidden &amp; remainder.</blockquote>", `href="https://example.com/?a=1&amp;b=2"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q: %s", want, s)
		}
	}
	if len(utf16.Encode([]rune(clip(strings.Repeat("😀", 4000), 3200)))) > 3200 {
		t.Fatal("UTF-16 limit exceeded")
	}
	s = articleHTML(types.Message{Text: strings.Repeat("word ", 200), URL: "javascript:bad"})
	if !strings.Contains(s, "<blockquote expandable>") || strings.Contains(s, "href=") {
		t.Fatal(s)
	}
}

func TestArticleDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, body, photoError string
		want                   []string
	}{
		{"short", "Lead.\n\nRest.", "", []string{"sendPhoto"}},
		{"fits caption", strings.Repeat("a", 800), "", []string{"sendPhoto"}},
		{"long", strings.Repeat("word ", 600), "", []string{"sendPhoto"}},
		{"bad image", "Lead.", "Bad Request: failed to get HTTP URL content", []string{"sendPhoto", "sendMessage"}},
		{"transport", "Lead.", "timeout", []string{"sendPhoto"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var methods []string
			err := sendArticle(func(method string, payload interface{}) ([]byte, error) {
				methods = append(methods, method)
				p := payload.(map[string]interface{})

				if method == "sendPhoto" {
					caption := p["caption"].(string)
					if !strings.HasPrefix(caption, "<b>Headline</b>") || !strings.Contains(caption, "Source") {
						t.Fatal("caption must include headline and article link")
					}
					root, parseErr := xhtml.Parse(strings.NewReader(caption))
					if parseErr != nil {
						t.Fatal(parseErr)
					}
					visible := ""
					var walk func(*xhtml.Node)
					walk = func(n *xhtml.Node) {
						if n.Type == xhtml.TextNode {
							visible += n.Data
						}
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							walk(c)
						}
					}
					walk(root)
					if len(utf16.Encode([]rune(visible))) > 1024 {
						t.Fatal("oversized caption")
					}
					if tc.photoError != "" {
						if tc.name == "bad image" {
							return nil, tb.ErrBadURLContent
						}
						return nil, errors.New(tc.photoError)
					}
				}
				return nil, nil
			}, types.Message{Title: "Headline", URL: "https://example.com/story", ImageURL: "https://example.com/photo.jpg", Text: tc.body})
			if strings.Join(methods, ",") != strings.Join(tc.want, ",") {
				t.Fatal(methods)
			}
			if (err != nil) != (tc.name == "transport") {
				t.Fatal(err)
			}
		})
	}
}

func TestLongArticleSingleMessage(t *testing.T) {
	for _, photo := range []bool{false, true} {
		m := types.Message{Title: strings.Repeat("😀<&>", 100), Text: strings.Repeat("😀<&>word ", 1800), URL: "https://example.com"}
		limit := 4096
		if photo {
			m.ImageURL = "https://example.com/image.jpg"
			limit = 1024
		}
		calls := 0
		err := sendArticle(func(method string, payload interface{}) ([]byte, error) {
			calls++
			p := payload.(map[string]interface{})
			key := "text"
			if photo {
				key = "caption"
				if method != "sendPhoto" {
					t.Fatal(method)
				}
			} else if method != "sendMessage" {
				t.Fatal(method)
			}
			markup := p[key].(string)
			if !strings.Contains(markup, "…") || !strings.Contains(markup, `href="https://example.com">Source</a>`) {
				t.Fatal("missing truncation or source")
			}
			root, err := xhtml.Parse(strings.NewReader(markup))
			if err != nil {
				t.Fatal(err)
			}
			var visible strings.Builder
			var walk func(*xhtml.Node)
			walk = func(n *xhtml.Node) {
				if n.Type == xhtml.TextNode {
					visible.WriteString(n.Data)
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(root)
			if len(utf16.Encode([]rune(visible.String()))) > limit {
				t.Fatal("message limit exceeded")
			}
			return nil, nil
		}, m)
		if err != nil || calls != 1 {
			t.Fatalf("calls=%d error=%v", calls, err)
		}
	}
}

func TestTelegramUnknownPhotoErrorFallsBackToText(t *testing.T) {
	for _, tc := range []struct {
		name        string
		code        int
		description string
		wantText    bool
	}{
		{"production wrong port", 400, "Bad Request: invalid file HTTP URL specified: Wrong port number specified in the URL", true},
		{"new photo rejection", 400, "Bad Request: PHOTO_INVALID_DIMENSIONS", true},
		{"rate limit", 429, "Too Many Requests: retry after 10", false},
		{"server error", 500, "Internal Server Error", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var methods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				w.Header().Set("Content-Type", "application/json")
				if method == "getMe" {
					fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test"}}`)
					return
				}
				methods = append(methods, method)
				if method == "sendPhoto" {
					w.WriteHeader(tc.code)
					json.NewEncoder(w).Encode(struct {
						OK          bool   `json:"ok"`
						Code        int    `json:"error_code"`
						Description string `json:"description"`
					}{false, tc.code, tc.description})
					return
				}
				var payload struct {
					Text string `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if !strings.Contains(payload.Text, "Headline") || !strings.Contains(payload.Text, "Source") {
					t.Error("fallback lost article")
				}
				fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
			}))
			defer server.Close()
			bot, err := tb.NewBot(tb.Settings{URL: server.URL, Token: "test", Client: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			err = sendArticle(bot.Raw, types.Message{ChatID: 1, Title: "Headline", Text: "Body", URL: "https://example.com/story", ImageURL: "https://example.com/image.jpg"})
			want := "sendPhoto"
			if tc.wantText {
				want += ",sendMessage"
			}
			if strings.Join(methods, ",") != want {
				t.Fatalf("got %v, want %s; error: %v", methods, want, err)
			}
			if (err == nil) != tc.wantText {
				t.Fatal(err)
			}
		})
	}
}
