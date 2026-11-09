package aggregatorservice

import (
	"fmt"
	"regexp"
	"strings"
	"tgRssBot/storage"
	"time"
)

type deliveryStatus struct {
	Attempt time.Time
	Stage   string
}

var diagnosticURL = regexp.MustCompile(`https?://[^\s]+`)
var diagnosticToken = regexp.MustCompile(`[0-9]+:[A-Za-z0-9_-]+`)

func safeDeliveryError(err error) string {
	text := diagnosticURL.ReplaceAllString(err.Error(), "[URL redacted]")
	text = diagnosticToken.ReplaceAllString(text, "[token redacted]")
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 240 {
		text = string(runes[:240]) + "…"
	}
	return text
}

func (as *AggregatorService) recordDelivery(sub storage.Subscription, stage string, err error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.delivery == nil {
		as.delivery = make(map[storage.Subscription]deliveryStatus)
	}
	previous := as.delivery[sub]
	if previous.Attempt.IsZero() || stage == "fetching article" {
		previous.Attempt = time.Now().UTC()
	}
	previous.Stage = stage
	if err != nil {
		previous.Stage += ": " + safeDeliveryError(err)
	}
	as.delivery[sub] = previous
}

func (as *AggregatorService) deliveryDescriptionLocked(sub storage.Subscription) string {
	state, ok := as.delivery[sub]
	if !ok {
		return "no delivery attempt since restart"
	}
	return fmt.Sprintf("last attempt %s: %s", state.Attempt.Format("2006-01-02 15:04:05 UTC"), state.Stage)
}
