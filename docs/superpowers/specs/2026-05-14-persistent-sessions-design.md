# Persistent Sessions — Design

**Status:** Approved
**Date:** 2026-05-14
**Owner:** auth service
**Related fixes context:** `8ead9a4 fix(auth): keep session alive on transient refresh errors`, `80dd63b fix(auth): serialize token refresh across tabs + soft-logout on 401`

## Problem

Users keep getting logged out, even after the recent transient-error and cross-tab fixes. Two unaddressed root causes:

1. **Hard ceiling.** Refresh token is a 7-day JWT. After 7 idle days, relogin is mandatory. No client-side fix can move that wall.
2. **Stateless revocation.** Refresh tokens are single-use, blacklisted in Redis. There's no `sessions` table, so:
   - Any 401 from `/auth/refresh` (Redis flush, JWT secret change, blacklist hiccup) silently kicks the user with no row to inspect.
   - There's no way to list a user's active devices or revoke a single one — only "rotate the JWT secret" can revoke globally.

The user wants a token tier above the 15-min access token that effectively never expires once issued, plus a settings UI to inspect and revoke active sessions.

## Goals

- A logged-in user stays logged in indefinitely as long as they visit at least once a month.
- A user can view their active sessions and revoke any of them (including a "sign out everywhere else" action).
- When login does drop, there's a DB row that explains why.
- No regression for the existing 7-day-JWT cohort during rollout.

## Non-Goals

- "Remember me" checkbox. Self-hosted; persistence is always on.
- Replacing access tokens with opaque tokens. The gateway and downstream services keep validating statelessly.
- Sub-15-min revocation propagation in v1 (deferred to optional Redis allowlist later).
- Multi-factor auth, device approval flow, location-based session blocking.

## Architecture

### Token model

| Token | Form | Lifetime | Storage | Validation |
|---|---|---|---|---|
| Access | JWT (HS256) | 15 min (unchanged) | `localStorage.token` + `access_token` httpOnly cookie | stateless, includes `sid` claim |
| Refresh | opaque random 32-byte string, hex-encoded with `rt_` prefix | sliding 30 days, bumped on every refresh | `refresh_token` httpOnly cookie at `/api/auth` (unchanged path) | DB lookup by `sha256(token)` against `user_sessions.refresh_token_hash` |

Access JWTs gain a `sid` claim (session UUID) so future per-request revocation checks can target a session, not a user.

### `user_sessions` table

```sql
CREATE TABLE user_sessions (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash          CHAR(64) NOT NULL,                     -- hex-encoded sha256, current
    previous_refresh_token_hash CHAR(64),                              -- previous RT hash, valid for grace_until
    grace_until                 TIMESTAMPTZ,                           -- previous_hash accepted until this time
    user_agent                  TEXT NOT NULL DEFAULT '',
    ip                          INET,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at                  TIMESTAMPTZ NOT NULL,
    revoked_at                  TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_user_sessions_rt_hash          ON user_sessions(refresh_token_hash);
CREATE        INDEX idx_user_sessions_prev_rt_hash     ON user_sessions(previous_refresh_token_hash) WHERE previous_refresh_token_hash IS NOT NULL;
CREATE        INDEX idx_user_sessions_user_id_alive    ON user_sessions(user_id) WHERE revoked_at IS NULL;
CREATE        INDEX idx_user_sessions_expires_at_alive ON user_sessions(expires_at) WHERE revoked_at IS NULL;
```

GORM handles the actual DDL via `AutoMigrate`; the SQL above documents the intended shape.

A session row is **alive** iff `revoked_at IS NULL AND expires_at > now()`.

### Flows

**Login / Register / Telegram-confirm** — after the existing user-resolution logic:
1. Generate access JWT (15 min) with `sid = new uuid`.
2. Generate opaque refresh token `rt_<64-hex>`.
3. Insert `user_sessions` row: `id = sid`, `refresh_token_hash = sha256(rt)`, `user_agent` from `User-Agent` header (truncated to 1024 chars), `ip` from request, `expires_at = now() + 30d`.
4. Set both cookies as today.

**Refresh:**
1. Read `refresh_token` cookie. Compute `hash = sha256(value)`.
2. **Try DB lookup first** — find an alive session where `refresh_token_hash = hash` OR (`previous_refresh_token_hash = hash` AND `grace_until > now()`).
   - **Match on `refresh_token_hash`** — rotate. Atomically: generate new RT, `UPDATE user_sessions SET previous_refresh_token_hash = refresh_token_hash, refresh_token_hash = sha256(new_rt), grace_until = now() + 30s, last_seen_at = now(), expires_at = now() + 30d, ip = req.ip WHERE id = ? AND refresh_token_hash = old_hash` (compare-and-swap on `refresh_token_hash` to detect concurrent rotation). If the UPDATE affects 0 rows, jump to the next bullet (the other tab's rotation already happened). If 1 row, mint new access JWT with the existing `sid` and return both cookies.
   - **Match on `previous_refresh_token_hash` (grace window)** — another tab just rotated. Don't rotate again; mint a fresh access JWT for this `sid` and return it. **Re-set the `refresh_token` cookie to the value the winning tab wrote** — but we don't know its raw value (only the hash). So instead: set Set-Cookie with the value that the client SHOULD now have — which we recover by reading `localStorage.token` is impossible server-side. The pragmatic fix: just return the new access JWT with no Set-Cookie for refresh; the client's cross-tab `storage` listener (already in `stores/auth.ts`) will adopt the winning tab's tokens within milliseconds. Worst case: the next refresh attempt fires with the old cookie, hits the grace path again (still within 30s), and gets another fresh access JWT. After grace expires, the loser tab's cookie is dead — but by then, the cross-tab sync has long since corrected it.
   - **No match** — fall through to legacy path.
3. **Legacy path** (transition window only): try parsing the cookie as a JWT refresh token. If valid and not in the Redis blacklist, treat it as a successful login: create a new `user_sessions` row, mint new access + opaque refresh, blacklist the legacy JWT, return.
4. If all paths fail: clear both cookies, return 401.

**Logout:**
1. Read `refresh_token` cookie. If matches a session row, set `revoked_at = now()`.
2. Clear both cookies.

**List sessions** — `GET /api/auth/sessions`:
- Return all alive sessions for the authed user. Each entry: `id`, `user_agent`, `ip`, `created_at`, `last_seen_at`, `expires_at`, `is_current` (true iff `id == claims.sid`).

**Revoke one** — `DELETE /api/auth/sessions/:id`:
- Only revokes if the row's `user_id` matches the authed user. `UPDATE … SET revoked_at = now() WHERE id = ? AND user_id = ? AND revoked_at IS NULL`.
- If the revoked row is the current session, also clear cookies + return 204 (the next access-token expiry will complete the boot).

**Revoke all others** — `POST /api/auth/sessions/revoke-others`:
- `UPDATE user_sessions SET revoked_at = now() WHERE user_id = ? AND id != current_sid AND revoked_at IS NULL`.
- Returns count of revoked rows.

### Race-proof rotation

The DB grace window replaces today's "RT1 invalidated mid-flight, tab B gets a real 401" failure mode:

- Tab A and tab B both POST `/auth/refresh` with the same RT (`hash_old`).
- Tab A's CAS `UPDATE` succeeds → row now has `refresh_token_hash = hash_new_A`, `previous_refresh_token_hash = hash_old`, `grace_until = now() + 30s`. Tab A's response sets cookie to `RT_A`.
- Tab B's CAS predicate `WHERE refresh_token_hash = hash_old` matches 0 rows.
- Tab B re-queries: no match on `refresh_token_hash`, **but match on `previous_refresh_token_hash` while `grace_until > now()`**. Tab B mints a fresh access JWT for the same `sid` and returns it without rotating. **Tab B does NOT issue a Set-Cookie for refresh** — the cross-tab `storage` listener will sync `RT_A` from tab A within ms.
- If tab B's cookie still hasn't been updated by the next access-token expiry, its next refresh either (a) fires within the 30s grace window and gets another grace-path response, or (b) hits a legitimate 401 — but by then the Web Lock + storage sync from `80dd63b` should have populated `localStorage.token` from tab A's mint.

Net effect: cross-tab races become invisible to the user even if the Web Lock fails (e.g., stale browser without Web Locks support), as long as the second tab's next request lands within the 30s grace window.

### Settings UI

New "Active Sessions" card in `/profile/settings` (or wherever the existing API Key card lives — verified during plan):

- Lists each alive session row.
- Each row shows: parsed UA (e.g., "Chrome 137 on Linux") via lightweight UA parsing on the frontend, last-seen relative time, IP (no geolocation in v1 — just the raw IP), and a "Revoke" button.
- The current session is marked with a "this device" badge and has no Revoke button (use Logout for that).
- A "Sign out everywhere else" button at the bottom calls `/api/auth/sessions/revoke-others` with confirmation.

### Migration

GORM `AutoMigrate(&domain.UserSession{})` creates the table on auth-service startup. No manual SQL migration step.

Legacy JWT refresh tokens stay valid via the dual-path refresh handler. After the legacy 7-day TTL elapses (i.e., 7 days after the deploy), drop the legacy fallback in a follow-up commit. No urgency — keeping it longer is harmless.

### Cleanup

A single goroutine in the auth service runs hourly:
```sql
DELETE FROM user_sessions
 WHERE (revoked_at IS NOT NULL AND revoked_at < now() - interval '7 days')
    OR expires_at < now() - interval '7 days';
```
Revoked rows linger for 7 days so a user who clicks "revoke" can still see what they killed if they refresh the settings page within a week (UI only shows alive rows by default, but the row exists for audit). Expired rows linger 7 days to absorb cookie clock skew.

## Security

- Refresh token never persisted in raw form (only sha256 hash).
- `rt_` prefix lets anti-secret scanning catch leaks.
- IP and UA captured for the user's own visibility, not for auth decisions (no IP-binding — kills mobile users on cell-tower handoff).
- Revocation is `user_id`-scoped; one user can never revoke another's session.
- The `sid` in the access JWT is a UUID; even if leaked, an attacker can't use it without the access JWT signature.

## Observability

Add `metrics.AuthEventsTotal` labels:
- `session_created` / `session_revoked` / `session_expired` / `session_legacy_upgraded` / `refresh_cas_miss`.

Log on every session create/revoke at info level with `user_id`, `session_id`, `reason`.

## Test plan

- Unit: `user_sessions` repo CRUD, CAS rotation correctness, hash collision behavior.
- Service: legacy JWT → opaque session upgrade path; double-refresh CAS race; revoke-others excludes current session.
- E2E (Playwright on `ui_audit_bot`): login → list sessions shows current → open second incognito → list shows two → revoke other → second context's next access-token refresh fails → second context bounces.
- Manual: 30-day clock test stubbed via `JWT_REFRESH_TTL=30s` for one container.

## Out of scope (explicit)

- Geolocation lookup on the IP.
- Email notification on new-session.
- 2FA.
- Per-session permissions (everything is full-user).
- Sub-15-min revocation propagation. (Add later via Redis set of revoked `sid`s checked by gateway middleware if needed.)

## Open questions resolved during brainstorming

- **Refresh TTL:** 30 days, sliding. Approved.
- **Instant revocation:** deferred. 15-min propagation is acceptable in v1.
- **Legacy migration window:** open-ended. Drop legacy path in a follow-up after 7+ days.
- **"Remember me" checkbox:** no. Always-on persistence.
