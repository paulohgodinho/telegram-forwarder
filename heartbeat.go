package main

import (
	"context"
	"log/slog"
	"time"
)

// heartbeatInterval is how often we verify the Telegram connection is alive.
const heartbeatInterval = time.Minute

// heartbeatPingTimeout bounds each individual ping attempt.
const heartbeatPingTimeout = 5 * time.Second

// pinger is satisfied by *telegram.Client; narrowed for testability.
type pinger interface {
	Ping(ctx context.Context) error
}

// startHeartbeat pings p every interval until ctx is canceled, logging the
// outcome of every attempt (success and failure).
func startHeartbeat(ctx context.Context, p pinger, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingOnce(ctx, p)
			}
		}
	}()
}

// pingOnce performs a single bounded ping attempt and logs its outcome.
func pingOnce(ctx context.Context, p pinger) {
	pingCtx, cancel := context.WithTimeout(ctx, heartbeatPingTimeout)
	defer cancel()

	if err := p.Ping(pingCtx); err != nil {
		slog.Error("telegram heartbeat failed", "error", err)
		return
	}
	slog.Info("telegram heartbeat ok")
}
