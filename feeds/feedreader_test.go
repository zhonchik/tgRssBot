package feeds

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"tgRssBot/types"
)

func TestFeedItemsUsePublicationTimeInsteadOfDateText(t *testing.T) {
	// The first and last entries use F1News's date format. Alphabetical sorting
	// selects Wednesday as the newest item even when Sunday's article is newer.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss version="2.0"><channel><title>News</title>
 <item><link>https://example.com/newest</link><pubDate>Sun, 08 Nov 2026 23:17:00 +0300</pubDate></item>
 <item><link>https://example.com/middle</link><pubDate>Sun, 08 Nov 2026 19:00:00 +0000</pubDate></item>
 <item><link>https://example.com/old</link><pubDate>Wed, 04 Nov 2026 23:55:00 +0300</pubDate></item>
 </channel></rss>`)
	}))
	defer server.Close()
	reader := NewFeedReader(&types.FeedOptions{FeedUrl: server.URL})
	items, err := reader.getFeedItems()
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"https://example.com/old", "https://example.com/middle", "https://example.com/newest"} {
		if items[i].Link != want {
			t.Fatalf("item %d: got %s, want %s; item limits would select stale news", i, items[i].Link, want)
		}
	}
}
