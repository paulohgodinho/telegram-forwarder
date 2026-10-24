package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// withTestLogger installs a pretty-JSON logger writing to buf, restoring
// the previous default logger afterward.
func withTestLogger(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(newPrettyJSONHandler(buf, slog.LevelInfo)))
	t.Cleanup(func() { slog.SetDefault(prev) })
}

func TestGotdLoggerIsUsable(t *testing.T) {
	var buf bytes.Buffer
	withTestLogger(t, &buf)

	l := gotdLogger()
	if l == nil {
		t.Fatal("gotdLogger() returned nil")
	}
	if !l.Enabled(context.Background(), 0) { // log.LevelInfo == 0
		t.Fatal("expected info level to be enabled")
	}
}

func TestOnChannelTooLongLogsChannelID(t *testing.T) {
	var buf bytes.Buffer
	withTestLogger(t, &buf)

	onChannelTooLong(42)

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["level"] != "ERROR" {
		t.Fatalf("expected ERROR level, got %v", decoded["level"])
	}
	if decoded["channel_id"] != float64(42) {
		t.Fatalf("expected channel_id=42, got %v", decoded["channel_id"])
	}
}

func TestOnChannelInaccessibleLogsChannelID(t *testing.T) {
	var buf bytes.Buffer
	withTestLogger(t, &buf)

	onChannelInaccessible(7)

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["channel_id"] != float64(7) {
		t.Fatalf("expected channel_id=7, got %v", decoded["channel_id"])
	}
}

func TestOnTooLongLogs(t *testing.T) {
	var buf bytes.Buffer
	withTestLogger(t, &buf)

	onTooLong()

	if buf.Len() == 0 {
		t.Fatal("expected a log entry, got none")
	}
}

func TestOnLoadUserStateFailedLogs(t *testing.T) {
	var buf bytes.Buffer
	withTestLogger(t, &buf)

	onLoadUserStateFailed()

	if buf.Len() == 0 {
		t.Fatal("expected a log entry, got none")
	}
}

func TestOnLoadChannelStateFailedLogsChannelID(t *testing.T) {
	var buf bytes.Buffer
	withTestLogger(t, &buf)

	onLoadChannelStateFailed(99)

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["channel_id"] != float64(99) {
		t.Fatalf("expected channel_id=99, got %v", decoded["channel_id"])
	}
}
