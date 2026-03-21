# Telegram Deep Link + QR Code Authentication

**Date:** 2026-03-22
**Status:** Approved
**Replaces:** Telegram Login Widget (iframe-based) authentication

## Problem

The current Telegram login uses the Telegram Login Widget, which opens `oauth.telegram.org` in the browser. Most users are not logged into Telegram in their browser and must inconveniently enter their phone number, receive a code, etc. This creates unnecessary friction.

## Solution

Replace the widget with a deep link flow that opens the user's native Telegram app (desktop or mobile). The browser shows a QR code (for cross-device: scan with phone) and a clickable "Open in Telegram" button (for same-device: opens Telegram desktop app). Authentication is confirmed via bot interaction and detected by the browser through polling.

## Architecture

### Flow

```
Browser                     Auth Service                 Telegram API
  │                              │                            │
  │ POST /auth/telegram/deeplink │                            │
  │─────────────────────────────>│                            │
  │                              │ Generate token (UUID)      │
  │                              │ Redis: tgauth:{token} = "" │
  │                              │ TTL = 5 min                │
  │  {token, deeplink_url,        │                            │
  │   expires_in}                │                            │
  │<─────────────────────────────│                            │
  │                              │                            │
  │ [User clicks link or         │                            │
  │  scans QR code]              │                            │
  │            ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─>│
  │                              │                            │
  │                              │ Telegram sends /start      │
  │                              │ payload = token            │
  │                              │<───────────────────────────│
  │                              │                            │
  │                              │ Bot sends: "Confirm login?"│
  │                              │ [Confirm] inline button    │
  │                              │───────────────────────────>│
  │                              │                            │
  │                              │ User clicks Confirm        │
  │                              │ → callback_query           │
  │                              │<───────────────────────────│
  │                              │                            │
  │                              │ Redis: tgauth:{token} =   │
  │                              │   {telegram user JSON}     │
  │                              │                            │
  │ GET /auth/telegram/check     │                            │
  │     ?token=X                 │                            │
  │─────────────────────────────>│                            │
  │                              │ Read Redis tgauth:X        │
  │ {status:"confirmed",        │                            │
  │  access_token, user, cookies}│                            │
  │<─────────────────────────────│                            │
```

### New API Endpoints

#### `POST /api/auth/telegram/deeplink`

Generate auth token and deep link URL.

**Response:**
```json
{
  "token": "uuid-v4",
  "deeplink_url": "https://t.me/animeenigma_auth_bot?start=uuid-v4",
  "expires_in": 300
}
```

The deep link uses `https://t.me/` format (not `tg://`) because it works universally: opens the native app if installed, falls back to Telegram Web if not.

**Redis side-effect:** Sets `tgauth:{token}` = `""` with 5-minute TTL.

#### `GET /api/auth/telegram/check?token=X`

Poll for auth status. Called by frontend every 2 seconds.

**Responses:**
- Pending: `{"status": "pending"}`
- Confirmed: `{"status": "confirmed", "access_token": "...", "user": {...}}` + HttpOnly cookies (same as current login)
- Expired/not found: `{"status": "expired"}`

On confirmed response, the Redis key is deleted (single-use).

#### `POST /api/auth/telegram/webhook`

Telegram webhook receiver. Handles two update types:

1. **`message` with `/start {token}`**: Bot validates token exists in Redis. Stores the Telegram user ID in Redis (`tgauth:{token}` = `{"telegram_id": 123, "status": "started"}`). Sends confirmation message with inline keyboard button: `callback_data = "confirm_login:{token}"`.
2. **`callback_query` with `confirm_login:{token}`**: Bot verifies the callback sender's Telegram ID matches the one stored at step 1 (prevents QR phishing — someone else scanning your QR can't confirm your session). Stores full Telegram user data in Redis. Answers the callback query and edits the message to "Login confirmed".

**Request body** (from Telegram, JSON):
```json
{
  "update_id": 123456,
  "message": {
    "from": {"id": 789, "first_name": "User", "username": "user123"},
    "text": "/start uuid-v4-token",
    "chat": {"id": 789}
  }
}
```
or:
```json
{
  "update_id": 123457,
  "callback_query": {
    "id": "abc123",
    "from": {"id": 789, "first_name": "User", "username": "user123"},
    "data": "confirm_login:uuid-v4-token",
    "message": {"chat": {"id": 789}, "message_id": 42}
  }
}
```

**Response:** Always `200 OK` (Telegram retries on non-2xx, which would cause webhook floods).

**Security:** Validates `X-Telegram-Bot-Api-Secret-Token` header matches `TELEGRAM_WEBHOOK_SECRET` env var.

**Error handling:**
- Token not found in Redis on `/start`: Bot replies "This login link has expired. Please request a new one from the website."
- Token not found on callback confirm (expired mid-flow): Bot edits message to "Session expired. Please request a new login link."
- Sender mismatch on callback: Bot answers callback with "This login session belongs to a different user." (prevents QR phishing)

### Telegram Bot API Calls

Direct HTTP calls (same pattern as player service error reports, no library):

- `sendMessage` — Send confirmation prompt with inline keyboard
- `answerCallbackQuery` — Acknowledge button press
- `editMessageText` — Update message to "Login confirmed" after user confirms
- `setWebhook` — One-time setup (called on service startup if webhook URL configured)

### Redis Keys

| Key | Value | TTL | Description |
|-----|-------|-----|-------------|
| `tgauth:{token}` | `""` (empty) | 5 min | Pending — token created, waiting for `/start` |
| `tgauth:{token}` | `{"telegram_id":123,"status":"started"}` | 5 min | Started — user opened bot, awaiting confirm |
| `tgauth:{token}` | `{"id":123,"first_name":"...","username":"...","status":"confirmed"}` | 5 min | Confirmed — user clicked Confirm |

Key prefix `tgauth:` defined as `PrefixTelegramAuth` constant in `libs/cache/ttl.go`, with `TTLTelegramAuth = 5 * time.Minute`.

Key is deleted after successful login (check endpoint consumes it).

### Frontend Changes

**Auth.vue** — Complete replacement of current widget/OAuth flow:

1. **On mount:** `POST /api/auth/telegram/deeplink` → receive token + deep link URL
2. **Display:**
   - QR code encoding the `https://t.me/` deep link (using `qrcode` npm package)
   - Clickable "Open in Telegram" button with the deep link URL
   - Timer showing remaining seconds (5 min countdown)
3. **Poll:** `GET /api/auth/telegram/check?token=X` every 2 seconds
   - Stop polling on `confirmed` or `expired` status
   - Maximum 150 poll attempts (300s / 2s) as safety net
4. **On confirmed:** Receive JWT + both HttpOnly cookies (refresh token scoped to `/api/auth`, access token scoped to `/`), redirect to home
5. **On expired:** Show "Session expired" with a "Try again" button that generates a new token
6. **Troubleshooting hint:** After 30 seconds of polling with no confirmation, show a subtle hint: "Having trouble? Make sure you haven't blocked @animeenigma_auth_bot in Telegram."

### Configuration

New environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_WEBHOOK_SECRET` | Yes | Random string for webhook verification |
| `TELEGRAM_WEBHOOK_URL` | Yes | Public URL, e.g. `https://animeenigma.ru/api/auth/telegram/webhook` |

Existing variables unchanged: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_NAME`.

### Gateway Routing

No changes needed. The existing wildcard `r.HandleFunc("/auth/*", proxyHandler.ProxyToAuth)` already covers all new endpoints. The webhook endpoint is public (no JWT required), which matches the existing `/api/auth/*` public routes.

**Rate limiting note:** The webhook endpoint receives requests from Telegram's servers (a small set of IPs). The gateway's existing per-IP rate limiting is generous enough for webhook traffic (Telegram sends at most one update per user action). No special exemption needed.

### Security Considerations

- **Token entropy:** UUID v4 (122 bits of randomness), unguessable
- **Token lifetime:** 5 minutes, auto-expires in Redis
- **Single-use:** Token deleted from Redis after successful login
- **Webhook verification:** Secret token header checked on every request
- **User verification:** Telegram user data comes from Telegram's servers via webhook (trusted), not from the client
- **Rate limiting:** Existing gateway rate limiting applies to deeplink/check endpoints
- **QR phishing defense:** Telegram user ID stored at `/start` step; callback confirm verifies same user. A different person scanning the QR cannot confirm the session.
- **Bot confirmation message:** Includes "You are logging into AnimeEnigma" to make the action clear to the user

### Removed Components

- Telegram Login Widget script injection (`telegram-widget.js`)
- `oauth.telegram.org` redirect flow
- Mobile OAuth URL construction
- `GetTelegramConfig` endpoint (no longer needed — bot_id exposure removed)
- `data-auth-url` redirect handling in `checkTelegramRedirect()`

### Kept Components

- `LoginWithTelegram` service method — user lookup/creation by telegram_id (core logic reused, but simplified: no HMAC verification needed since data comes from trusted webhook)
- JWT token generation, cookie setting (unchanged)

### Additionally Removed

- `verifyTelegramAuth` method (webhook data is authenticated by secret token header, no HMAC check needed)
- `TelegramLoginRequest` domain type (replaced by a new `TelegramWebhookUser` struct that only contains the fields available from webhook data: `id`, `first_name`, `last_name`, `username`; no `auth_date`/`hash` fields)

## File Changes

### Backend (auth service)

| File | Change |
|------|--------|
| `internal/handler/auth.go` | Add `DeepLink`, `CheckDeepLink`, `TelegramWebhook` handlers; remove `TelegramLogin`, `GetTelegramConfig` |
| `internal/handler/telegram_bot.go` | New file: Telegram Bot API HTTP helpers (sendMessage, answerCallbackQuery, editMessageText, setWebhook) |
| `internal/service/auth.go` | Add `CreateDeepLinkToken`, `CheckDeepLinkToken`, `HandleTelegramWebhook` methods; remove `verifyTelegramAuth` |
| `internal/transport/router.go` | Update routes: add deeplink/check/webhook, remove old telegram endpoints |
| `internal/config/config.go` | Add `WebhookSecret`, `WebhookURL` to TelegramConfig |
| `cmd/auth-api/main.go` | Call `setWebhook` on startup if `TELEGRAM_WEBHOOK_URL` is set; log warning and continue on failure (auth service still works for username/password login) |

### Frontend

| File | Change |
|------|--------|
| `src/views/Auth.vue` | Replace widget/OAuth with QR + deep link + polling |
| `src/stores/auth.ts` | Add `requestDeepLink()`, `checkDeepLink()` methods; remove `loginWithTelegram()` |
| `package.json` | Add `qrcode` dependency |

### Config

| File | Change |
|------|--------|
| `docker/.env` | Add `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_WEBHOOK_URL` |
| `docker/docker-compose.yml` | Pass new env vars to auth service |
