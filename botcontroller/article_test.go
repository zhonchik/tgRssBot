package botcontroller

import (
	"errors"
	xhtml "golang.org/x/net/html"
	tb "gopkg.in/tucnak/telebot.v3"
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
		{"long", strings.Repeat("word ", 600), "", []string{"sendPhoto", "sendMessage"}},
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
					if !strings.HasPrefix(caption, "<b>Headline</b>") || !strings.Contains(caption, "Original article") {
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

func TestLongArticlePreservesText(t *testing.T) {
	body := strings.Repeat("😀<&>word ", 1800)
	pages := articlePagesWithLimit(types.Message{Title: "Title", Text: body, URL: "https://example.com"}, 650)
	if len(pages) < 2 {
		t.Fatal("expected multiple pages")
	}
	recovered := ""
	for i, page := range pages {
		root, err := xhtml.Parse(strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		rendered := ""
		var walk func(*xhtml.Node)
		walk = func(n *xhtml.Node) {
			if n.Type == xhtml.ElementNode && (n.Data == "b" || n.Data == "a") {
				return
			}
			if n.Type == xhtml.TextNode {
				rendered += n.Data
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(root)
		limit := 4096
		if i == 0 {
			limit = 1024 - 256 - 32
		}
		if len(utf16.Encode([]rune(rendered))) > limit {
			t.Fatal("oversized message")
		}
		recovered += strings.Join(strings.Fields(rendered), "")
		if i > 0 && !strings.Contains(page, "<blockquote expandable>") {
			t.Fatal("continuation must be collapsed")
		}
	}
	if recovered != strings.Join(strings.Fields(body), "") {
		t.Fatal("article text lost")
	}
}
