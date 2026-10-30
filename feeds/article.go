package feeds

import (
	"net/url"
	"strings"
	"tgRssBot/types"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

// articleMessage uses publisher-provided content; it does not fetch arbitrary article pages.
func articleMessage(item *gofeed.Item) types.Message {
	content := item.Content
	if strings.TrimSpace(content) == "" {
		content = item.Description
	}
	text, inlineImage := articleContent(content)
	m := types.Message{URL: item.Link, Title: item.Title, Text: text}
	candidates := []string{}
	if item.Image != nil {
		candidates = append(candidates, item.Image.URL)
	}
	for _, name := range []string{"content", "thumbnail"} {
		for _, ext := range item.Extensions["media"][name] {
			candidates = append(candidates, ext.Attrs["url"])
		}
	}
	for _, enclosure := range item.Enclosures {
		if strings.HasPrefix(enclosure.Type, "image/") {
			candidates = append(candidates, enclosure.URL)
		}
	}
	candidates = append(candidates, inlineImage)
	for _, candidate := range candidates {
		if m.ImageURL = resolveImage(item.Link, candidate); m.ImageURL != "" {
			break
		}
	}
	return m
}

func resolveImage(base, image string) string {
	if strings.TrimSpace(image) == "" {
		return ""
	}
	u, err := url.Parse(image)
	if err != nil {
		return ""
	}
	if b, err := url.Parse(base); err == nil {
		u = b.ResolveReference(u)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return u.String()
}

func articleContent(content string) (string, string) {
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return "", ""
	}
	var text strings.Builder
	image := ""
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript":
				return
			case "img":
				if image == "" {
					for _, a := range n.Attr {
						if a.Key == "src" {
							image = a.Val
						}
					}
				}
			case "p", "div", "br", "li", "h1", "h2", "h3", "blockquote":
				text.WriteString("\n")
			}
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "li", "h1", "h2", "h3", "blockquote":
				text.WriteString("\n")
			}
		}
	}
	walk(root)
	var paragraphs []string
	for _, line := range strings.Split(text.String(), "\n") {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			paragraphs = append(paragraphs, line)
		}
	}
	return strings.Join(paragraphs, "\n\n"), image
}
