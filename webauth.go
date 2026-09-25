package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

type WebAuth struct {
	phone   string
	addr    string
	mu      sync.Mutex
	pending string
	ready   chan string
}

func NewWebAuth(phone, addr string) *WebAuth {
	return &WebAuth{
		phone: phone,
		addr:  addr,
		ready: make(chan string, 1),
	}
}

func (w *WebAuth) Phone(_ context.Context) (string, error) {
	if w.phone != "" {
		return w.phone, nil
	}
	return "", errors.New("missing phone number")
}

func (w *WebAuth) Password(_ context.Context) (string, error) {
	return "", nil
}

func (w *WebAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{}
}

func (w *WebAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign-up not implemented in WebAuth")
}

func (w *WebAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	select {
	case code := <-w.ready:
		return code, nil
	case <-time.After(10 * time.Minute):
		return "", errors.New("timed out waiting for Telegram auth code")
	}
}

func (w *WebAuth) queueCode(code string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = strings.TrimSpace(code)
	select {
	case w.ready <- w.pending:
	default:
	}
}

func (w *WebAuth) codeHandler(wr http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(wr, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(wr, "bad form", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.Form.Get("code"))
	if code == "" {
		http.Error(wr, "code required", http.StatusBadRequest)
		return
	}
	w.queueCode(code)
	_, _ = fmt.Fprint(wr, "sent")
}

func (w *WebAuth) pageHandler(wr http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(wr, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tmpl := template.Must(template.New("code").Parse(`<!doctype html>
<html>
<head><meta charset="utf-8"><title>Telegram Code</title></head>
<body>
  <form method="POST" action="/code">
    <label>Telegram code
      <input name="code" type="text" autocomplete="one-time-code" required>
    </label>
    <button type="submit">Submit</button>
  </form>
</body>
</html>`))
	_ = tmpl.Execute(wr, nil)
}

func (w *WebAuth) Serve() error {
	slog.Info("Telegram web auth is ready", "url", "http://localhost"+w.addr)
	http.HandleFunc("/", w.pageHandler)
	http.HandleFunc("/code", w.codeHandler)
	return http.ListenAndServe(w.addr, nil)
}
