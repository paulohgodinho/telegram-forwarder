# telegram-forwarder

Gets messages from Telegram (via [Telethon](https://docs.telethon.dev/)) and forwards them to an n8n webhook. Photo attachments are uploaded to an S3-compatible bucket (e.g. Garage) and their public URL is included in the forwarded payload.

## Required environment variables

| Variable | Description |
|---|---|
| `TELEGRAM_APP_ID` | Telegram API app ID (from [my.telegram.org](https://my.telegram.org)) |
| `TELEGRAM_APP_HASH` | Telegram API app hash |
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
| `WEB_AUTH_PORT` | `56899` | Port for the one-time web form used to log in |

## Running

The container uses `network_mode: host`, sharing the host machine's network stack (including `/etc/resolv.conf`) directly - so `TELEGRAM_N8N_WEBHOOK_URL` resolves hostnames (e.g. `http://n8n:5678/...`) exactly as the host itself would, with no Docker bridge network or port mapping involved. `WEB_AUTH_PORT` binds directly on the host.

```sh
docker compose up -d --build
```

On first run, open `http://<host>:${WEB_AUTH_PORT}` and complete the login flow: phone number, then the code sent to your Telegram account, then your 2FA password if you have one enabled. The session is persisted in the `telegram-forwarder-session` volume, so this is only needed once.

## Testing

All dependencies install only inside Docker - nothing is installed on the host. The pytest suite runs in its own build stage:

```sh
docker compose -f docker-compose.test.yml build
docker compose -f docker-compose.test.yml run --rm tests
```

## Development notes

- `uv.lock` is checked in and must be regenerated inside Docker if `pyproject.toml` changes: `docker run --rm -v "$PWD":/app -w /app ghcr.io/astral-sh/uv:python3.13-bookworm-slim uv lock`.
- The `Dockerfile` has three stages: `deps` (prod-only, cached), `test` (adds dev deps + runs pytest), `runtime` (prod deps + app, non-root user). `docker compose build` targets `runtime` by default.

## Behavior notes

- **Every** message is forwarded, including ones you send yourself (no incoming-only filter) - this mirrors a straightforward `events.NewMessage` listener, covering DMs, basic groups, supergroups, and channels alike.
- Photo uploads are retried up to 3 times. If all attempts fail, the error is logged and the message is still forwarded to n8n without an `image_url` - a failed upload never blocks forwarding.
- Only photo attachments are uploaded; other media types are ignored for now.
- A heartbeat pings Telegram every minute (a raw MTProto `PingRequest`) and logs the result, success or failure.
- The Telegram session (auth key, entity access hashes, update state) is persisted via Telethon's own SQLite session file in the mounted volume, so it survives restarts without the custom access-hash bookkeeping a from-scratch implementation would otherwise need.
- Shutdown (`SIGINT`/`SIGTERM`, e.g. `docker stop`) cancels the main task directly rather than waiting for the library to notice a disconnect - this keeps shutdown bounded even if an in-flight Telegram RPC never resolves.

## n8n payload shape

Matches field-for-field what a sender would expect regardless of implementation language:

```json
{"sender": "Jane Doe", "group_name": "Family Group", "text": "hello", "date": "2026-10-24T12:00:00Z", "image_url": "https://.../photos/2026/10/24/123.jpg"}
```

`group_name` is `""` for private chats. `image_url` is omitted entirely when the message has no photo (or the upload failed).

## S3 key scheme

Photos are uploaded to `photos/YYYY/MM/DD/<photo_id>.jpg`, sharded by the photo's own date (not the message's), so listings stay browsable and sorted without one huge flat prefix.

## Logging

All logs are structured JSON written to stdout. Most entries are compact, single-line JSON:

```json
{"time": "2026-10-24T23:43:58Z", "level": "INFO", "msg": "telegram heartbeat ok"}
```

The "received telegram message" log is intentionally narrow - just the channel name and the first 40 characters of the message:

```json
{"time": "2026-10-24T23:43:58Z", "level": "INFO", "msg": "received telegram message", "channel": "Family Group", "message": "hey, check out this photo from the trip"}
```

The only exception is n8n delivery logs, which are pretty-printed (multi-line, alphabetically-sorted keys) and include the uploaded image's URL when present:

```json
{
  "image_url": "https://offers-storage.web.internal.ghomelab.top/photos/2026/10/24/123.jpg",
  "level": "INFO",
  "msg": "n8n delivery result",
  "sender": "Jane Doe",
  "status_code": 200,
  "text": "hey, check out this photo from the trip"
}
```

Telethon's own internal logger is routed into the same compact handler (tagged via `base_logger="telethon"`), so connection/reconnect diagnostics are visible instead of silently discarded.

## Exception isolation

Every message is handled through `safe_handle_message`, which catches any exception and logs it as `"msg": "exception recovered"` (with `where`, `error`, and a full traceback) instead of letting it propagate - one bad message can't take down the listener or affect any other message. The same pattern wraps the n8n delivery call and the asyncio loop's own exception handler.
