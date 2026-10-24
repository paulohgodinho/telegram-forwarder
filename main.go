package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	pebbledb "github.com/cockroachdb/pebble"
	"github.com/go-faster/errors"
	boltstor "github.com/gotd/contrib/bbolt"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/contrib/pebble"
	"github.com/gotd/contrib/storage"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/tg"
	"go.etcd.io/bbolt"
	"golang.org/x/time/rate"
)

type N8NPayload struct {
	Sender    string    `json:"sender"`
	GroupName string    `json:"group_name"` // Empty string "" if 1-on-1 private chat
	Text      string    `json:"text"`
	Date      time.Time `json:"date"`
	ImageURL  string    `json:"image_url,omitempty"` // Public S3 URL, set only if the message had a photo and upload succeeded
}

var n8nWebhook string
var peerDB *pebble.PeerStorage
var tgClient *telegram.Client

func run(ctx context.Context) error {
	// 1. Environment Configuration
	appID, err := strconv.Atoi(os.Getenv("TELEGRAM_APP_ID"))
	if err != nil {
		return errors.Wrap(err, "invalid TELEGRAM_APP_ID")
	}
	appHash := os.Getenv("TELEGRAM_APP_HASH")
	phone := os.Getenv("TELEGRAM_TG_PHONE")
	webAuthPort := os.Getenv("WEB_AUTH_PORT")
	if webAuthPort == "" {
		webAuthPort = "56899"
	}
	n8nWebhook = os.Getenv("TELEGRAM_N8N_WEBHOOK_URL")
	if n8nWebhook == "" {
		return errors.New("TELEGRAM_N8N_WEBHOOK_URL is required")
	}

	if err := configureS3(); err != nil {
		return errors.Wrap(err, "configure S3")
	}

	// 2. Storage & Session Directory Setup
	sessionDir := filepath.Join("session", "data")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return err
	}

	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, "session.json"),
	}

	// Peer storage (PebbleDB) to keep track of user entity IDs across restarts
	pdb, err := pebbledb.Open(filepath.Join(sessionDir, "peers.pebble.db"), &pebbledb.Options{})
	if err != nil {
		return errors.Wrap(err, "open pebble db")
	}
	defer pdb.Close()
	peerDB = pebble.NewPeerStorage(pdb)

	// Update state recovery storage (BBolt) for tracking sequence state (pts/qts)
	boltdb, err := bbolt.Open(filepath.Join(sessionDir, "updates.bolt.db"), 0666, nil)
	if err != nil {
		return errors.Wrap(err, "open bolt db")
	}
	defer boltdb.Close()

	// 3. Dispatcher & Event Handlers
	dispatcher := tg.NewUpdateDispatcher()
	updateHandler := storage.UpdateHook(dispatcher, peerDB)

	accessHasher := newBoltAccessHasher(boltdb)

	updatesRecovery := updates.New(updates.Config{
		Handler:                         updateHandler,
		Storage:                         boltstor.NewStateStorage(boltdb),
		AccessHasher:                    accessHasher,
		UserAccessHasher:                accessHasher,
		Logger:                          gotdLogger(),
		OnChannelTooLong:                onChannelTooLong,
		OnChannelInaccessible:           onChannelInaccessible,
		OnTooLong:                       onTooLong,
		OnLoadUserStateFailed:           onLoadUserStateFailed,
		OnLoadChannelStateFailed:        onLoadChannelStateFailed,
		MaxChannelDifferenceConcurrency: maxChannelDifferenceConcurrency,
	})

	waiter := floodwait.NewWaiter()

	// Register message listener
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		if msg, ok := u.Message.(*tg.Message); ok {
			return safeHandleMessage(ctx, e, msg)
		}
		return nil
	})

	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		if msg, ok := u.Message.(*tg.Message); ok {
			return safeHandleMessage(ctx, e, msg)
		}
		return nil
	})

	// 4. Client Instantiation
	options := telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  updatesRecovery,
		Logger:         gotdLogger(),
		Middlewares: []telegram.Middleware{
			waiter,
			updhook.AffectedHook(updatesRecovery),
			ratelimit.New(rate.Every(time.Millisecond*100), 5),
		},
	}

	client := telegram.NewClient(appID, appHash, options)
	tgClient = client

	// 5. Auth Flow & Execution Engine
	slog.Info("starting telegram auth flow via web form")
	wa := NewWebAuth(phone, ":"+webAuthPort)
	go func() {
		if err := wa.Serve(); err != nil && err != http.ErrServerClosed {
			slog.Error("web auth server error", "error", err)
		}
	}()
	flow := auth.NewFlow(wa, auth.SendCodeOptions{})

	return waiter.Run(ctx, func(ctx context.Context) error {
		return client.Run(ctx, func(ctx context.Context) error {
			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				return errors.Wrap(err, "authentication failed")
			}

			self, err := client.Self(ctx)
			if err != nil {
				return errors.Wrap(err, "get self failed")
			}
			slog.Info("logged in to telegram", "first_name", self.FirstName, "username", self.Username)

			startHeartbeat(ctx, client, heartbeatInterval)

			// Start update processing loop
			return updatesRecovery.Run(ctx, client.API(), self.ID, updates.AuthOptions{
				IsBot: self.Bot,
				OnStart: func(ctx context.Context) {
					slog.Info("listener running, forwarding incoming messages to n8n")
				},
			})
		})
	})
}

// peerDisplayName resolves a human-friendly name (channel/group title, or
// user first+last name / username) from a persisted storage.Peer.
func peerDisplayName(peer storage.Peer) string {
	switch {
	case peer.Channel != nil:
		if title := strings.TrimSpace(peer.Channel.Title); title != "" {
			return title
		}
		if peer.Channel.Username != "" {
			return "@" + peer.Channel.Username
		}
	case peer.Chat != nil:
		if title := strings.TrimSpace(peer.Chat.Title); title != "" {
			return title
		}
	case peer.User != nil:
		if name := strings.TrimSpace(peer.User.FirstName + " " + peer.User.LastName); name != "" {
			return name
		}
		if peer.User.Username != "" {
			return "@" + peer.User.Username
		}
	}
	// Fallback: no friendly name available (e.g. forbidden/min entity), use
	// the debug representation so we at least know the kind/ID.
	return peer.String()
}

// messagePreviewLen bounds how much of a message's text is logged.
const messagePreviewLen = 40

// truncateRunes returns the first n runes of s, unchanged if already
// shorter (counts runes, not bytes, so multi-byte text isn't cut mid-char).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// logPanic writes a structured log entry for a recovered panic value r,
// tagged with where it happened, including a stack trace.
func logPanic(logger *slog.Logger, where string, r any) {
	logger.Error("panic recovered",
		"where", where,
		"panic", fmt.Sprint(r),
		"stack", string(debug.Stack()),
	)
}

// safeHandleMessage calls handleMessage, recovering any panic so one bad
// message can't crash the whole process (unrecovered panics are fatal).
func safeHandleMessage(ctx context.Context, e tg.Entities, msg *tg.Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(slog.Default(), "handling message", r)
			err = nil
		}
	}()
	return handleMessage(ctx, e, msg)
}

func handleMessage(ctx context.Context, e tg.Entities, msg *tg.Message) error {
	if msg.Out {
		slog.Debug("ignoring outgoing message", "peer_id", msg.GetPeerID(), "text", msg.Message)
		return nil // Ignore outgoing messages sent by you
	}

	senderName := "Unknown"
	groupName := ""

	// 1. Resolve Group / Supergroup / Channel Name using local peerDB
	switch p := msg.GetPeerID().(type) {
	case *tg.PeerChat, *tg.PeerChannel:
		if peer, err := storage.FindPeer(ctx, peerDB, p); err == nil {
			groupName = peerDisplayName(peer)
		}
	}

	// 2. Resolve Sender Name
	if msg.FromID != nil {
		if peer, err := storage.FindPeer(ctx, peerDB, msg.FromID); err == nil {
			senderName = peerDisplayName(peer)
		}
	} else if peer, err := storage.FindPeer(ctx, peerDB, msg.GetPeerID()); err == nil {
		// Fallback to chat/user peer
		senderName = peerDisplayName(peer)
	} else if groupName != "" {
		senderName = groupName
	}

	// 3. If the message carries a photo, upload it to S3 (with retries built
	// into uploadPhoto) and attach the resulting public URL to the payload.
	// A failed upload is logged but must never prevent the message itself
	// from being forwarded to n8n.
	imageURL := ""
	if photoMedia, ok := msg.Media.(*tg.MessageMediaPhoto); ok {
		url, err := uploadPhoto(ctx, photoMedia)
		if err != nil {
			slog.Error("failed to upload photo", "error", err, "sender", senderName)
		} else {
			imageURL = url
		}
	}

	payload := N8NPayload{
		Sender:    senderName,
		GroupName: groupName,
		Text:      msg.Message,
		Date:      time.Unix(int64(msg.Date), 0),
		ImageURL:  imageURL,
	}

	slog.Info("received telegram message",
		"channel", groupName,
		"message", truncateRunes(msg.Message, messagePreviewLen),
	)

	// Non-blocking POST dispatch to n8n
	go forwardToN8N(n8nWebhook, payload)
	return nil
}

// forwardToN8N POSTs payload to the n8n webhook (pretty multi-line logs via
// n8nLogger). Runs in its own goroutine per message, so panics are recovered.
func forwardToN8N(url string, payload N8NPayload) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(n8nLogger, "forwarding to n8n", r)
		}
	}()

	data, err := json.Marshal(payload)
	if err != nil {
		n8nLogger.Error("failed to marshal n8n payload", "error", err, "payload", payload)
		return
	}

	n8nLogger.Info("attempting to send message to n8n",
		"url", url,
		"sender", payload.Sender,
		"text", payload.Text,
		"date", payload.Date,
		"image_url", payload.ImageURL,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		n8nLogger.Error("failed to construct n8n request",
			"url", url,
			"sender", payload.Sender,
			"image_url", payload.ImageURL,
			"error", err,
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		n8nLogger.Error("failed to send message to n8n",
			"url", url,
			"sender", payload.Sender,
			"text", payload.Text,
			"image_url", payload.ImageURL,
			"error", err,
		)
		return
	}
	defer resp.Body.Close()

	statusText := http.StatusText(resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		n8nLogger.Error("n8n webhook rejected request",
			"url", url,
			"status_code", resp.StatusCode,
			"status_text", statusText,
			"sender", payload.Sender,
			"text", payload.Text,
			"image_url", payload.ImageURL,
			"response_body", string(body),
		)
		return
	}

	n8nLogger.Info("n8n delivery result",
		"url", url,
		"status_code", resp.StatusCode,
		"status_text", statusText,
		"sender", payload.Sender,
		"text", payload.Text,
		"image_url", payload.ImageURL,
		"response_body", string(body),
	)
}

func main() {
	setupLogging(os.Stdout, slog.LevelInfo)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("terminated by user")
			os.Exit(0)
		}
		slog.Error("runtime error", "error", err)
		os.Exit(1)
	}
}
