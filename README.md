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
- A heartbeat pings Telegram every 10s and logs the result (success or failure) to verify the connection is alive.

## Logging

All logs are structured, pretty-printed JSON written to stdout (one indented JSON object per entry), e.g.:

```json
{
  "level": "INFO",
  "msg": "telegram heartbeat ok",
  "time": "2026-10-23T23:43:58-03:00"
}
```
