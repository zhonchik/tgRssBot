package feeds

import (
	"encoding/json"
	"github.com/mmcdole/gofeed"
	"testing"
	"tgRssBot/types"
)

func TestArticleFromFeed(t *testing.T) {
	item := &gofeed.Item{Title: "Title", Link: "https://example.com/news/story", Content: `<p>First &amp; visible.</p><script>bad()</script><p>Remaining <b>text</b>.</p><img src="../hero.jpg">`, Description: "fallback"}
	m := articleMessage(item)
	if m.Text != "First & visible.\n\nRemaining text." || m.ImageURL != "https://example.com/hero.jpg" {
		t.Fatalf("%+v", m)
	}
	item.Image = &gofeed.Image{URL: "https://example.com/main.jpg"}
	if articleMessage(item).ImageURL != item.Image.URL {
		t.Fatal("main image priority")
	}
	item.Content = ""
	if articleMessage(item).Text != "fallback" {
		t.Fatal("description fallback")
	}
	m.ChatID = 42
	old, _ := json.Marshal(types.Message{ChatID: 42, URL: item.Link})
	rich, _ := json.Marshal(m)
	if string(old) != string(rich) {
		t.Fatalf("deduplication key changed: %s != %s", old, rich)
	}
}
