package main

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
)

func TestLargestPhotoSize(t *testing.T) {
	cases := []struct {
		name      string
		sizes     []tg.PhotoSizeClass
		wantType  string
		wantFound bool
	}{
		{
			name: "picks the largest by area",
			sizes: []tg.PhotoSizeClass{
				&tg.PhotoSize{Type: "s", W: 100, H: 75},
				&tg.PhotoSize{Type: "x", W: 800, H: 600},
				&tg.PhotoSize{Type: "m", W: 320, H: 240},
			},
			wantType:  "x",
			wantFound: true,
		},
		{
			name: "skips non-downloadable inline sizes",
			sizes: []tg.PhotoSizeClass{
				&tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1, 2, 3}},
				&tg.PhotoPathSize{Type: "j", Bytes: []byte{4, 5, 6}},
				&tg.PhotoSize{Type: "y", W: 1280, H: 960},
			},
			wantType:  "y",
			wantFound: true,
		},
		{
			name: "handles PhotoCachedSize and PhotoSizeProgressive variants",
			sizes: []tg.PhotoSizeClass{
				&tg.PhotoCachedSize{Type: "a", W: 90, H: 90},
				&tg.PhotoSizeProgressive{Type: "w", W: 1920, H: 1080, Sizes: []int{1000, 5000, 20000}},
			},
			wantType:  "w",
			wantFound: true,
		},
		{
			name:      "no sizes at all",
			sizes:     nil,
			wantType:  "",
			wantFound: false,
		},
		{
			name: "only non-downloadable sizes available",
			sizes: []tg.PhotoSizeClass{
				&tg.PhotoStrippedSize{Type: "i", Bytes: []byte{1}},
				&tg.PhotoPathSize{Type: "j", Bytes: []byte{2}},
			},
			wantType:  "",
			wantFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := largestPhotoSize(tc.sizes)
			if found != tc.wantFound {
				t.Fatalf("largestPhotoSize() found = %v, want %v", found, tc.wantFound)
			}
			if got != tc.wantType {
				t.Fatalf("largestPhotoSize() = %q, want %q", got, tc.wantType)
			}
		})
	}
}

func TestUploadPhotoRequiresClient(t *testing.T) {
	// Guard against calling the Telegram API before the client is initialized.
	saved := tgClient
	tgClient = nil
	defer func() { tgClient = saved }()

	_, err := uploadPhoto(context.Background(), &tg.MessageMediaPhoto{})
	if err == nil {
		t.Fatal("expected error when tgClient is nil, got nil")
	}
}
