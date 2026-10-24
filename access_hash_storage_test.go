package main

import (
	"context"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func newTestBoltAccessHasher(t *testing.T) *boltAccessHasher {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), "test.bolt.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open bolt db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return newBoltAccessHasher(db)
}

func TestBoltAccessHasherChannelRoundTrip(t *testing.T) {
	h := newTestBoltAccessHasher(t)
	ctx := context.Background()

	if _, found, err := h.GetChannelAccessHash(ctx, 1, 100); err != nil || found {
		t.Fatalf("expected not found before Set, got found=%v err=%v", found, err)
	}

	if err := h.SetChannelAccessHash(ctx, 1, 100, 999); err != nil {
		t.Fatalf("SetChannelAccessHash: %v", err)
	}

	hash, found, err := h.GetChannelAccessHash(ctx, 1, 100)
	if err != nil || !found {
		t.Fatalf("expected found=true after Set, got found=%v err=%v", found, err)
	}
	if hash != 999 {
		t.Fatalf("hash = %d, want 999", hash)
	}
}

func TestBoltAccessHasherUserRoundTrip(t *testing.T) {
	h := newTestBoltAccessHasher(t)
	ctx := context.Background()

	if err := h.SetUserAccessHash(ctx, 1, 42, 777); err != nil {
		t.Fatalf("SetUserAccessHash: %v", err)
	}

	hash, found, err := h.GetUserAccessHash(ctx, 1, 42)
	if err != nil || !found {
		t.Fatalf("expected found=true, got found=%v err=%v", found, err)
	}
	if hash != 777 {
		t.Fatalf("hash = %d, want 777", hash)
	}
}

func TestBoltAccessHasherSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bolt.db")
	ctx := context.Background()

	db1, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open bolt db: %v", err)
	}
	h1 := newBoltAccessHasher(db1)
	if err := h1.SetChannelAccessHash(ctx, 1, 100, 999); err != nil {
		t.Fatalf("SetChannelAccessHash: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close db1: %v", err)
	}

	// Simulate a process restart: reopen the same file with a fresh handler.
	db2, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("reopen bolt db: %v", err)
	}
	t.Cleanup(func() { db2.Close() })
	h2 := newBoltAccessHasher(db2)

	hash, found, err := h2.GetChannelAccessHash(ctx, 1, 100)
	if err != nil || !found {
		t.Fatalf("expected hash to survive reopen, got found=%v err=%v", found, err)
	}
	if hash != 999 {
		t.Fatalf("hash = %d, want 999", hash)
	}
}

func TestBoltAccessHasherDistinctUsersAndTargets(t *testing.T) {
	h := newTestBoltAccessHasher(t)
	ctx := context.Background()

	if err := h.SetChannelAccessHash(ctx, 1, 100, 111); err != nil {
		t.Fatalf("SetChannelAccessHash: %v", err)
	}
	if err := h.SetChannelAccessHash(ctx, 2, 100, 222); err != nil {
		t.Fatalf("SetChannelAccessHash: %v", err)
	}

	hash1, _, _ := h.GetChannelAccessHash(ctx, 1, 100)
	hash2, _, _ := h.GetChannelAccessHash(ctx, 2, 100)
	if hash1 != 111 || hash2 != 222 {
		t.Fatalf("expected per-user isolation, got hash1=%d hash2=%d", hash1, hash2)
	}
}
