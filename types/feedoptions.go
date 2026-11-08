package types

import "time"

type FeedOptions struct {
	FeedUrl       string
	Settings      map[int64]SubscriptionSettings
	Chats         map[int64]bool
	CheckInterval time.Duration
}

type SubscriptionSettings struct {
	Calendar      string
	CalendarAfter time.Time
	Paused        bool
	Minutes       int
	MaxItems      int
	Keyword       string
}

func (f *FeedOptions) ForChat(id int64) SubscriptionSettings {
	s := f.Settings[id]
	if s.Minutes == 0 {
		s.Minutes = 1
	}
	return s
}
