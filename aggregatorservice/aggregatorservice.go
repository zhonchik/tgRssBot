package aggregatorservice

import (
	"errors"
	"github.com/PuerkitoBio/purell"
	log "github.com/sirupsen/logrus"
	"net/url"
	"strings"
	"sync"
	"tgRssBot/feeds"
	"tgRssBot/storage"
	"tgRssBot/types"
	"time"
)

type (
	AggregatorService struct {
		mu           sync.Mutex
		sendMessage  func(message types.Message) error
		storage      *storage.Storage
		chats        map[int64]*types.Chat
		feeds        map[string]*feeds.FeedReader
		wake         chan struct{}
		done         chan struct{}
		workers      sync.WaitGroup
		messageQueue chan types.Message
	}
)

func NewAggregatorService(
	messageSender func(message types.Message) error,
	st *storage.Storage,
) (*AggregatorService, error) {
	messageQueue := make(chan types.Message)

	as := &AggregatorService{
		sendMessage:  messageSender,
		storage:      st,
		chats:        make(map[int64]*types.Chat),
		feeds:        make(map[string]*feeds.FeedReader),
		messageQueue: messageQueue, wake: make(chan struct{}, 1), done: make(chan struct{}),
	}

	feedList, err := st.GetFeeds()
	if err != nil {
		return nil, err
	}
	for _, feed := range feedList {
		feedUrl, err := purell.NormalizeURLString(feed.FeedUrl, purell.FlagsUsuallySafeGreedy)
		if err != nil {
			continue
		}
		feed.FeedUrl = feedUrl
		as.addReader(feed)
	}
	as.chats, err = st.GetChats()
	if err != nil {
		log.Warnf("Failed to read chats: %s", err)
	}

	as.workers.Add(2)
	go as.processMessages()
	go as.deliver()
	as.signalDelivery()

	return as, nil
}

func (as *AggregatorService) Close() error {
	log.Info("Closing service")
	close(as.done)
	as.workers.Wait()
	return as.storage.Close()
}

func (as *AggregatorService) Start(chatID int64) (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	chat := as.chats[chatID]
	if chat != nil {
		return "", errors.New("i know you")
	}
	chat = &types.Chat{}
	as.chats[chatID] = chat
	err := as.storage.SaveChat(chatID, chat)
	if err != nil {
		return "", err
	}
	return "you're welcome", nil
}

func (as *AggregatorService) TrySubscribe(chatID int64, feedUrl string) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	feedUrl, err := purell.NormalizeURLString(feedUrl, purell.FlagsUsuallySafeGreedy)
	if err != nil {
		return errors.New("wrong url")
	}
	u, e := url.Parse(feedUrl)
	if e != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("enter an HTTP or HTTPS feed URL")
	}
	reader := as.feeds[feedUrl]
	if reader == nil {
		feedOptions := &types.FeedOptions{
			FeedUrl:       feedUrl,
			Chats:         map[int64]bool{},
			CheckInterval: time.Minute,
		}
		reader = feeds.NewFeedReader(feedOptions)

	}

	reader.Mu.Lock()
	defer reader.Mu.Unlock()
	if reader.Options.Chats[chatID] {
		return nil
	}
	items, err := reader.GetItems()
	if err == nil {
		for _, item := range items {
			message := types.Message{ChatID: chatID, URL: item}
			_ = as.storage.MarkAsProcessed(message)
		}
	} else {
		return errors.New("unsupported feed format")
	}

	reader.Options.Chats[chatID] = true
	err = as.storage.SaveFeed(reader.Options)
	if err != nil {
		delete(reader.Options.Chats, chatID)
		return err
	}
	as.feeds[feedUrl] = reader
	if !reader.IsRunning {
		reader.IsRunning = true
		go reader.Run(as.messageQueue)
	}
	return nil
}

func (as *AggregatorService) TryUnsubscribe(chatID int64, feedUrl string) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	feedUrl, err := purell.NormalizeURLString(feedUrl, purell.FlagsUsuallySafeGreedy)
	if err != nil {
		return errors.New("wrong url")
	}
	reader := as.feeds[feedUrl]
	if reader == nil {
		return errors.New("unknown feed")
	}
	reader.Mu.Lock()
	defer reader.Mu.Unlock()
	if !reader.Options.Chats[chatID] {
		return errors.New("was not subscribed")
	}
	old := reader.Options.ForChat(chatID)
	delete(reader.Options.Settings, chatID)
	delete(reader.Options.Chats, chatID)
	err = as.storage.RemoveSubscription(reader.Options, chatID)
	if err != nil {
		reader.Options.Chats[chatID] = true
		if reader.Options.Settings == nil {
			reader.Options.Settings = make(map[int64]types.SubscriptionSettings)
		}
		reader.Options.Settings[chatID] = old
		return err
	}
	return nil
}

func (as *AggregatorService) GetFeedList(chatID int64) (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	var lines []string
	var subscribed []string
	var suggestions []string

	for feedUrl, reader := range as.feeds {
		if reader.Options.Chats[chatID] {
			subscribed = append(subscribed, feedUrl)
		} else {
			suggestions = append(suggestions, feedUrl)
		}
	}

	if len(subscribed) > 0 {
		lines = append(lines, "Subscribed:")
		lines = append(lines, subscribed...)
	}

	if len(suggestions) > 0 {
		lines = append(lines, "Suggestions:")
		lines = append(lines, suggestions...)
	}
	text := strings.Join(lines, "\n")
	if text == "" {
		text = "No subscriptions yet. Try to /subscribe"
	}
	return text, nil
}

func (as *AggregatorService) CheckChat(chatID int64) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.chats[chatID] == nil {
		return errors.New("I don't know you. Try /start")
	}
	return nil
}

func (as *AggregatorService) addReader(feedOptions *types.FeedOptions) {
	fr := feeds.NewFeedReader(feedOptions)
	fr.IsRunning = true
	go fr.Run(as.messageQueue)
	as.feeds[feedOptions.FeedUrl] = fr
}

func (as *AggregatorService) processMessages() {
	defer as.workers.Done()
	for {
		select {
		case <-as.done:
			return
		case m := <-as.messageQueue:
			as.mu.Lock()
			r := as.feeds[m.FeedURL]
			if r != nil && r.Options.Chats[m.ChatID] {
				if err := as.storage.Enqueue(m); err != nil {
					log.Warn(err)
				} else if !r.Options.ForChat(m.ChatID).Paused {
					as.signalDelivery()
				}
			}
			as.mu.Unlock()
		}
	}
}

func (as *AggregatorService) signalDelivery() {
	select {
	case as.wake <- struct{}{}:
	default:
	}
}

func (as *AggregatorService) deliver() {
	defer as.workers.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-as.done:
			return
		case <-ticker.C:
		case <-as.wake:
		}
		subs, err := as.storage.Queues()
		if err != nil {
			log.Warn(err)
			continue
		}
		for _, sub := range subs {
			for {
				select {
				case <-as.done:
					return
				default:
				}
				more, err := as.deliverOne(sub)
				if err != nil {
					log.Warn(err)
				}
				if err != nil || !more {
					break
				}
			}
		}
	}
}

func (as *AggregatorService) deliverOne(sub storage.Subscription) (bool, error) {
	// Serialize the final pause check and send with subscription changes.
	as.mu.Lock()
	defer as.mu.Unlock()
	r := as.feeds[sub.Feed]
	if r == nil || !r.Options.Chats[sub.Chat] || r.Options.ForChat(sub.Chat).Paused {
		return false, nil
	}
	m, seq, err := as.storage.First(sub)
	if err != nil || seq == 0 {
		return false, err
	}
	if err := as.sendMessage(feeds.EnrichArticle(m)); err != nil {
		return false, err
	}
	return true, as.storage.Acknowledge(sub, seq)
}

func (as *AggregatorService) Subscriptions(id int64) map[string]types.SubscriptionSettings {
	as.mu.Lock()
	defer as.mu.Unlock()
	result := make(map[string]types.SubscriptionSettings)
	for url, r := range as.feeds {
		if r.Options.Chats[id] {
			result[url] = r.Options.ForChat(id)
		}
	}
	return result
}
func (as *AggregatorService) Update(id int64, url string, s types.SubscriptionSettings) error {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.updateLocked(id, url, s)
}

func (as *AggregatorService) SetPaused(id int64, url string, paused bool) error {
	url, err := purell.NormalizeURLString(url, purell.FlagsUsuallySafeGreedy)
	if err != nil {
		return errors.New("wrong url")
	}
	as.mu.Lock()
	defer as.mu.Unlock()
	r := as.feeds[url]
	if r == nil || !r.Options.Chats[id] {
		return errors.New("subscription missing")
	}
	s := r.Options.ForChat(id)
	s.Paused = paused
	return as.updateLocked(id, url, s)
}

func (as *AggregatorService) updateLocked(id int64, url string, s types.SubscriptionSettings) error {
	r := as.feeds[url]
	if r == nil || !r.Options.Chats[id] {
		return errors.New("subscription missing")
	}
	if s.Minutes < 1 || s.Minutes > 1440 || s.MaxItems < 0 || s.MaxItems > 100 || len([]rune(s.Keyword)) > 120 {
		return errors.New("invalid settings")
	}
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if r.Options.Settings == nil {
		r.Options.Settings = make(map[int64]types.SubscriptionSettings)
	}
	old := r.Options.ForChat(id)
	r.Options.Settings[id] = s
	if err := as.storage.SaveFeed(r.Options); err != nil {
		r.Options.Settings[id] = old
		return err
	}
	if !s.Paused {
		as.signalDelivery()
	}
	return nil
}
