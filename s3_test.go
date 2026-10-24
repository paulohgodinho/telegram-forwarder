package main

import (
	"context"
	"os"
	"testing"
)

// withEnv sets the given env vars for the duration of the test and restores
// their previous values (or unsets them if they weren't set) afterward.
func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		prev, had := os.LookupEnv(k)
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
		t.Cleanup(func() {
			if had {
				os.Setenv(k, prev)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestConfigureS3RequiresAllVars(t *testing.T) {
	full := map[string]string{
		"S3_ENDPOINT_URL":      "https://garage-s3.internal.ghomelab.top",
		"S3_REGION":            "garage",
		"S3_BUCKET":            "offers-storage",
		"S3_ACCESS_KEY_ID":     "GKtest",
		"S3_SECRET_ACCESS_KEY": "secret",
		"S3_PUBLIC_BASE_URL":   "https://offers-storage.web.internal.ghomelab.top",
	}

	cases := []struct {
		name    string
		missing string // env var to blank out; "" means none (happy path)
	}{
		{name: "all present", missing: ""},
		{name: "missing endpoint", missing: "S3_ENDPOINT_URL"},
		{name: "missing region", missing: "S3_REGION"},
		{name: "missing bucket", missing: "S3_BUCKET"},
		{name: "missing access key", missing: "S3_ACCESS_KEY_ID"},
		{name: "missing secret key", missing: "S3_SECRET_ACCESS_KEY"},
		{name: "missing public base url", missing: "S3_PUBLIC_BASE_URL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := make(map[string]string, len(full))
			for k, v := range full {
				env[k] = v
			}
			if tc.missing != "" {
				env[tc.missing] = ""
			}
			withEnv(t, env)

			// Reset package state so tests don't leak into each other.
			savedClient, savedBucket, savedURL := s3Client, s3Bucket, s3PublicBaseURL
			t.Cleanup(func() { s3Client, s3Bucket, s3PublicBaseURL = savedClient, savedBucket, savedURL })

			err := configureS3()
			if tc.missing == "" {
				if err != nil {
					t.Fatalf("configureS3() with all vars set: unexpected error: %v", err)
				}
				if s3Client == nil {
					t.Fatal("configureS3() left s3Client nil on success")
				}
				if s3Bucket != "offers-storage" {
					t.Fatalf("s3Bucket = %q, want %q", s3Bucket, "offers-storage")
				}
				if s3PublicBaseURL != "https://offers-storage.web.internal.ghomelab.top" {
					t.Fatalf("s3PublicBaseURL = %q, want trimmed base URL", s3PublicBaseURL)
				}
				return
			}
			if err == nil {
				t.Fatalf("configureS3() with %s missing: expected error, got nil", tc.missing)
			}
		})
	}
}

func TestConfigureS3TrimsTrailingSlashFromPublicBaseURL(t *testing.T) {
	withEnv(t, map[string]string{
		"S3_ENDPOINT_URL":      "https://garage-s3.internal.ghomelab.top",
		"S3_REGION":            "garage",
		"S3_BUCKET":            "offers-storage",
		"S3_ACCESS_KEY_ID":     "GKtest",
		"S3_SECRET_ACCESS_KEY": "secret",
		"S3_PUBLIC_BASE_URL":   "https://offers-storage.web.internal.ghomelab.top/",
	})
	savedClient, savedBucket, savedURL := s3Client, s3Bucket, s3PublicBaseURL
	t.Cleanup(func() { s3Client, s3Bucket, s3PublicBaseURL = savedClient, savedBucket, savedURL })

	if err := configureS3(); err != nil {
		t.Fatalf("configureS3() unexpected error: %v", err)
	}
	if s3PublicBaseURL != "https://offers-storage.web.internal.ghomelab.top" {
		t.Fatalf("s3PublicBaseURL = %q, want trailing slash trimmed", s3PublicBaseURL)
	}
}

func TestUploadPhotoBytesRequiresConfiguredClient(t *testing.T) {
	saved := s3Client
	s3Client = nil
	t.Cleanup(func() { s3Client = saved })

	_, err := uploadPhotoBytes(context.Background(), "photos/2026/01/01/1.jpg", []byte("data"), "image/jpeg")
	if err == nil {
		t.Fatal("expected error when s3Client is nil, got nil")
	}
}
