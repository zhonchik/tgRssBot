package aggregatorservice

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"tgRssBot/feeds"
	"tgRssBot/storage"
	"tgRssBot/types"
	"time"
)

func TestDeliverySharesTurnsBetweenSubscriptions(t *testing.T) {
	db, err := storage.NewStorage(storage.Options{Path: filepath.Join(t.TempDir(), "queue.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	as := &AggregatorService{storage: db, feeds: make(map[string]*feeds.FeedReader), wake: make(chan struct{}, 1), done: make(chan struct{})}
	for _, feed := range []string{"https://example.com/a", "https://example.com/b"} {
		as.feeds[feed] = feeds.NewFeedReader(&types.FeedOptions{FeedUrl: feed, Chats: map[int64]bool{1: true}})
	}
	for _, m := range []types.Message{{ChatID: 1, FeedURL: "https://example.com/a", URL: "a1"}, {ChatID: 1, FeedURL: "https://example.com/a", URL: "a2"}, {ChatID: 1, FeedURL: "https://example.com/b", URL: "b1"}} {
		if err := db.Enqueue(m); err != nil {
			t.Fatal(err)
		}
	}
	sent := make(chan string, 3)
	as.sendMessage = func(m types.Message) error { sent <- m.FeedURL; return nil }
	as.workers.Add(1)
	go as.deliver()
	as.signalDelivery()
	defer func() { close(as.done); as.workers.Wait() }()
	for i, want := range []string{"https://example.com/a", "https://example.com/b", "https://example.com/a"} {
		select {
		case got := <-sent:
			if got != want {
				t.Fatalf("delivery %d: got %s, want %s; one backlog delayed other subscriptions", i, got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("delivery stalled")
		}
	}
}

func TestDeliveryStatusReportsFailureWithoutLeakingCredentials(t *testing.T) {
	db, err := storage.NewStorage(storage.Options{Path: filepath.Join(t.TempDir(), "status.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const feed = "https://example.com/rss"
	as := &AggregatorService{storage: db, feeds: map[string]*feeds.FeedReader{feed: feeds.NewFeedReader(&types.FeedOptions{FeedUrl: feed, Chats: map[int64]bool{1: true}})}}
	if err := db.Enqueue(types.Message{ChatID: 1, FeedURL: feed, URL: "article"}); err != nil {
		t.Fatal(err)
	}
	as.sendMessage = func(types.Message) error {
		return errors.New("telegram unknown: PHOTO_INVALID_DIMENSIONS (400); https://api.telegram.org/bot123456:SECRET/sendPhoto token 123456:SECRET")
	}
	if sent, err := as.deliverOne(storage.Subscription{Feed: feed, Chat: 1}); sent || err == nil {
		t.Fatal(sent, err)
	}
	status, err := as.GetFeedList(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"active, 1 queued", "last attempt", "sending to Telegram", "PHOTO_INVALID_DIMENSIONS"} {
		if !strings.Contains(status, want) {
			t.Fatal(status)
		}
	}
	if strings.Contains(status, "SECRET") || strings.Contains(status, "api.telegram.org") {
		t.Fatal("credentials leaked")
	}
	other, err := as.GetFeedList(2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(other, "PHOTO_INVALID_DIMENSIONS") {
		t.Fatal("other chat saw delivery error")
	}
	as.sendMessage = func(types.Message) error { return nil }
	if sent, err := as.deliverOne(storage.Subscription{Feed: feed, Chat: 1}); !sent || err != nil {
		t.Fatal(sent, err)
	}
	status, err = as.GetFeedList(1)
	if err != nil || strings.Contains(status, "queued") {
		t.Fatal(status, err)
	}
}
