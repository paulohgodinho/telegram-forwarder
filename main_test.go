package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
)

func TestPeerDisplayName(t *testing.T) {
	cases := []struct {
		name string
		peer storage.Peer
		want string
	}{
		{
			name: "channel with title",
			peer: storage.Peer{
				Key:     dialogs.DialogKey{Kind: dialogs.Channel, ID: 3195745015},
				Channel: &tg.Channel{ID: 3195745015, Title: "My Cool Channel"},
			},
			want: "My Cool Channel",
		},
		{
			name: "channel with no title falls back to username",
			peer: storage.Peer{
				Key:     dialogs.DialogKey{Kind: dialogs.Channel, ID: 42},
				Channel: &tg.Channel{ID: 42, Username: "coolchannel"},
			},
			want: "@coolchannel",
		},
		{
			name: "channel with no title or username falls back to debug string",
			peer: storage.Peer{
				Key:     dialogs.DialogKey{Kind: dialogs.Channel, ID: 42},
				Channel: &tg.Channel{ID: 42},
			},
			want: "Channel(42)[Channel]",
		},
		{
			name: "basic chat with title",
			peer: storage.Peer{
				Key:  dialogs.DialogKey{Kind: dialogs.Chat, ID: 7},
				Chat: &tg.Chat{ID: 7, Title: "Family Group"},
			},
			want: "Family Group",
		},
		{
			name: "user with first and last name",
			peer: storage.Peer{
				Key:  dialogs.DialogKey{Kind: dialogs.User, ID: 99},
				User: &tg.User{ID: 99, FirstName: "Jane", LastName: "Doe", Username: "janedoe"},
			},
			want: "Jane Doe",
		},
		{
			name: "user with only username",
			peer: storage.Peer{
				Key:  dialogs.DialogKey{Kind: dialogs.User, ID: 100},
				User: &tg.User{ID: 100, Username: "ghostuser"},
			},
			want: "@ghostuser",
		},
		{
			name: "user with no name info falls back to debug string",
			peer: storage.Peer{
				Key:  dialogs.DialogKey{Kind: dialogs.User, ID: 101},
				User: &tg.User{ID: 101},
			},
			want: "User(101)[User]",
		},
		{
			name: "forbidden channel (no Channel entity) falls back to debug string",
			peer: storage.Peer{
				Key: dialogs.DialogKey{Kind: dialogs.Channel, ID: 55},
			},
			want: "Channel(55)[]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := peerDisplayName(tc.peer)
			if got != tc.want {
				t.Fatalf("peerDisplayName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "shorter than limit", s: "hello", n: 40, want: "hello"},
		{name: "exactly at limit", s: "01234567890123456789012345678901234567890123456789"[:40], n: 40, want: "01234567890123456789012345678901234567890123456789"[:40]},
		{name: "longer than limit", s: "0123456789012345678901234567890123456789extra", n: 40, want: "0123456789012345678901234567890123456789"},
		{name: "multi-byte runes not split", s: repeatEmoji(45), n: 40, want: repeatEmoji(40)},
		{name: "empty string", s: "", n: 40, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.s, tc.n)
			if got != tc.want {
				t.Fatalf("truncateRunes(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
		})
	}
}

// repeatEmoji builds n repetitions of a multi-byte rune, used to verify
// truncateRunes counts runes, not bytes.
func repeatEmoji(n int) string {
	r := make([]rune, n)
	for i := range r {
		r[i] = '🙂'
	}
	return string(r)
}

func TestLogPanicWritesStructuredEntry(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logPanic(logger, "some operation", "boom")

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["level"] != "ERROR" {
		t.Fatalf("expected ERROR level, got %v", decoded["level"])
	}
	if decoded["where"] != "some operation" {
		t.Fatalf("expected where=%q, got %v", "some operation", decoded["where"])
	}
	if decoded["panic"] != "boom" {
		t.Fatalf("expected panic=%q, got %v", "boom", decoded["panic"])
	}
	stack, _ := decoded["stack"].(string)
	if !strings.Contains(stack, "goroutine") {
		t.Fatalf("expected a stack trace, got: %q", stack)
	}
}

func TestSafeHandleMessageRecoversPanicAndReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	prevDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prevDefault) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// A nil *tg.Message makes handleMessage panic on msg.Out, proving a
	// malformed/unexpected message is contained instead of crashing the
	// whole process.
	err := safeHandleMessage(context.Background(), tg.Entities{}, nil)
	if err != nil {
		t.Fatalf("expected recovered panic to yield nil error, got: %v", err)
	}

	var decoded map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &decoded); jsonErr != nil {
		t.Fatalf("expected a panic log entry, output not valid JSON: %v\n%s", jsonErr, buf.String())
	}
	if decoded["where"] != "handling message" {
		t.Fatalf("expected where=%q, got %v", "handling message", decoded["where"])
	}
}
