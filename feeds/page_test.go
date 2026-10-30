package feeds

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tgRssBot/types"
)

func TestArticlePageBody(t *testing.T) {
	first := strings.Repeat("The opening paragraph explains the discovery, its context, and the evidence. ", 8)
	last := strings.Repeat("The final paragraph discusses limitations and further research. ", 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/story", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Page headline</title><meta property="og:image" content="/hero.jpg"></head><body><nav>Navigation junk</nav><article><h1>Page headline</h1><p>` + first + `</p><p>` + last + `</p></article><footer>Footer junk</footer></body></html>`))
	}))
	defer server.Close()
	original := types.Message{ChatID: 42, URL: server.URL + "/redirect", Title: "Feed headline", Text: "Short summary"}
	got, err := fetchArticle(server.Client(), original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, strings.TrimSpace(first)) || !strings.Contains(got.Text, strings.TrimSpace(last)) || strings.Contains(got.Text, "Navigation junk") || strings.Contains(got.Text, "Footer junk") {
		t.Fatalf("wrong article body: %s", got.Text)
	}
	if got.URL != original.URL || got.ChatID != original.ChatID || got.Title != original.Title || got.ImageURL != server.URL+"/hero.jpg" {
		t.Fatalf("wrong metadata: %+v", got)
	}
}

func TestArticlePageFallback(t *testing.T) {
	for _, mode := range []string{"status", "type", "empty", "large", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "status":
					w.WriteHeader(http.StatusForbidden)
				case "type":
					w.Header().Set("Content-Type", "application/pdf")
					_, _ = w.Write([]byte("document"))
				case "empty":
					w.Header().Set("Content-Type", "text/html")
					_, _ = w.Write([]byte("<html><body></body></html>"))
				case "large":
					w.Header().Set("Content-Type", "text/html")
					_, _ = w.Write([]byte(strings.Repeat("x", maxArticleBytes+1)))
				case "timeout":
					time.Sleep(30 * time.Millisecond)
				}
			}))
			defer server.Close()
			client := server.Client()
			if mode == "timeout" {
				client.Timeout = time.Millisecond
			}
			original := types.Message{ChatID: 42, URL: server.URL, Title: "Headline", Text: "Feed summary", ImageURL: "https://example.com/feed.jpg"}
			got, err := fetchArticle(client, original)
			if err == nil || got != original {
				t.Fatalf("expected unchanged fallback, got %+v, %v", got, err)
			}
		})
	}
}
