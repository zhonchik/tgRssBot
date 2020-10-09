package storage

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
	"tgRssBot/types"
	"time"
)

type (
	Options struct {
		Path string
	}

	Storage struct {
		options Options
		db      *bolt.DB
	}
)

func NewStorage(options Options) (*Storage, error) {
	db, err := bolt.Open(options.Path, 0666, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("Feeds"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		_, err = tx.CreateBucketIfNotExists([]byte("ProcessedMessages"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	storage := &Storage{
		options: options,
		db:      db,
	}
	return storage, nil
}

func (storage *Storage) Close() error {
	log.Info("Closing storage")
	return storage.db.Close()
}

func (storage *Storage) IsProcessed(message types.Message) bool {
	var result bool
	err := storage.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedMessages"))
		key, err := json.Marshal(message)
		if err != nil {
			return err
		}
		result = b.Get(key) != nil
		return nil
	})
	if err != nil {
		return false
	}
	return result
}

func (storage *Storage) MarkAsProcessed(message types.Message) error {
	err := storage.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("ProcessedMessages"))
		key, err := json.Marshal(message)
		if err != nil {
			return err
		}
		err = b.Put(key, []byte{1})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (storage *Storage) GetFeeds() ([]*types.FeedOptions, error) {
	var feeds []*types.FeedOptions
	err := storage.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Feeds"))
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			feed := &types.FeedOptions{}
			err := json.Unmarshal(v, feed)
			if err != nil {
				return err
			}
			feeds = append(feeds, feed)
		}
		return nil
	})
	return feeds, err
}

func (storage *Storage) SaveFeed(feed *types.FeedOptions) error {
	err := storage.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("Feeds"))
		key := []byte(feed.FeedUrl)
		value, err := json.Marshal(feed)
		if err != nil {
			return err
		}
		err = b.Put(key, value)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
