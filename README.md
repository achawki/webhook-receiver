# Webhook Receiver
[![Go Report Card](https://goreportcard.com/badge/achawki/webhook-receiver?style=flat)](https://goreportcard.com/report/achawki/webhook-receiver) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/achawki/webhook-receiver/blob/master/LICENSE)

Minimal Go application to receive webhook and HTTP requests, inspect them in a small built-in UI, and persist captured traffic to SQLite.

[Live Demo](https://webhook-receiver.devmino.cloud)

## Features

- Server-rendered UI at `/`
- JSON API for creating receivers and reading paginated captured requests
- Public ingest endpoint at `/hooks/{id}`
- SQLite persistence
- Automatic deletion 48 hours after webhook creation
- Each webhook keeps only its newest 100 captured requests
- Optional basic auth
- Optional header token
- Optional HMAC SHA-256 verification
- Additive request validation: if multiple auth checks are configured, all of them must pass
- Failed auth attempts are still captured with a 401 result and a non-secret error message for debugging
- Basic per-IP rate limiting
- Message filtering by outcome: `all`, `accepted`, `rejected`

## Run server

From source. Requires Go `1.25+`:

The app requires `WEBHOOK_RECEIVER_ENCRYPTION_KEY`, which must contain base64 or hex encoded 32-byte key material.

```bash
export WEBHOOK_RECEIVER_ENCRYPTION_KEY="$(openssl rand -base64 32)"
go run main.go
```

Open the UI at [http://localhost:8080](http://localhost:8080).

By default, data is persisted to `./webhook-receiver.db`.
Override the location with `WEBHOOK_RECEIVER_STORE_PATH`:

```bash
WEBHOOK_RECEIVER_ENCRYPTION_KEY="$(openssl rand -base64 32)" \
WEBHOOK_RECEIVER_STORE_PATH=/var/lib/webhook-receiver/webhook-receiver.db \
go run main.go
```

Basic-auth passwords and header-token values are stored as bcrypt hashes. HMAC secrets are stored encrypted in the database.

Optional runtime settings:

- `WEBHOOK_RECEIVER_PUBLIC_BASE_URL`
  Use this absolute URL when returning `detailUrl`, `hookUrl`, and `messagesUrl`. Set this in any deployed environment. Without it, the app only emits absolute URLs for trusted local loopback requests and otherwise falls back to relative paths.
- `WEBHOOK_RECEIVER_CLIENT_IP_HEADER`
  Optional request header to use for client IP detection in the rate limiter. If it is unset, the app uses `RemoteAddr`.
- `WEBHOOK_RECEIVER_LISTEN_ADDR`
  Override the listen address. Default: `:8080`.

## Create receiver

Create an open receiver:

```bash
curl \
  --header "Content-Type: application/json" \
  --request POST \
  --data '{}' \
  https://webhook-receiver.devmino.cloud/api/webhooks
```

Response:

```json
{
  "id": "010d1338-5323-4e3d-93a9-4277bae8d7c4",
  "detailUrl": "https://webhook-receiver.devmino.cloud/webhooks/010d1338-5323-4e3d-93a9-4277bae8d7c4",
  "hookUrl": "https://webhook-receiver.devmino.cloud/hooks/010d1338-5323-4e3d-93a9-4277bae8d7c4",
  "messagesUrl": "https://webhook-receiver.devmino.cloud/api/webhooks/010d1338-5323-4e3d-93a9-4277bae8d7c4/messages",
  "expiresAt": "2026-03-23T12:00:00Z"
}
```

Create a receiver with basic auth:

```bash
curl \
  --header "Content-Type: application/json" \
  --request POST \
  --data '{"username":"username","password":"password"}' \
  https://webhook-receiver.devmino.cloud/api/webhooks
```

Create a receiver with a header token:

```bash
curl \
  --header "Content-Type: application/json" \
  --request POST \
  --data '{"tokenName":"Auth-Token","tokenValue":"token"}' \
  https://webhook-receiver.devmino.cloud/api/webhooks
```

Create a receiver with token and HMAC enabled:

```bash
curl \
  --header "Content-Type: application/json" \
  --request POST \
  --data '{"tokenName":"Auth-Token","tokenValue":"token","hmacHeader":"X-Hub-Signature-256","hmacSecret":"secret"}' \
  https://webhook-receiver.devmino.cloud/api/webhooks
```

## Send requests

Send requests to the public endpoint:

```bash
curl \
  --header "Content-Type: application/json" \
  --request POST \
  --data '{"information":"content"}' \
  https://webhook-receiver.devmino.cloud/hooks/WEBHOOK_ID
```

If a header token is configured:

```bash
curl \
  --header "Content-Type: application/json" \
  --header "Auth-Token: token" \
  --request POST \
  --data '{"information":"content"}' \
  https://webhook-receiver.devmino.cloud/hooks/WEBHOOK_ID
```

If HMAC is configured, compute a SHA-256 signature over the raw request body and send it in the configured header:

```bash
BODY='{"information":"content"}'
SIGNATURE=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac 'secret' -hex | sed 's/^.* //')

curl \
  --header "Content-Type: application/json" \
  --header "X-Hub-Signature-256: sha256=$SIGNATURE" \
  --request POST \
  --data "$BODY" \
  https://webhook-receiver.devmino.cloud/hooks/WEBHOOK_ID
```

If a delivery fails webhook auth, the receiver still records that attempt so it can be inspected later. The stored message will include `statusCode: 401` and an `error` describing which check failed, without persisting secret header values.

Each webhook keeps only its newest 100 captured requests. Once that limit is exceeded, the oldest captured requests are deleted automatically.

## Show captured requests

Read captured requests for one receiver:

```bash
curl "https://webhook-receiver.devmino.cloud/api/webhooks/WEBHOOK_ID/messages?page=1&pageSize=25&outcome=all"
```

Example response:

```json
{
  "webhookId": "WEBHOOK_ID",
  "expiresAt": "2026-03-23T12:00:00Z",
  "outcome": "all",
  "messages": [
    {
      "method": "POST",
      "path": "/hooks/WEBHOOK_ID",
      "payload": "{\"information\":\"content\"}",
      "statusCode": 200,
      "headers": {
        "Accept": [
          "*/*"
        ],
        "Content-Length": [
          "25"
        ],
        "Content-Type": [
          "application/json"
        ],
        "User-Agent": [
          "curl/8.0.1"
        ]
      },
      "time": "2026-03-21T12:00:00Z"
    }
  ],
  "page": 1,
  "pageSize": 25,
  "totalMessages": 1,
  "totalPages": 1,
  "hasNextPage": false,
  "hasPreviousPage": false
}
```

Use `outcome=accepted` or `outcome=rejected` to focus on successful deliveries or rejected attempts.

There is no global list endpoint. Keep `detailUrl`, `hookUrl`, or `messagesUrl` if you want to come back to the webhook before it expires.

The built-in rate limiter allows 300 requests per 5 minutes per IP address across the app. By default it uses `RemoteAddr`. If `WEBHOOK_RECEIVER_CLIENT_IP_HEADER` is set, the app will use that header when it contains a valid IP address and otherwise fall back to `RemoteAddr`.
