package main

import (
	"context"
	"encoding/binary"

	bolt "go.etcd.io/bbolt"
)

// channelHashBucket and userHashBucket hold persisted Telegram access hashes.
var (
	channelHashBucket = []byte("channel_access_hashes")
	userHashBucket    = []byte("user_access_hashes")
)

// boltAccessHasher persists channel/user access hashes in bbolt so the
// updates manager recognizes previously-seen channels/users across restarts.
type boltAccessHasher struct {
	db *bolt.DB
}

// newBoltAccessHasher wraps db; the caller retains ownership (Close it).
func newBoltAccessHasher(db *bolt.DB) *boltAccessHasher {
	return &boltAccessHasher{db: db}
}

// hashKey builds the composite (userID, targetID) lookup key.
func hashKey(userID, targetID int64) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b[:8], uint64(userID))
	binary.BigEndian.PutUint64(b[8:], uint64(targetID))
	return b
}

func int64ToBytes(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func bytesToInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

// SetChannelAccessHash implements updates.ChannelAccessHasher.
func (h *boltAccessHasher) SetChannelAccessHash(_ context.Context, userID, channelID, accessHash int64) error {
	return h.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(channelHashBucket)
		if err != nil {
			return err
		}
		return b.Put(hashKey(userID, channelID), int64ToBytes(accessHash))
	})
}

// GetChannelAccessHash implements updates.ChannelAccessHasher.
func (h *boltAccessHasher) GetChannelAccessHash(_ context.Context, userID, channelID int64) (int64, bool, error) {
	return h.get(channelHashBucket, userID, channelID)
}

// SetUserAccessHash implements updates.UserAccessHasher.
func (h *boltAccessHasher) SetUserAccessHash(_ context.Context, userID, targetUserID, accessHash int64) error {
	return h.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(userHashBucket)
		if err != nil {
			return err
		}
		return b.Put(hashKey(userID, targetUserID), int64ToBytes(accessHash))
	})
}

// GetUserAccessHash implements updates.UserAccessHasher.
func (h *boltAccessHasher) GetUserAccessHash(_ context.Context, userID, targetUserID int64) (int64, bool, error) {
	return h.get(userHashBucket, userID, targetUserID)
}

// get reads a persisted hash from bucket, returning found=false if absent.
func (h *boltAccessHasher) get(bucket []byte, userID, targetID int64) (int64, bool, error) {
	var hash int64
	var found bool
	err := h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return nil
		}
		v := b.Get(hashKey(userID, targetID))
		if v == nil {
			return nil
		}
		hash, found = bytesToInt64(v), true
		return nil
	})
	return hash, found, err
}
