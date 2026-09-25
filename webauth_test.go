package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebAuthCodeReadsQueuedCode(t *testing.T) {
	wa := NewWebAuth("+15551234567", ":0")
	wa.queueCode("12345")

	got, err := wa.Code(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "12345" {
		t.Fatalf("expected 12345, got %q", got)
	}
}

func TestWebAuthSubmitHandlerStoresCode(t *testing.T) {
	wa := NewWebAuth("+15551234567", ":0")
	req := httptest.NewRequest(http.MethodPost, "/code", nil)
	req.PostForm = map[string][]string{"code": {"98765"}}
	res := httptest.NewRecorder()

	wa.codeHandler(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if res.Body.String() != "sent" {
		t.Fatalf("expected body %q, got %q", "sent", res.Body.String())
	}

	got, err := wa.Code(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "98765" {
		t.Fatalf("expected 98765, got %q", got)
	}
}
