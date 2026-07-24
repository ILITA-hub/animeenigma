# Alternative Login: Passkeys (WebAuthn) + TLS Client-Certificate Auto-Login — Design

- **Date:** 2026-07-24
- **Origin:** owner TODO, feedback report `2026-07-23T15-56-53_tNeymik_telegram` («сделать вход по паски и по тлс сертификату»)
- **Status:** approved direction (owner, 2026-07-24); this doc is the implementation contract
- **Metrics:** UXΔ = +3 (Better) · CDI = 0.06 * 21 · MVQ = Griffin 88%/85%

## Goal

Two new alternative login methods alongside the existing Telegram QR/deeplink login:

1. **Passkey (WebAuthn)** — enroll in settings; a small "Sign in with passkey" button on the login page.
2. **TLS client certificate auto-login** — issue certificates from settings; a per-user toggle "log in automatically when a certificate is detected". When enabled and the browser holds a valid cert, the user is logged in silently — the login page is never shown.

## Non-goals

- No "sign in with certificate" button on the login page (owner decision 2026-07-24). Certificate login is *only* the automatic path governed by the settings toggle.
- No password-login UI resurrection, no changes to Telegram login.
- No CRL/OCSP infrastructure — revocation is a DB check in the auth service.
- `.ru` domain support: passkeys are RP-ID-bound to `animeenigma.org`; the retiring `.ru` mirror gets neither feature.

## Constraints & context (verified 2026-07-24)

- Login page is `frontend/web/src/views/Auth.vue` (route `/auth`), Telegram-only today.
- All logins funnel through `createSessionAndAuthResponse` (`services/auth/internal/service/auth.go`) → non-rotating session + `refresh_token`/`access_token` cookies. Both new methods MUST reuse it unchanged.
- `animeenigma.org` is DNS-only (owner-authoritative): TLS terminates at the **host nginx**, so a client-cert handshake can reach us. ⚠ The new `cert.animeenigma.org` record was created **proxied (orange-cloud)** — it MUST be flipped to **DNS only** (A `152.53.160.135`, AAAA `2a0a:4cc0:c0:ccee:680e:d6ff:fe97:e81c`) or CF terminates TLS and mTLS silently breaks.
- Gateway proxies `/api/auth/*` to auth as a public wildcard — new passkey routes need no gateway changes.
- Settings surface: Profile.vue → Settings tab (cards: Privacy, Player, API Key, Active Sessions, Account).

## Architecture decision

**mTLS terminates on a dedicated host-nginx vhost `cert.animeenigma.org`** (own CA, `ssl_verify_client optional`), NOT on the main vhost. Rejected alternatives:

- *Main-vhost mTLS*: the main vhost speaks QUIC/HTTP-3 (nginx client-cert-over-h3 is unreliable); any TLS change there risks all production traffic; cert holders would get browser cert-picker prompts during ordinary browsing even with the toggle off.
- *Cloudflare mTLS*: site is not CF-proxied.

The dedicated vhost means the cert prompt can only ever appear when the frontend deliberately probes that subdomain.

---

## Part 1 — Passkeys (WebAuthn)

### Backend (`services/auth`)

- Dependency: `github.com/go-webauthn/webauthn`. Config: RP ID `animeenigma.org`, RP origin `https://animeenigma.org`, RP display name "AnimeEnigma" (env-overridable for dev: `WEBAUTHN_RP_ID`, `WEBAUTHN_RP_ORIGINS`).
- New GORM table `webauthn_credentials`:
  - `id` uuid PK · `user_id` uuid (index) · `credential_id` bytea/base64, unique · `public_key` bytea · `sign_count` uint · `transports` text · `aaguid` bytea · `name` varchar(64) (user label, default "Passkey N") · `created_at` · `last_used_at` · `deleted_at` (soft delete)
- Ceremony state (`webauthn.SessionData`) in Redis, TTL 5 min, key `authn:webauthn:{register|login}:<challenge-id>`.
- Registration requires **discoverable credentials** (`ResidentKeyRequirement: required`, `UserVerification: preferred`) so login is usernameless.

### Routes

| Route | Auth | Behavior |
|---|---|---|
| `POST /api/auth/passkey/login/begin` | public | assertion options, empty `allowCredentials` |
| `POST /api/auth/passkey/login/finish` | public | verify assertion; user resolved via `userHandle` (user UUID); update `sign_count`/`last_used_at`; → `createSessionAndAuthResponse` |
| `POST /api/auth/passkey/register/begin` | JWT | creation options for current user |
| `POST /api/auth/passkey/register/finish` | JWT | store credential (with optional `name` from FE) |
| `GET /api/auth/passkeys` | JWT | list (id, name, created_at, last_used_at) |
| `DELETE /api/auth/passkeys/{id}` | JWT | remove own credential |

Login throttling: reuse `loginthrottle` keyed by IP for `login/finish` failures.

### Frontend

- `Auth.vue`: small secondary button «🔑 Войти через passkey» under the QR card, rendered only when `window.PublicKeyCredential` exists. Flow: begin → `navigator.credentials.get` → finish → authStore hydrates session → redirect (same post-login path as Telegram).
- Errors (user cancel, no credential) → non-blocking toast, stay on login page.

## Part 2 — TLS client-certificate auto-login

### CA (auth service)

- On startup, auth ensures a platform user-CA exists: ECDSA P-256, ~20y validity, subject `CN=AnimeEnigma User CA`. Private key + cert PEM stored in a single-row GORM table `auth_ca` (backed up with the DB; acceptable trust model for a self-hosted friend group).
- CA public PEM exposed at `GET /api/auth/cert/ca.pem` (public, it's not secret) — also used by the host setup step to install `/etc/nginx/certs/ae-user-ca.pem`.

### Certificate issuance & management

- `POST /api/auth/cert/issue` (JWT): body `{name}`. Server generates ECDSA P-256 keypair + cert signed by the CA. Subject `CN=<username>`, serial random; validity **10 years**. Authorization NEVER trusts the subject — mapping is by fingerprint.
- New GORM table `user_certificates`: `id` uuid · `user_id` (index) · `name` varchar(64) · `fingerprint_sha256` char(64) unique · `serial` · `not_after` · `created_at` · `last_used_at` · `revoked_at` (nullable).
- Response: PKCS#12 (`.p12`) download + a generated import password (random, ~10 chars) **shown exactly once** in the modal (iOS refuses empty-password p12). Private key is not persisted server-side.
- `GET /api/auth/certs` (JWT): list own certs (name, created, last_used, not_after, revoked). `DELETE /api/auth/certs/{id}`: sets `revoked_at` (nginx still passes the handshake; auth rejects by fingerprint — no CRL).
- Auto-login toggle: new `User.CertAutoLogin bool` column, PATCHed via the existing profile-settings path. Server-side is the single source of truth; checked during handshake-login.

### nginx vhost (host-level, reference in `infra/nginx/cert.animeenigma.org.conf`)

- `listen 443 ssl` + `[::]:443 ssl` — **TCP only, no QUIC/h3** (client certs over h3 are unreliable in nginx).
- Let's Encrypt cert via the same certbot webroot pattern as `ext.animeenigma.org` (direct DNS → HTTP-01 works).
- `ssl_client_certificate /etc/nginx/certs/ae-user-ca.pem; ssl_verify_client optional; ssl_verify_depth 1;`
- Single location `GET /cert-login` → `proxy_pass http://127.0.0.1:8080/cert/handshake-login` (**directly to auth's loopback port, bypassing the gateway**) with headers:
  - `X-AE-Cert-Verify: $ssl_client_verify`
  - `X-AE-Cert-PEM: $ssl_client_escaped_cert`
  - `X-Real-IP: $remote_addr`
- CORS: `Access-Control-Allow-Origin: https://animeenigma.org` on responses (simple GET — no preflight).
- Everything else 404s. `limit_req` (auth-tier zone) + fail2ban coverage like other vhosts.
- **Trust boundary:** `/cert/handshake-login` is registered on auth's root mux (like the magic-link routes) and is *not* proxied by the gateway, so it is reachable ONLY via this vhost (or the internal Docker network). A forged `X-AE-Cert-PEM` from the public internet has no path to it. Auth additionally re-verifies the PEM's CA signature + validity window itself (defense in depth, and `optional` handshake means `X-AE-Cert-Verify` must equal `SUCCESS`).

### Auth endpoint `/cert/handshake-login` (auth root mux)

1. Require `X-AE-Cert-Verify == SUCCESS` and parse `X-AE-Cert-PEM`; verify signature chain against the stored CA + `NotAfter`.
2. SHA-256 fingerprint → `user_certificates` lookup; reject if unknown or `revoked_at != nil`; stamp `last_used_at`.
3. Load user; if `CertAutoLogin == false` → `403 {"reason":"auto_login_disabled"}` (FE treats as silent no-op and negative-caches).
4. Mint a **one-time login token** (magic-link machinery pattern: random token in Redis, TTL 60 s, single consume) and return `{token}`.

### Session bridge (main origin)

- `POST /api/auth/cert/consume` (public, via gateway): body `{token}` → consume one-time token → `createSessionAndAuthResponse` → normal cookies on `animeenigma.org`. Rate-limited like login.

### Frontend auto-login flow

- Router guard (unauthenticated → `/auth` path): before rendering `Auth.vue`, if `localStorage.ae_cert_suppress != '1'`, probe `https://cert.animeenigma.org/cert-login` with a **2.5 s timeout** while showing a minimal "checking" state:
  - Success → `POST /api/auth/cert/consume` → session → continue to the originally requested route. **The login page never renders.**
  - Any failure/timeout/403 → render `Auth.vue` normally. On `auto_login_disabled`, set a negative-cache flag (localStorage, 24 h) so toggled-off cert holders aren't re-prompted with the browser cert picker on every visit.
- Browser cert-picker: users *without* an AE cert see nothing (the CertificateRequest names only our CA). Users *with* one get the native picker approximately once per browser session (Firefox can remember; Chrome re-asks per session). Inherent to mTLS; accepted.
- **Logout suppression (owner-approved):** explicit logout sets `ae_cert_suppress=1` — auto-login is paused in this browser; the login page shows normally. Any successful manual login (Telegram or passkey) clears the flag, re-arming auto-login. Issuing a cert in this browser also clears it.

## Part 3 — Settings UI: «Продвинутый логин»

New card/button in Profile → Settings (between API Key and Active Sessions): **«Продвинутый логин»** → opens `AdvancedLoginModal.vue` with two sections:

1. **Passkeys** — list (name, created, last used), «Добавить passkey» (name input → WebAuthn create ceremony), delete with confirm.
2. **TLS-сертификат** —
   - Collapsible instructions: installing a `.p12` on Windows / macOS / iOS / Android / Linux (i18n).
   - «Выпустить сертификат» → name input → downloads `.p12`, shows the import password once (copy button, "you won't see it again" warning).
   - Cert list: name, created, last used, expiry, revoke (confirm dialog).
   - Toggle «Входить автоматически при обнаружении сертификата» (binds `CertAutoLogin`; disabled with a hint until ≥1 active cert exists).

Design-system tokens only; i18n en/ru/ja parity; `/frontend-verify` before finishing. The modal is a settings/logic surface (forms + lists), not a visual redesign — `design-prototyping` not required.

## Observability

- Auth Prometheus counters via `libs/metrics` (NOT plain promauto): `auth_login_total{method="telegram|passkey|cert"}`, `auth_passkey_ceremony_failures_total`, `auth_cert_handshake_total{outcome}`.
- Structured logs for issue/revoke/handshake-login.

## Testing

- Go unit: CA bootstrap idempotency; issue → verify chain; revoked/expired/foreign-CA rejection; fingerprint mapping; one-time token single-consume + TTL; webauthn register/login ceremonies with library test helpers; toggle-off 403.
- FE: vitest for the guard's probe/suppress/negative-cache logic (mock fetch); component tests for the modal.
- E2E: Playwright virtual authenticator (CDP `WebAuthn.enable`) for passkey happy path; if flaky, manual verification instead. mTLS path is verified manually with a real issued cert (curl `--cert` + browser).

## Deploy / ops checklist (owner + host steps)

1. ⚠ **CF DNS: flip `cert.animeenigma.org` to DNS only** (currently proxied — blocks everything).
2. Redeploy auth (CA auto-bootstraps) + gateway untouched; `curl :8080/cert/ca.pem > /etc/nginx/certs/ae-user-ca.pem` (helper `bin/ae-cert-ca-install.sh`).
3. Certbot cert for `cert.animeenigma.org`; install vhost from `infra/nginx/cert.animeenigma.org.conf`; `nginx -t && reload`.
4. FE env: `VITE_CERT_LOGIN_BASE=https://cert.animeenigma.org` (empty disables the probe entirely — dev default).
5. `/animeenigma-after-update` (lint, build, redeploy, health, changelog, push); mark feedback report `ai_done`.
