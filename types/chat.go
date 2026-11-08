package types

import "time"

type Calendar struct {
	Starts  []time.Time
	Updated time.Time
}
type Chat struct {
	Calendars map[string]Calendar
}
