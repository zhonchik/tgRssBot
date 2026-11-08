package types

import "time"

type Calendar struct {
	Summaries map[string]string
	Starts    []time.Time
	Updated   time.Time
}
type Chat struct {
	Calendars map[string]Calendar
}
