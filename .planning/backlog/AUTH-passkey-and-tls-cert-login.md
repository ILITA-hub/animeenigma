---
id: AUTH-passkey-and-tls-cert-login
title: Passkey (WebAuthn/FIDO2) login and TLS client-certificate login
captured_at: 2026-07-23
captured_during: admin TODO capture (Telegram, @tNeymik)
target_milestone: unscheduled / TBD
deferred_from: N/A (net-new request)
status: backlog
depends_on: n/a — additive to existing Telegram + password + magic-link auth
---

# Passkey login and TLS client-certificate login

## Original request

Admin TODO («Туду: сделать вход по паски и по тлс сертификату») — add two new
login methods: passkey (WebAuthn/FIDO2) and TLS client-certificate auth.

## Current state (context, not yet verified against live code)

`services/auth/internal/{handler,service}/` already has `telegram_bot.go`,
`magiclink.go`, `passwordhash.go`, and `sessions.go` — so the method-picker pattern
from `[[AUTH-email-magic-link]]` (Telegram / email-magic-link / password) is the
precedent to extend, not a green field. Both new methods are **additive** auth
options on the existing session/JWT model (`[[project_auth_nonrotating_sessions]]`),
not a replacement.

## Scope

### 1. Passkey / WebAuthn (FIDO2) login
- Server: WebAuthn relying-party registration + assertion ceremony (a Go WebAuthn
  library, e.g. `go-webauthn/webauthn`), credential storage (public key + credential
  ID per user, new table), challenge issuance/verification endpoints in
  `services/auth`.
- Client: `navigator.credentials.create()` / `.get()` wiring in `Auth.vue`'s method
  picker, platform-authenticator UX (Face ID / Windows Hello / hardware key), and a
  "manage passkeys" panel in account settings for registering/revoking credentials.
- Requires HTTPS + a stable RP ID (`animeenigma.org`) — already satisfied.

### 2. TLS client-certificate login (mutual TLS)
- Requires the TLS termination point (currently nginx/gateway, per
  `docs/k8s-deploy.md` — need to confirm current ingress TLS config) to request and
  verify a client certificate (`ssl_verify_client optional` + forward the verified
  cert/subject to the gateway via a header), then a mapping from certificate
  subject/fingerprint → AnimeEnigma user account, plus a self-service cert
  issuance/enrollment flow (users need to obtain a client cert somehow — either
  self-generated + upload the public cert/fingerprint, or the platform acts as its
  own mini-CA).
- This is the more architecturally invasive of the two: it touches the TLS
  termination layer (ingress/nginx config), not just an application-level auth
  service, and needs a decision on certificate issuance UX before implementation
  starts (self-generated-and-registered vs. platform-issued).

## Why deferred (not done inline)

- Admin explicitly asked for a TODO capture ("Туду: сделать…"), not an immediate
  build — per the maintenance-bot feedback-store rules, a capture request records
  future work and is recorded as backlog, not implemented; the feedback entry stays
  a single open task.
- Both are genuine feature requests (new auth surfaces), which the maintenance bot
  never auto-implements regardless of risk tier.
- TLS client-cert auth in particular needs an explicit decision on certificate
  issuance/enrollment UX (self-service vs. platform-as-CA) before any planning is
  useful — that's a product/security call, not something to default silently.
- Auth is a high-blast-radius surface (session/JWT model, `libs/errors`-wrapped
  handlers, existing Telegram/password/magic-link flows) — changes here should go
  through a real design/brainstorm pass, not a mechanical maintenance-bot edit.

## Cost estimate

| Component | Effort (Fib) | Risk |
|---|---|---|
| WebAuthn server (registration + assertion ceremony, credential storage) | 13 | Medium — new dependency, crypto-adjacent |
| WebAuthn client UX (`Auth.vue` method picker + passkey management panel) | 8 | Low |
| TLS client-cert verification at ingress (nginx/gateway config + subject→user mapping) | 13 | High — touches TLS termination layer, deploy-sensitive |
| Certificate issuance/enrollment UX (self-service or platform-CA) | 21 | High — open product decision, no default exists |
| i18n (en/ru/ja) for both new flows | 2 | Low |
| **Total** | **~57** | |

## Cross-references

- Precedent for a multi-method auth picker: `[[AUTH-email-magic-link]]`
- Session model: `[[project_auth_nonrotating_sessions]]`
- Existing auth service layout: `services/auth/internal/{handler,service}/`
  (`telegram_bot.go`, `magiclink.go`, `passwordhash.go`, `sessions.go`)
- TLS/ingress reference (to confirm current termination point before scoping mTLS):
  `docs/k8s-deploy.md`
- Source feedback: `2026-07-23T15-56-53_tNeymik_telegram`
