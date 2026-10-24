package main

import (
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
