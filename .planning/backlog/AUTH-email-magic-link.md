---
id: AUTH-email-magic-link
title: Email magic-link as universal auth fallback (Tier 1)
captured_at: 2026-05-11
captured_during: auth UX discussion
target_milestone: v2.1 (TBD)
deferred_from: 2026-05-11 Tier 0 auth polish session
status: backlog
priority: high
---

# Email magic-link — universal login fallback

## Problem

Telegram deep-link is the only login path. It works for the majority of RU users
who have the native app installed, but leaves three segments stranded:

1. **tg-web-only users.** Even with the Tier 0 manual `/start TOKEN` affordance,
   the flow requires the user to navigate between two browser tabs and paste a
   token. It works, but it's the worst UX on the site.
2. **RU users with TG carrier-throttled / blocked.** Roskomnadzor signals are
   trending hostile in 2026. A user whose carrier silently throttles Telegram
   has no way to authenticate at all today.
3. **Non-RU users without Telegram.** Not the primary audience, but the site is
   multi-locale (EN/RU/JA) and we should not require Telegram in EN/JA contexts.

## Scope

Add `email magic-link` as a parallel auth method. **Not** replacing Telegram —
adding a second method. Refactor `Auth.vue` into a method picker:

```
Sign in to AnimeEnigma
┌─────────────────────────────┐
│  🔵 Continue with Telegram   │  → existing QR + tg:// + tg-web fallback (Tier 0)
└─────────────────────────────┘
            or
┌─────────────────────────────┐
│  ✉️  Email magic link        │  → new flow
└─────────────────────────────┘
```

## Backend work

1. New table `email_auth_tokens` (or Redis-only, like the existing `tgauth:` keys):
   - `token` (UUID), `email` (lowercased), `created_at`, `expires_at` (15 min),
     `consumed_at` (nullable, single-use)
2. New endpoints under `services/auth/internal/handler/`:
   - `POST /api/auth/email/request` — `{ email }` → enqueue mail, return `{ status: "sent" }`
     - Rate-limit: 3 requests per email per hour
     - Anti-enumeration: always return 200 even for non-existent emails (don't leak who has an account)
   - `GET /api/auth/email/verify?token=X` — looks up token, on success
     creates/links a user, sets refresh + access cookies, redirects to `/`
3. Transactional mail integration:
   - Option A: **Resend.com** (free 3k/mo, simplest API)
   - Option B: **Yandex Postmaster** (free 50/day for RU IPs, best RU deliverability)
   - Option C: **Mailgun** (free 5k/mo for 3 months)
   - Recommend A for speed, switch to B once mail volume justifies it
4. User model:
   - Add `Email *string` to `domain.User` with `uniqueIndex` (nullable — TG-only
     users keep their NULL email)
   - On first email-link claim, prompt user to optionally link their existing TG
     account if logged in via TG previously (or create a new account)

## Frontend work

1. `Auth.vue` — method picker (TG default, Email below)
2. New view `EmailLinkSent.vue` — "Check your inbox" screen with retry option
3. Update i18n for all three locales

## Security considerations

- Token must be cryptographically random (use `crypto/rand`, not UUID v4)
- HMAC the token in the database so a DB breach doesn't allow login impersonation
- Single-use enforcement (mark consumed before issuing JWT)
- Constant-time comparison on lookup
- Rate-limit at gateway level too, not just service level
- SPF + DKIM + DMARC on the outbound mail domain

## Effort estimate

~2 dev days end-to-end:
- Day 1: backend (DB migration, endpoints, mail provider integration, rate-limit)
- Day 2: frontend (method picker refactor, mail-sent screen, i18n) + e2e test

## Why this is the right Tier 1

- One additive method covers all three failed segments
- Email is universal in 2026, no third-party app dependency
- Cheaper than OAuth (no app registration with VK/Yandex/Google)
- Better UX than password login (no "forgot password" flows)
- Defensible against the "Russia loses Telegram next year" scenario

## Out of scope (defer)

- VK ID / Yandex ID OAuth — solves only RU, email already covers it
- Password login — strictly worse UX than magic link, more security tax
- SMS — costs $0.01-0.10/msg, abuseable, same carrier risk as TG
