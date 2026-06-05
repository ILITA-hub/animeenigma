# Simplify Auth Sessions — Non-Rotating Server-Side Sessions

**Date:** 2026-06-05
**Status:** Approved (design)
**Supersedes mechanism in:** [`2026-05-14-persistent-sessions-design.md`](./2026-05-14-persistent-sessions-design.md)

## Problem

Users are intermittently logged out ("randomly logged out AGAIN"). The root cause is **refresh-token rotation**: every `/auth/refresh` single-uses the old refresh token and issues a new one. Concurrent refreshes (multiple tabs, or a tab + the gateway's admin-session middleware) make one caller present a token that was just rotated away → `401` → logout.

A large amount of machinery exists **solely to survive this race**, and that machinery is itself the source of edge-case bugs:

- Backend grace window: `previous_refresh_token_hash`, `grace_until`, `grace_opened_at`, `slideGraceUntil()`, CAS `Rotate`, `PreviousHashExists`, `GraceWindow`/`MaxGraceLifetime` constants.
- A `refresh_grace_lapsed` metric whose own comment calls it *"the 'random logout' symptom."*
- Frontend cross-tab coordination via Web Locks, plus `isRefreshing`/`refreshPromise`/`failedQueue`/`authExpired`.
- A legacy JWT-refresh-token upgrade path kept from before persistent sessions.

The system is over-engineered, and the over-engineering is what's buggy.

## Goals

1. **Effectively infinite login** — an active browser is never time-expired; the session lives until explicitly revoked.
2. **Revocable sessions from settings** — already built (`ActiveSessionsCard.vue` + `/api/auth/sessions`); must continue to work.
3. **Simplify** — delete the rotation/grace machinery that causes the logout bug.

## Decisions (locked)

- **Access JWT TTL:** 15m → **1h** (`JWT_ACCESS_TTL`). Revoke-device latency ≤ 1h.
- **Session expiry:** none — a session is alive iff `revoked_at IS NULL`.
- **No rotation:** the opaque session/refresh token is stable for the session's life.
- **No DB migration:** grace columns go dormant (GORM never drops columns); `expires_at` is kept and set to a far-future sentinel.

## Design

### Token model (unchanged shape, changed lifecycle)

- **Access JWT** — stateless, short (1h), validated cheaply downstream by all services via shared secret. Carries `sid` (session id). *Kept as-is; only the TTL changes.*
- **Session token** — opaque `rt_<64hex>`, sha256-hashed into `user_sessions.refresh_token_hash`, delivered as an httpOnly+Secure cookie. **Now non-rotating:** `/auth/refresh` validates it and mints a new access JWT but returns the *same* session token.

### Refresh flow (new)

```
POST /auth/refresh  (cookie: refresh_token=rt_…)
  → FindAliveByHash(sha256(rt))            // revoked_at IS NULL AND refresh_token_hash = ? AND expires_at > now
      hit  → Touch(session.id, ip, now, now+100yr)   // bump last_seen + ip, push expiry forward
           → mint 1h access JWT bound to session.id
           → 200 { access_token }          // same refresh token; handler re-sets cookie w/ fresh 10yr maxAge
      miss → 401 (clear cookie, soft logout)
```

No rotation, no grace, no `rotated bool`, no legacy JWT path.

### Backend changes

**`services/auth/internal/domain/session.go`**
- `IsAlive(now) = RevokedAt == nil`.
- Struct keeps the three grace fields as dormant columns (no reads/writes). Optional: annotate as deprecated. `ExpiresAt` stays (written far-future), used only as a cheap tripwire.

**`services/auth/internal/repo/session.go`** — delete:
- `Rotate`, `RotateResult`, `slideGraceUntil`, `PreviousHashExists`, `GraceWindow`, `MaxGraceLifetime`.

Add / change:
- `Touch(ctx, sessionID, ip string, lastSeen, expiresAt time.Time) error` — `UPDATE … SET last_seen_at, ip, expires_at WHERE id = ? AND revoked_at IS NULL`.
- `FindAliveByHash` — drop the `previous_refresh_token_hash OR (… grace_until …)` clause; becomes `revoked_at IS NULL AND expires_at > now AND refresh_token_hash = ?`.
- `Revoke`, `RevokeOthers`, `ListAlive` — keep (drop the now-meaningless `expires_at > now` predicate from these if convenient; not required).
- `Cleanup` — delete only `revoked_at < now-7d` (drop the expiry-based deletion; nothing meaningfully expires now).

**`services/auth/internal/service/auth.go`**
- `RefreshToken(ctx, req, sc) (*domain.AuthResponse, error)` — drop the `bool`. Body: `FindAliveByHash` → on hit `Touch` + mint access JWT + return resp **without** a new refresh token (cookie value is unchanged). On miss → `errors.Unauthorized`.
- Remove the legacy JWT-refresh branch (`ValidateRefreshToken`, the Redis `blacklist:` set/get) and the `refresh_grace_lapsed` / `refresh_cas_miss` / `session_legacy_upgraded` metrics.
- `SessionTTL` const → replace with `SessionExpirySentinel = 100 * 365 * 24 * time.Hour` (or similar). `createSessionAndAuthResponse` sets `ExpiresAt = now + sentinel`.
- `Logout` — keep the session-revoke path; drop the legacy blacklist fallback.

**`services/auth/internal/handler/auth.go`**
- `refreshTokenMaxAge` 30d → **10yr** (kept ≈ `SessionExpirySentinel` so the cookie outlives nothing).
- `RefreshToken` handler — always re-set the refresh cookie with the incoming `cookie.Value` and the fresh maxAge (no `rotated` conditional). Update the call site to the new single-return signature.

**`services/auth/internal/config/config.go` + `docker/docker-compose.yml`**
- `JWT_ACCESS_TTL` default 15m → 1h.

**`libs/authz/jwt.go`** — bump the `AccessTokenTTL` default to 1h if defaulted there too. If `GenerateTokenPair`'s JWT "refresh token" and `ValidateRefreshToken` become unreferenced after the legacy-path removal, delete them; otherwise leave untouched (out of scope to refactor shared lib consumers).

### Frontend changes — `frontend/web/src/api/client.ts`

- Remove the `navigator.locks.request('auth-refresh', …)` wrapper in `doTokenRefresh`; call `performRefresh()` directly. Concurrent same-token refreshes are now idempotent, so cross-tab coordination is unnecessary.
- Keep within-tab single-flight (`refreshPromise`), the `failedQueue` drain, and the `authExpired` latch — all still correct and simple.
- No change to `isTokenExpired` (still refreshes ~30s before the 1h exp).

### What does NOT change

- `ActiveSessionsCard.vue`, `useSessions.ts`, `api/sessions.ts`, and the `GET/DELETE /api/auth/sessions` + `revoke-others` routes — the revoke-from-settings feature already works; we only smoke-verify it.
- Downstream services' JWT validation.
- The gateway admin-session refresh middleware (it reads the same cookie; non-rotation makes it strictly safer).

## Migration & rollout

- **No schema migration.** Dormant columns remain; `expires_at` reused as sentinel.
- **Existing sessions** auto-upgrade to infinite on their next refresh (Touch pushes `expires_at` to the sentinel).
- **Legacy JWT-refresh cookies** (pre-persistent-sessions) get one `401 → re-login` after deploy. One-time, acceptable.
- Deploy order: redeploy `auth`, then `web`. No coordinated cutover needed (non-rotation is backward-compatible with cookies already in browsers).

## Tradeoffs

- **Loses refresh-token-theft detection** (rotation's benefit). Mitigations: httpOnly+Secure cookie, and the working revoke-device button. Consistent with the explicit "infinite token" intent for a small self-hosted group.
- Dormant columns are mild cruft; a later optional migration can drop them.

## Testing

Delete: `TestSessionRepo_RotateCASWinAndGracePath`, `TestSessionRepo_GraceSlides_*`, `TestSessionRepo_Rotate_GraceExpired_ThirdReplayFails`.

Add / update:
- `FindAliveByHash` returns the row for a valid hash; `NotFound` once revoked.
- `Touch` bumps `last_seen_at`/`ip` and pushes `expires_at`; no-ops on a revoked row.
- Refresh returns a **new access JWT** but the **same** refresh token; `last_seen_at` advanced.
- Revoked session → refresh yields `401`.
- `Cleanup` deletes only revoked-older-than-7d rows.

## Verification (post-deploy)

- Browser smoke: log in, idle past 1h, confirm a transparent refresh (no logout), open two tabs and force concurrent refreshes — both stay logged in.
- Profile → Settings → Active Sessions: list renders, "this device" badge correct, revoke another device works, revoke-others works.
- `make redeploy-auth && make redeploy-web && make health`.

## Out of scope

- Refactoring `libs/authz` consumers beyond removing now-dead refresh helpers.
- Dropping dormant columns via migration (optional follow-up).
- Any redesign of the Active Sessions UI.
