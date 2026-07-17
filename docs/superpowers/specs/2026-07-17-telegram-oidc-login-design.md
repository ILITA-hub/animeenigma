# Telegram OIDC Login ("new widget") — Design

**Date:** 2026-07-17
**Origin:** feedback TODO `2026-07-06T14-02-58_tNeymik_manual` — "Make tg login new wiget"
**Decision:** replace the QR + deep-link flow entirely with Telegram's 2026 OIDC login (owner-approved 2026-07-17).

**Metrics:** UXΔ = +3 (Better) · CDI = 0.06 * 8 · MVQ = Phoenix 90%/85%

## Background

Telegram shipped a new official login stack around April 2026: standard OpenID Connect
(OAuth2 authorization-code + PKCE) at `oauth.telegram.org`, with discovery at
`/.well-known/openid-configuration`, RS256-signed JWT id_tokens, and client credentials
minted via BotFather. Scopes: `openid`, `profile`, `phone`, `telegram:bot_access`.
The classic 2018 iframe widget is archived as legacy. Docs: `core.telegram.org/bots/telegram-login`.

AnimeEnigma's current login (built 2026-03-22 `c859121c`, refined 2026-05-11 `1d0b29ca`)
is a QR + `tg://` deep-link + Telegram-Web manual fallback, confirmed via the auth bot's
webhook and polled by the SPA (`POST /api/auth/telegram/deeplink`, `GET /api/auth/telegram/check`,
2 s polling, 5 min TTL). It works, but it is a multi-step dance (open bot → press Start →
press Confirm → wait for poll) with three fallback affordances stacked on the auth page.

## Chosen approach: backend-driven standard OIDC

Auth service owns the whole flow; the frontend is one styled button. No third-party JS
on our pages (privacy-core stance), secrets never reach the browser, works in every
browser including in-app webviews (plain redirects).

Rejected alternatives:
- **`Telegram.Login` JS library in the SPA** — loads Telegram script on our page,
  unknown maturity, CSP/ad-blocker exposure.
- **Identity broker (Keycloak/Authentik)** — an extra service for one provider.

## Phase 0 — contract verification spike (HARD GATE)

Knowledge of the OIDC contract comes from post-cutoff web research. Before any code:

1. Fetch live `https://oauth.telegram.org/.well-known/openid-configuration`; record
   authorize/token/jwks endpoints, supported scopes, PKCE methods.
2. Confirm the BotFather client-registration steps and produce exact owner instructions
   for minting `client_id`/`client_secret` for the existing auth bot.
3. Confirm id_token claim shape (`sub` = Telegram user id; username/name/photo claims
   under `profile`).
4. If reality differs from this spec (e.g. no client_secret, different claims), amend
   the implementation plan before coding.

Owner step: mint credentials in BotFather, add to `docker/.env` + k8s secret.

## Backend (`services/auth`)

New endpoints (public, via gateway `/api/auth/*`):

- `GET /api/auth/telegram/oidc/start?return=<path>` — generate `state` + PKCE verifier,
  store in Redis (5 min TTL), 302 to the authorize endpoint with
  `scope=openid profile telegram:bot_access`. `telegram:bot_access` preserves the bot's
  ability to message users (future notifications); **`phone` is deliberately not requested**.
- `GET /api/auth/telegram/oidc/callback?code&state` — validate + consume state, exchange
  the code server-side (client_secret + PKCE verifier), verify id_token via JWKS
  (`coreos/go-oidc/v3` + `golang.org/x/oauth2`), map claims → existing
  `domain.TelegramWebhookUser` shape → existing `LoginWithTelegram` (find-or-create by
  `telegram_id` uniqueIndex — existing accounts map cleanly) → set the existing access +
  refresh cookies → 302 to the validated `return` path.

`return` validation: must be a same-origin relative path (`/…`, no `//`), else `/`.

Config (`internal/config`): `TELEGRAM_OIDC_CLIENT_ID`, `TELEGRAM_OIDC_CLIENT_SECRET`,
`TELEGRAM_OIDC_REDIRECT_URL` (default `https://animeenigma.org/api/auth/telegram/oidc/callback`).
Documented in `docs/environment-variables.md`; secrets in `docker/.env` + k8s secret overlay.

Deleted (replace entirely, no legacy path):
- `POST /api/auth/telegram/deeplink`, `GET /api/auth/telegram/check`,
  `POST /api/auth/telegram/webhook` routes.
- `handler/telegram_bot.go` (webhook handler, confirm buttons, sendMessage helpers).
- `service` deep-link functions (`CreateDeepLinkToken`, `CheckDeepLinkToken`,
  `HandleTelegramStart`, `HandleTelegramCallback`) and `domain.TelegramAuthSession`,
  `DeepLinkResponse`, `DeepLinkCheckResponse`.
- Startup `SetWebhook` — replaced by a one-time `deleteWebhook` call on boot so Telegram
  stops POSTing to the removed route. (`TELEGRAM_BOT_TOKEN` stays: needed for
  `deleteWebhook` and any future bot messaging; nothing else consumes the auth bot.
  `TELEGRAM_BOT_NAME` becomes unused — remove from config + env docs.)

Kept: `LoginWithTelegram`, `TelegramWebhookUser`, cookie/session machinery,
`AuthEventsTotal{telegram_login}` metric (+ failure-reason label for OIDC errors).

## Frontend (`frontend/web`)

`Auth.vue` rewritten: Neon-Tokyo glass card with one "Continue with Telegram" button →
`location.href = /api/auth/telegram/oidc/start?return=<returnUrl>` (returnUrl from the
existing sessionStorage handoff). Error banner driven by `?error=` on bounce-back.
Deleted: QR canvas + `qrcode` dep, polling/countdown/troubleshooting/tg-web-fallback
code and their i18n keys; new keys added to en/ru/ja. `stores/auth.ts` drops
`requestDeepLink`/`checkDeepLink`. The existing `oauth.telegram.org` preconnects in
`index.html` become load-bearing again and stay.

Small visual scope → no design-prototyping sandbox; `/frontend-verify` gates the change.

## Error handling

- State missing/expired/mismatch → `/auth?error=expired` (retry button re-runs start).
- Token-exchange or JWKS failure → `/auth?error=telegram` (reason logged server-side).
- User denies consent → `/auth` with neutral message.
- All failures land on the auth page with one-click retry; no dead ends.

## Testing

- Go: handler tests with an httptest fake IdP (discovery + JWKS + token endpoints)
  covering happy path, bad state, bad signature, denied consent, return-path validation.
  Existing `LoginWithTelegram` tests untouched.
- FE: `/frontend-verify` (DS-lint, i18n en/ru/ja parity, real build).
- e2e unaffected (`ui_audit_bot` uses password login).

## Rollout

1. Phase 0 spike + owner mints BotFather creds → env/secrets in place.
2. Deploy auth (new routes live, old routes gone — single deploy, replace-entirely).
3. Deploy web.
4. Existing sessions unaffected (non-rotating refresh cookies keep working).
5. `/animeenigma-after-update` (changelog, health, push).

Risk accepted by owner: single login path — if `oauth.telegram.org` is down, login is
down (registered sessions persist, so only new logins are affected).
