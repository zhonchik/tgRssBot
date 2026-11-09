package aggregatorservice

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tgRssBot/feeds"
	"tgRssBot/storage"
	"tgRssBot/types"
)

func TestPausedQueueSurvivesRestartAndSendFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	open := func() *storage.Storage {
		db, err := storage.NewStorage(storage.Options{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	const feed = "https://example.com/rss"
	sub := storage.Subscription{Feed: feed, Chat: 1}
	db := open()
	s := &AggregatorService{storage: db, feeds: map[string]*feeds.FeedReader{
		feed: feeds.NewFeedReader(&types.FeedOptions{FeedUrl: feed, Chats: map[int64]bool{1: true}}),
	}}
	if err := s.SetPaused(1, feed, true); err != nil {
		t.Fatal(err)
	}
	checkList := func(want string) {
		t.Helper()
		got, err := s.GetFeedList(1)
		if err != nil || got != "Subscribed:\n"+feed+" ("+want+")" {
			t.Fatalf("unexpected subscription list: %q, %v", got, err)
		}
	}
	checkList("paused, 0 queued; reason: manual pause")
	first := types.Message{ReceivedAt: time.Now().UTC(), ChatID: 1, FeedURL: feed, URL: "a", Title: "One", Text: "Article", ImageURL: "photo"}
	second := first
	second.URL = "b"
	for _, m := range []types.Message{first, second, first} {
		if err := db.Enqueue(m); err != nil {
			t.Fatal(err)
		}
	}
	if sent, err := s.deliverOne(sub); sent || err != nil {
		t.Fatal(sent, err)
	}
	checkList("paused, 2 queued; reason: manual pause; no delivery attempt since restart")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s.storage = open()
	defer s.storage.Close()
	saved, err := s.storage.GetFeeds()
	if err != nil || len(saved) != 1 || !saved[0].ForChat(1).Paused {
		t.Fatal(saved, err)
	}
	s.feeds[feed] = feeds.NewFeedReader(saved[0])
	checkList("paused, 2 queued; reason: manual pause; no delivery attempt since restart")
	if err := s.SetPaused(1, feed, false); err != nil {
		t.Fatal(err)
	}
	checkList("active, 2 queued; no delivery attempt since restart")
	s.sendMessage = func(types.Message) error { return errors.New("offline") }
	if _, err := s.deliverOne(sub); err == nil {
		t.Fatal("expected send failure")
	}
	for _, want := range []types.Message{first, second} {
		s.sendMessage = func(m types.Message) error {
			if m != want {
				t.Fatalf("got %+v, want %+v", m, want)
			}
			return nil
		}
		if sent, err := s.deliverOne(sub); !sent || err != nil {
			t.Fatal(sent, err)
		}
	}
	if sent, err := s.deliverOne(sub); sent || err != nil {
		t.Fatal(sent, err)
	}
}

func TestPauseDuringArticleFetch(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte("<html><body><p>Article body</p></body></html>"))
	}))
	defer server.Close()
	defer unblock()
	db, err := storage.NewStorage(storage.Options{Path: filepath.Join(t.TempDir(), "queue.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const feed = "https://example.com/rss"
	sub := storage.Subscription{Feed: feed, Chat: 1}
	var sent atomic.Bool
	s := &AggregatorService{storage: db, sendMessage: func(types.Message) error { sent.Store(true); return nil }, feeds: map[string]*feeds.FeedReader{
		feed: feeds.NewFeedReader(&types.FeedOptions{FeedUrl: feed, Chats: map[int64]bool{1: true}}),
	}}
	if err := db.Enqueue(types.Message{ChatID: 1, FeedURL: feed, URL: server.URL}); err != nil {
		t.Fatal(err)
	}
	delivered := make(chan error, 1)
	go func() { _, err := s.deliverOne(sub); delivered <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("article fetch did not start")
	}
	paused := make(chan error, 1)
	go func() { paused <- s.SetPaused(1, feed, true) }()
	select {
	case err := <-paused:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		unblock()
		<-delivered
		<-paused
		t.Fatal("pause blocked on article fetch")
	}
	unblock()
	if err := <-delivered; err != nil {
		t.Fatal(err)
	}
	if sent.Load() {
		t.Fatal("article sent after pause")
	}
	if count, err := db.QueueCount(sub); err != nil || count != 1 {
		t.Fatalf("queue count = %d, error = %v", count, err)
	}
}
