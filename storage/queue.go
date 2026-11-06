package storage

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"

	bolt "go.etcd.io/bbolt"
	"tgRssBot/types"
)

type Subscription struct {
	Feed string
	Chat int64
}

var queueBucket = []byte("DeliveryQueues")

// Enqueue records ownership of the article and its full payload atomically.
// ProcessedMessages keeps its historical chat/URL key format.
func (s *Storage) Enqueue(m types.Message) error {
	sub, err := json.Marshal(Subscription{m.FeedURL, m.ChatID})
	if err != nil {
		return err
	}
	key, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		seen := tx.Bucket([]byte("ProcessedMessages"))
		if seen.Get(key) != nil {
			return nil
		}
		q, err := tx.Bucket(queueBucket).CreateBucketIfNotExists(sub)
		if err != nil {
			return err
		}
		n, err := q.NextSequence()
		if err != nil {
			return err
		}
		var data bytes.Buffer
		if err := gob.NewEncoder(&data).Encode(m); err != nil {
			return err
		}
		if err := q.Put(binary.BigEndian.AppendUint64(nil, n), data.Bytes()); err != nil {
			return err
		}
		return seen.Put(key, []byte{1})
	})
}

func (s *Storage) Queues() ([]Subscription, error) {
	var subs []Subscription
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(queueBucket).ForEach(func(k, _ []byte) error {
			var sub Subscription
			if err := json.Unmarshal(k, &sub); err != nil {
				return err
			}
			subs = append(subs, sub)
			return nil
		})
	})
	return subs, err
}

// QueueCount returns the number of messages awaiting delivery for a subscription.
func (s *Storage) QueueCount(sub Subscription) (int, error) {
	key, err := json.Marshal(sub)
	if err != nil {
		return 0, err
	}
	count := 0
	err = s.db.View(func(tx *bolt.Tx) error {
		if q := tx.Bucket(queueBucket).Bucket(key); q != nil {
			count = q.Stats().KeyN
		}
		return nil
	})
	return count, err
}

func (s *Storage) First(sub Subscription) (types.Message, uint64, error) {
	var m types.Message
	var seq uint64
	key, err := json.Marshal(sub)
	if err != nil {
		return m, 0, err
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		q := tx.Bucket(queueBucket).Bucket(key)
		if q == nil {
			return nil
		}
		k, data := q.Cursor().First()
		if k == nil {
			return nil
		}
		seq = binary.BigEndian.Uint64(k)
		return gob.NewDecoder(bytes.NewReader(data)).Decode(&m)
	})
	return m, seq, err
}

func (s *Storage) Acknowledge(sub Subscription, seq uint64) error {
	key, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket(queueBucket)
		q := root.Bucket(key)
		if q == nil {
			return nil
		}
		if err := q.Delete(binary.BigEndian.AppendUint64(nil, seq)); err != nil {
			return err
		}
		if k, _ := q.Cursor().First(); k == nil {
			return root.DeleteBucket(key)
		}
		return nil
	})
}
