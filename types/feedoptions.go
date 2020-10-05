package types

import "time"

type FeedOptions struct {
	FeedUrl       string
	Chats         map[int64]bool
	CheckInterval time.Duration
}
