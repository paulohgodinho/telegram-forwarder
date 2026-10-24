package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestPrettyJSONHandlerIsIndented(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newPrettyJSONHandler(&buf, slog.LevelInfo))
	logger.Info("hello", "foo", "bar")

	out := buf.String()
	if !strings.Contains(out, "\n  \"foo\": \"bar\"") {
		t.Fatalf("expected indented output, got:\n%s", out)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if decoded["msg"] != "hello" || decoded["foo"] != "bar" || decoded["level"] != "INFO" {
		t.Fatalf("unexpected decoded fields: %+v", decoded)
	}
}

func TestPrettyJSONHandlerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newPrettyJSONHandler(&buf, slog.LevelInfo))
	logger.Debug("should be suppressed")
	if buf.Len() != 0 {
		t.Fatalf("expected no output below min level, got: %s", buf.String())
	}
}

func TestPrettyJSONHandlerWithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newPrettyJSONHandler(&buf, slog.LevelInfo)).
		With("request_id", "abc123").
		WithGroup("http").
		With("status", 200)
	logger.Info("done")

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["request_id"] != "abc123" {
		t.Fatalf("expected top-level request_id, got: %+v", decoded)
	}
	group, ok := decoded["http"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested http group, got: %+v", decoded)
	}
	if group["status"] != float64(200) {
		t.Fatalf("expected http.status = 200, got: %+v", group)
	}
}

func TestPrettyJSONHandlerErrorValue(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newPrettyJSONHandler(&buf, slog.LevelInfo))
	logger.Error("failed", "error", errBoom)

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded["error"] != "boom" {
		t.Fatalf("expected error to be rendered as its message, got: %+v", decoded)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

var errBoom = &testError{msg: "boom"}
