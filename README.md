# telegram-forwarder

Gets messages from Telegram using MTProto and forwards them to an n8n webhook. Photo attachments are uploaded to an S3-compatible bucket (e.g. Garage) and their public URL is included in the forwarded payload.

## Required environment variables

| Variable | Description |
|---|---|
| `TELEGRAM_APP_ID` | Telegram API app ID (from [my.telegram.org](https://my.telegram.org)) |
| `TELEGRAM_APP_HASH` | Telegram API app hash |
| `TELEGRAM_TG_PHONE` | Phone number used to log in (MTProto auth flow) |
| `TELEGRAM_N8N_WEBHOOK_URL` | n8n webhook URL that messages are forwarded to |
| `S3_ENDPOINT_URL` | S3-compatible endpoint used to store photo attachments (e.g. `https://garage-s3.internal.ghomelab.top`) |
| `S3_REGION` | S3 region (e.g. `garage`) |
| `S3_BUCKET` | Bucket name photos are uploaded to |
| `S3_ACCESS_KEY_ID` | Access key ID for the bucket |
| `S3_SECRET_ACCESS_KEY` | Secret access key for the bucket |
| `S3_PUBLIC_BASE_URL` | Public read base URL for the bucket, used to build the link sent to n8n (e.g. `https://offers-storage.web.internal.ghomelab.top`) |

## Optional environment variables

| Variable | Default | Description |
|---|---|---|
| `WEB_AUTH_PORT` | `56899` | Port for the one-time web form used to submit the Telegram login code |

## Running

```sh
docker compose up -d
```

On first run, open `http://<host>:${WEB_AUTH_PORT}` and submit the login code sent to your Telegram account to complete authentication. The session is persisted in the `telegram-forwarder-session` volume, so this is only needed once.

## Behavior notes

- Photo uploads are retried up to 3 times. If all attempts fail, the error is logged and the message is still forwarded to n8n without an `image_url` — a failed upload never blocks forwarding.
- Only photo attachments (`MessageMediaPhoto`) are uploaded; other media types are ignored for now.
- A heartbeat pings Telegram every minute and logs the result (success or failure) to verify the connection is alive.
- New messages are handled for every chat type: private DMs and basic groups (`OnNewMessage`, including short-form updates the library normalizes internally) and supergroups/channels (`OnNewChannelMessage`). Edited messages, service messages (joins/pins/etc.), and non-photo media are intentionally not forwarded yet.

## Update gap visibility

Telegram's update stream can develop gaps (e.g. after downtime) that are recovered via `getDifference`. If a gap is too large to replay, messages sent during it are permanently lost from the update stream — this is tracked **per chat** for groups/channels, so a busy group can hit this independently of other chats. We log every such event so missing messages are explainable instead of silent:

| Log message | Meaning |
|---|---|
| `telegram update gap too long for channel, messages were lost` | A specific group/channel/supergroup's gap could not be replayed; messages from that chat during the gap are gone. |
| `telegram account-wide update gap too long, messages were lost` | Same as above, but for the account-wide (DM/basic-group) update stream. |
| `telegram channel became inaccessible, stopped tracking it` | We lost access to a group/channel (kicked, banned, or it was deleted). |
| `telegram channel state missing locally, channel skipped` | A channel's access hash wasn't found; it's skipped until rediscovered. See note below. |
| `telegram update state not found locally, resynced from server` | No local update state on startup; a full resync was performed. |

Previously these conditions were silently discarded (the underlying library defaults to a no-op logger). All internal connection/reconnect/gap diagnostics now flow through the default logger, tagged with `"component": "gotd"`.

**On "channel state missing locally":** the update manager persists each channel's `pts` (via bbolt), but by default persists channel/user *access hashes* only in memory — wiped on every restart, so every previously-tracked group/channel got skipped on every deploy. We now persist access hashes in bbolt too (`access_hash_storage.go`), so this only fires for a channel the very first time, not on every restart.

## Logging

All logs are structured JSON written to stdout. Most entries are compact, single-line JSON:

```json
{"time":"2026-10-23T23:43:58-03:00","level":"INFO","msg":"telegram heartbeat ok"}
```

The "received telegram message" log is intentionally narrow — just the channel name and the first 40 characters of the message:

```json
{"time":"2026-10-23T23:43:58-03:00","level":"INFO","msg":"received telegram message","channel":"Family Group","message":"hey, check out this photo from the trip"}
```

The only exception is n8n delivery logs (`forwardToN8N`), which are pretty-printed (multi-line) and include the uploaded image's URL when present:

```json
{
  "image_url": "https://offers-storage.web.internal.ghomelab.top/photos/2026/10/23/123.jpg",
  "level": "INFO",
  "msg": "n8n delivery result",
  "sender": "Jane Doe",
  "status_code": 200,
  "text": "hey, check out this photo from the trip"
}
```
