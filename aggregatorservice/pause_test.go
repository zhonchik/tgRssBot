package aggregatorservice

import (
	"errors"
	"path/filepath"
	"testing"

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
	checkList("paused, 0 queued")
	first := types.Message{ChatID: 1, FeedURL: feed, URL: "a", Title: "One", Text: "Article", ImageURL: "photo"}
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
	checkList("paused, 2 queued")
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
	checkList("paused, 2 queued")
	if err := s.SetPaused(1, feed, false); err != nil {
		t.Fatal(err)
	}
	checkList("active")
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
