package main

import (
	"log/slog"

	gotdlog "github.com/gotd/log"
	"github.com/gotd/log/logslog"
)

// maxChannelDifferenceConcurrency bounds concurrent per-channel gap
// recovery, avoiding a FLOOD_WAIT storm across many active groups.
const maxChannelDifferenceConcurrency = 4

// gotdLogger adapts slog.Default() to gotd's log.Logger port, so internal
// connection/reconnect/gap diagnostics stop being silently discarded.
func gotdLogger() gotdlog.Logger {
	return logslog.New(slog.Default().With("component", "gotd"))
}

// onChannelTooLong fires when a channel/group's update gap can't be
// recovered; messages sent during the gap are permanently lost.
func onChannelTooLong(channelID int64) {
	slog.Error("telegram update gap too long for channel, messages were lost",
		"channel_id", channelID,
	)
}

// onChannelInaccessible fires when we lose access to a channel (kicked,
// banned, or deleted), detected via CHANNEL_PRIVATE.
func onChannelInaccessible(channelID int64) {
	slog.Warn("telegram channel became inaccessible, stopped tracking it",
		"channel_id", channelID,
	)
}

// onTooLong fires when the account-wide update gap can't be recovered;
// pending private/basic-group messages in that gap are lost.
func onTooLong() {
	slog.Error("telegram account-wide update gap too long, messages were lost")
}

// onLoadUserStateFailed fires on startup when no local update state was
// found, so a full resync was performed from the server.
func onLoadUserStateFailed() {
	slog.Warn("telegram update state not found locally, resynced from server")
}

// onLoadChannelStateFailed fires when a channel's locally stored access
// hash is missing, so that channel is skipped until rediscovered.
func onLoadChannelStateFailed(channelID int64) {
	slog.Warn("telegram channel state missing locally, channel skipped",
		"channel_id", channelID,
	)
}
