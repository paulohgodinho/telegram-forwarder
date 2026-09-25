package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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
}

var n8nWebhook string
var peerDB *pebble.PeerStorage

func run(ctx context.Context) error {
	// 1. Environment Configuration
	appID, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		return errors.Wrap(err, "invalid APP_ID")
	}
	appHash := os.Getenv("APP_HASH")
	phone := os.Getenv("TG_PHONE")
	webAuthPort := os.Getenv("WEB_AUTH_PORT")
	if webAuthPort == "" {
		webAuthPort = "56899"
	}
	n8nWebhook = os.Getenv("N8N_WEBHOOK_URL")
	if n8nWebhook == "" {
		return errors.New("N8N_WEBHOOK_URL is required")
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

	updatesRecovery := updates.New(updates.Config{
		Handler: updateHandler,
		Storage: boltstor.NewStateStorage(boltdb),
	})

	waiter := floodwait.NewWaiter()

	// Register message listener
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		if msg, ok := u.Message.(*tg.Message); ok {
			return handleMessage(ctx, e, msg)
		}
		return nil
	})

	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		if msg, ok := u.Message.(*tg.Message); ok {
			return handleMessage(ctx, e, msg)
		}
		return nil
	})

	// 4. Client Instantiation
	options := telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  updatesRecovery,
		Middlewares: []telegram.Middleware{
			waiter,
			updhook.AffectedHook(updatesRecovery),
			ratelimit.New(rate.Every(time.Millisecond*100), 5),
		},
	}

	client := telegram.NewClient(appID, appHash, options)

	// 5. Auth Flow & Execution Engine
	log.Println("Starting Telegram auth flow via web form")
	slog.Info("Starting Telegram auth flow via web form")
	wa := NewWebAuth(phone, ":"+webAuthPort)
	go func() {
		if err := wa.Serve(); err != nil && err != http.ErrServerClosed {
			log.Printf("web auth server error: %v", err)
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
			log.Printf("Logged in as %s (@%s)", self.FirstName, self.Username)

			// Start update processing loop
			return updatesRecovery.Run(ctx, client.API(), self.ID, updates.AuthOptions{
				IsBot: self.Bot,
				OnStart: func(ctx context.Context) {
					log.Println("Listener running. Forwarding incoming messages to n8n...")
				},
			})
		})
	})
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
			groupName = peer.String()
		}
	}

	// 2. Resolve Sender Name
	if msg.FromID != nil {
		if peer, err := storage.FindPeer(ctx, peerDB, msg.FromID); err == nil {
			senderName = peer.String()
		}
	} else if peer, err := storage.FindPeer(ctx, peerDB, msg.GetPeerID()); err == nil {
		// Fallback to chat/user peer
		senderName = peer.String()
	} else if groupName != "" {
		senderName = groupName
	}

	payload := N8NPayload{
		Sender:    senderName,
		GroupName: groupName,
		Text:      msg.Message,
		Date:      time.Unix(int64(msg.Date), 0),
	}

	slog.Info("received telegram message",
		"sender", senderName,
		"text", msg.Message,
		"date", payload.Date,
		"peer_id", msg.GetPeerID(),
	)

	// Non-blocking POST dispatch to n8n
	go forwardToN8N(n8nWebhook, payload)
	return nil
}

func forwardToN8N(url string, payload N8NPayload) {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal n8n payload", "error", err, "payload", payload)
		return
	}

	slog.Info("attempting to send message to n8n",
		"url", url,
		"sender", payload.Sender,
		"text", payload.Text,
		"date", payload.Date,
	)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		slog.Error("failed to send message to n8n",
			"url", url,
			"sender", payload.Sender,
			"text", payload.Text,
			"error", err,
		)
		return
	}
	defer resp.Body.Close()

	statusText := http.StatusText(resp.StatusCode)
	slog.Info("n8n delivery result",
		"url", url,
		"status_code", resp.StatusCode,
		"status_text", statusText,
		"sender", payload.Sender,
		"text", payload.Text,
	)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Println("Terminated by user.")
			os.Exit(0)
		}
		log.Fatalf("Runtime error: %+v", err)
	}
}
