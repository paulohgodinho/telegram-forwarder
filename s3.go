package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-faster/errors"
)

// s3Client is the shared client used to upload photos. Nil until
// configureS3 has run successfully.
var s3Client *s3.Client

// s3Bucket is the bucket uploads are written to.
var s3Bucket string

// s3PublicBaseURL is the bucket's public read endpoint; object keys are
// appended to it to build the link forwarded to n8n.
var s3PublicBaseURL string

// s3UploadRetries is how many attempts are made before giving up.
const s3UploadRetries = 3

// s3RetryBaseDelay is the base backoff between upload retry attempts.
const s3RetryBaseDelay = 500 * time.Millisecond

// configureS3 reads the S3/Garage connection details from the environment
// and initializes the shared client. All variables are required.
func configureS3() error {
	endpoint := os.Getenv("S3_ENDPOINT_URL")
	region := os.Getenv("S3_REGION")
	bucket := os.Getenv("S3_BUCKET")
	accessKeyID := os.Getenv("S3_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("S3_SECRET_ACCESS_KEY")
	publicBaseURL := os.Getenv("S3_PUBLIC_BASE_URL")

	switch {
	case endpoint == "":
		return errors.New("S3_ENDPOINT_URL is required")
	case region == "":
		return errors.New("S3_REGION is required")
	case bucket == "":
		return errors.New("S3_BUCKET is required")
	case accessKeyID == "":
		return errors.New("S3_ACCESS_KEY_ID is required")
	case secretAccessKey == "":
		return errors.New("S3_SECRET_ACCESS_KEY is required")
	case publicBaseURL == "":
		return errors.New("S3_PUBLIC_BASE_URL is required")
	}

	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	}

	s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		// Garage only supports path-style addressing, not virtual-hosted.
		o.UsePathStyle = true
	})
	s3Bucket = bucket
	s3PublicBaseURL = strings.TrimRight(publicBaseURL, "/")

	slog.Info("S3 client configured", "endpoint", endpoint, "region", region, "bucket", bucket)
	return nil
}

// uploadPhotoBytes uploads data to key, retrying up to s3UploadRetries
// times, and returns the object's public URL on success.
func uploadPhotoBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if s3Client == nil {
		return "", errors.New("S3 client not configured")
	}

	var lastErr error
	for attempt := 1; attempt <= s3UploadRetries; attempt++ {
		_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(s3Bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(contentType),
		})
		if err == nil {
			return fmt.Sprintf("%s/%s", s3PublicBaseURL, key), nil
		}

		lastErr = err
		slog.Warn("S3 upload attempt failed",
			"key", key,
			"attempt", attempt,
			"max_attempts", s3UploadRetries,
			"error", err,
		)

		if attempt < s3UploadRetries {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(s3RetryBaseDelay * time.Duration(attempt)):
			}
		}
	}

	return "", errors.Wrapf(lastErr, "upload %q after %d attempts", key, s3UploadRetries)
}
