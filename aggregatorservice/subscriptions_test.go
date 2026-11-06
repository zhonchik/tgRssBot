package aggregatorservice

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"tgRssBot/storage"
	"tgRssBot/types"
)

func TestIndependentSubscriptions(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<rss version=\"2.0\"><channel><title>Test</title></channel></rss>"))
	}))
	defer source.Close()
	db, err := storage.NewStorage(storage.Options{Path: filepath.Join(t.TempDir(), "storage.db")})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewAggregatorService(func(types.Message) error { return nil }, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, id := range []int64{1, 2} {
		if err := s.TrySubscribe(id, source.URL); err != nil {
			t.Fatal(err)
		}
	}
	var url string
	for url = range s.Subscriptions(1) {
	}
	custom := types.SubscriptionSettings{Paused: true, Minutes: 15, MaxItems: 2, Keyword: "science"}
	if err := s.Update(1, url, custom); err != nil {
		t.Fatal(err)
	}
	saved, err := db.GetFeeds()
	if err != nil || len(saved) != 1 || saved[0].ForChat(1) != custom || saved[0].ForChat(2) != (types.SubscriptionSettings{Minutes: 1}) {
		t.Fatal("settings were not persisted independently", err)
	}
	if err := s.TryUnsubscribe(1, url); err != nil {
		t.Fatal(err)
	}
	if len(s.Subscriptions(1)) != 0 || len(s.Subscriptions(2)) != 1 {
		t.Fatal("removal affected wrong subscriptions")
	}
}
