package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/tg"
)

// sizedPhotoSize is implemented by the downloadable PhotoSizeClass variants
// (tg.PhotoSize, tg.PhotoCachedSize, tg.PhotoSizeProgressive).
type sizedPhotoSize interface {
	GetW() int
	GetH() int
	GetType() string
}

// largestPhotoSize returns the "type" identifier (e.g. "x", "y", "w") of the
// highest-resolution downloadable size available for a photo.
func largestPhotoSize(sizes []tg.PhotoSizeClass) (string, bool) {
	var (
		best       string
		maxW, maxH int
		found      bool
	)
	for _, s := range sizes {
		sz, ok := s.(sizedPhotoSize)
		if !ok {
			continue
		}
		if w, h := sz.GetW(), sz.GetH(); w*h >= maxW*maxH {
			maxW, maxH = w, h
			best = sz.GetType()
			found = true
		}
	}
	return best, found
}

// photoContentType is the MIME type Telegram always transcodes compressed
// photos to (regardless of the original format the user sent).
const photoContentType = "image/jpeg"

// uploadPhoto downloads the highest-resolution version of a message's photo
// into memory and uploads it to S3, returning its public URL on success.
func uploadPhoto(ctx context.Context, media *tg.MessageMediaPhoto) (string, error) {
	if tgClient == nil {
		return "", errors.New("telegram client not initialized")
	}

	photo, ok := media.Photo.AsNotEmpty()
	if !ok {
		return "", errors.New("photo is empty or not resolved")
	}

	thumbSize, ok := largestPhotoSize(photo.Sizes)
	if !ok {
		return "", errors.New("no downloadable photo size found")
	}

	loc := photo.AsInputPhotoFileLocation(thumbSize)

	var buf bytes.Buffer
	if _, err := tgClient.Download(loc).Stream(ctx, &buf); err != nil {
		return "", errors.Wrap(err, "download photo")
	}

	key := fmt.Sprintf(
		"photos/%s/%d.jpg",
		time.Unix(int64(photo.Date), 0).Format("2006/01/02"),
		photo.ID,
	)

	url, err := uploadPhotoBytes(ctx, key, buf.Bytes(), photoContentType)
	if err != nil {
		return "", errors.Wrap(err, "upload photo to s3")
	}

	slog.Info("uploaded photo",
		"key", key,
		"url", url,
		"photo_id", photo.ID,
		"size_type", thumbSize,
		"bytes", buf.Len(),
	)

	return url, nil
}
