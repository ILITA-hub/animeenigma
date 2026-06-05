# Simplify Auth Sessions (Non-Rotating) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop refresh-token rotation so the intermittent "random logout" race disappears, make sessions live until explicitly revoked (effectively infinite login), and raise the access JWT to 1h — while keeping the already-built revoke-from-settings feature.

**Architecture:** Server-side opaque session tokens become **non-rotating**: `/auth/refresh` validates the token, bumps `last_seen`, mints a fresh 1h access JWT, and returns the *same* refresh token. This deletes the grace-window machinery (CAS rotate, previous-hash, sliding grace) and the frontend cross-tab Web Locks that only existed to survive rotation races. No DB migration — the three grace columns go dormant and `expires_at` is reused as a far-future sentinel.

**Tech Stack:** Go (chi, GORM, golang-jwt), Postgres, Redis, Vue 3 + axios. Auth service at `services/auth`. Integration tests are `//go:build integration` and need the dev stack (`make dev`) running on localhost:5432 / :6379.

**Spec:** `docs/superpowers/specs/2026-06-05-simplify-auth-sessions-design.md`

---

## File Structure

| File | Change | Responsibility after change |
|------|--------|------------------------------|
| `services/auth/internal/domain/session.go` | Modify | `IsAlive = RevokedAt == nil`; grace fields marked dormant |
| `services/auth/internal/repo/session.go` | Modify (large delete) | Non-rotating lookup + `Touch`; no grace/CAS |
| `services/auth/internal/repo/session_test.go` | Modify | Drop grace/rotate tests; add `Touch` test |
| `services/auth/internal/service/auth.go` | Modify | `RefreshToken` non-rotating, single return; sentinel expiry; no legacy path |
| `services/auth/internal/service/auth_session_test.go` | Rewrite | Non-rotating refresh expectations; legacy test removed |
| `services/auth/internal/handler/auth.go` | Modify | New `RefreshToken` signature; cookie maxAge → 10yr; always re-set cookie |
| `services/auth/internal/config/config.go` | Modify | `JWT_ACCESS_TTL` default 15m → 1h |
| `libs/authz/jwt.go` | Modify | `DefaultJWTConfig` access TTL 15m → 1h |
| `docker/docker-compose.yml` | Modify | Document `JWT_ACCESS_TTL` on the auth service |
| `frontend/web/src/api/client.ts` | Modify | Remove Web Locks block in `doTokenRefresh` |

**Note on test infra:** Steps that run integration tests assume `make dev` is up. Run them with:
`INTEGRATION=1 go test -tags=integration ./services/auth/internal/<pkg>/... -run <Name> -v`

---

## Task 1: Domain — session is alive until revoked

**Files:**
- Modify: `services/auth/internal/domain/session.go`

- [ ] **Step 1: Change `IsAlive` and mark grace columns dormant**

In `services/auth/internal/domain/session.go`, replace the `IsAlive` method and annotate the three grace fields. Find:

```go
	GraceUntil               *time.Time `json:"-"` // nil = no grace window active; set on rotation, cleared when window expires
	GraceOpenedAt            *time.Time `json:"-"` // when the current grace window first opened (last real rotation); bounds how far the window can slide
```

Replace the two lines' trailing comments with a dormant marker (keep the columns so GORM does not need to drop anything):

```go
	GraceUntil               *time.Time `json:"-"` // DORMANT (non-rotating sessions): column kept, never read/written
	GraceOpenedAt            *time.Time `json:"-"` // DORMANT (non-rotating sessions): column kept, never read/written
```

Also update the `PreviousRefreshTokenHash` comment to mark it dormant:

```go
	PreviousRefreshTokenHash *string    `gorm:"type:char(64);index:idx_user_sessions_prev_rt_hash" json:"-"` // DORMANT (non-rotating sessions): column kept, never read/written
```

Then replace:

```go
// IsAlive reports whether the session can be used for refresh.
func (s *UserSession) IsAlive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}
```

with:

```go
// IsAlive reports whether the session can be used for refresh. With
// non-rotating sessions there is no time wall: a session lives until it is
// explicitly revoked. ExpiresAt is kept as a far-future sentinel and is not
// consulted here (the `now` arg is retained for signature stability).
func (s *UserSession) IsAlive(now time.Time) bool {
	_ = now
	return s.RevokedAt == nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/auth/internal/domain/...`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add services/auth/internal/domain/session.go
git commit -m "refactor(auth): session is alive until revoked (no time wall)"
```

---

## Task 2: Repo — delete rotation/grace, add Touch

**Files:**
- Modify: `services/auth/internal/repo/session.go`
- Test: `services/auth/internal/repo/session_test.go`

- [ ] **Step 1: Update the failing tests first (drop grace/rotate, add Touch)**

In `services/auth/internal/repo/session_test.go`, **delete** these four test functions entirely:
- `TestSessionRepo_RotateCASWinAndGracePath`
- `TestSessionRepo_GraceSlides_SurvivesBeyondOriginalWindow`
- `TestSessionRepo_GraceSlide_BoundedCap`
- `TestSessionRepo_Rotate_GraceExpired_ThirdReplayFails`

Then **replace** `TestSessionRepo_Cleanup_DeletesStaleRowsOnly` with a revoked-only version, and **add** a `Touch` test. Append/replace with:

```go
func TestSessionRepo_Touch_BumpsLastSeenAndExpiry(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	old := time.Now().Add(-48 * time.Hour)
	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: padHash("touch"),
		UserAgent:        "go-test",
		IP:               "1.1.1.1",
		LastSeenAt:       old,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, r.Create(context.Background(), s))

	far := time.Now().Add(1000 * time.Hour)
	require.NoError(t, r.Touch(context.Background(), s.ID, "2.2.2.2", time.Now(), far))

	got, err := r.FindAliveByHash(context.Background(), s.RefreshTokenHash)
	require.NoError(t, err)
	require.Equal(t, "2.2.2.2", got.IP)
	require.True(t, got.LastSeenAt.After(old), "last_seen should advance")
	require.True(t, got.ExpiresAt.After(time.Now().Add(900*time.Hour)), "expiry should be pushed far out")
}

func TestSessionRepo_Touch_NoOpOnRevoked(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: padHash("revoked"),
		UserAgent:        "go-test",
		LastSeenAt:       time.Now(),
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, r.Create(context.Background(), s))
	require.NoError(t, r.Revoke(context.Background(), s.ID, userID))

	// Touch must not resurrect a revoked session into the alive set.
	_ = r.Touch(context.Background(), s.ID, "3.3.3.3", time.Now(), time.Now().Add(1000*time.Hour))
	_, err := r.FindAliveByHash(context.Background(), s.RefreshTokenHash)
	require.Error(t, err, "revoked session must stay un-findable")
}

func TestSessionRepo_Cleanup_DeletesRevokedOlderThan7d(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	// Revoked 8 days ago → should be deleted.
	staleRevoked := &domain.UserSession{
		UserID: userID, RefreshTokenHash: padHash("stale"),
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(1000 * time.Hour),
		RevokedAt: ptrTime(time.Now().Add(-8 * 24 * time.Hour)),
	}
	require.NoError(t, r.Create(context.Background(), staleRevoked))

	// Alive (never revoked), far-future expiry → must survive.
	alive := &domain.UserSession{
		UserID: userID, RefreshTokenHash: padHash("alive"),
		LastSeenAt: time.Now(), ExpiresAt: time.Now().Add(1000 * time.Hour),
	}
	require.NoError(t, r.Create(context.Background(), alive))

	n, err := r.Cleanup(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, int64(1))

	_, err = r.FindAliveByHash(context.Background(), alive.RefreshTokenHash)
	require.NoError(t, err, "alive session must survive cleanup")
}

// padHash builds a deterministic 64-char hash from a prefix.
func padHash(prefix string) string {
	raw := prefix + uuid.NewString()
	for len(raw) < 64 {
		raw += "0"
	}
	return raw[:64]
}

func ptrTime(t time.Time) *time.Time { return &t }
```

If `TestSessionRepo_CreateAndFindByHash` defines a local `hashLen64` helper, leave it; the new tests use `padHash`. If there is a name collision, rename the local one. Ensure `uuid` is imported (it already is).

- [ ] **Step 2: Run the repo tests — expect COMPILE FAILURE (no `Touch`, grace funcs referenced gone)**

Run: `cd /data/animeenigma && INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -run 'Touch|Cleanup' -v`
Expected: build error — `r.Touch undefined` (method not implemented yet). This confirms the test drives the change.

- [ ] **Step 3: Rewrite `session.go` — delete grace/rotation, add `Touch`, simplify queries**

Open `services/auth/internal/repo/session.go`. **Delete** the following entirely:
- The `GraceWindow` and `MaxGraceLifetime` consts (lines ~15-32) and their doc comments.
- The `slideGraceUntil` func.
- The `RotateResult` struct.
- The `Rotate` method.
- The `PreviousHashExists` method.

**Replace** `FindAliveByHash` with the non-rotating version:

```go
// FindAliveByHash returns the alive (not revoked) session whose current hash
// equals `hash`. ExpiresAt is a far-future sentinel and only acts as a cheap
// tripwire. Returns NotFound if none.
func (r *SessionRepository) FindAliveByHash(ctx context.Context, hash string) (*domain.UserSession, error) {
	now := time.Now()
	var s domain.UserSession
	err := r.db.WithContext(ctx).
		Where("revoked_at IS NULL AND expires_at > ? AND refresh_token_hash = ?", now, hash).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("session")
		}
		return nil, fmt.Errorf("find session by hash: %w", err)
	}
	return &s, nil
}
```

**Add** the `Touch` method (place it where `Rotate` was):

```go
// Touch records activity on a refresh: bumps last_seen_at + ip and pushes
// expires_at to the caller-supplied (far-future) sentinel. No-op on a revoked
// row. This replaces the old rotate-on-refresh: the refresh token itself is
// stable, so the only per-refresh write is this activity stamp.
func (r *SessionRepository) Touch(ctx context.Context, sessionID, ip string, lastSeen, expiresAt time.Time) error {
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Updates(map[string]any{
			"last_seen_at": lastSeen,
			"ip":           ip,
			"expires_at":   expiresAt,
		})
	if res.Error != nil {
		return fmt.Errorf("touch session: %w", res.Error)
	}
	return nil
}
```

**Replace** `Cleanup` so it only removes long-revoked rows (nothing meaningfully expires now):

```go
// Cleanup removes sessions revoked more than 7 days ago. Non-rotating sessions
// never time-expire, so there is no expiry-based deletion.
func (r *SessionRepository) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	res := r.db.WithContext(ctx).
		Where("revoked_at IS NOT NULL AND revoked_at < ?", cutoff).
		Delete(&domain.UserSession{})
	if res.Error != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}
```

Leave `Create`, `Revoke`, `RevokeOthers`, `ListAlive` as-is.

- [ ] **Step 4: Verify the package builds**

Run: `cd /data/animeenigma && go build ./services/auth/internal/repo/...`
Expected: no output. (If `slideGraceUntil`/`GraceWindow` are still referenced anywhere, the build names the file:line — remove that reference.)

- [ ] **Step 5: Run the repo tests — expect PASS**

Run: `cd /data/animeenigma && INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -v`
Expected: PASS (including the new `Touch`/`Cleanup` tests; grace tests are gone).

- [ ] **Step 6: Commit**

```bash
git add services/auth/internal/repo/session.go services/auth/internal/repo/session_test.go
git commit -m "refactor(auth): drop rotate/grace machinery, add Touch (non-rotating sessions)"
```

---

## Task 3: Service — non-rotating RefreshToken, sentinel expiry, no legacy path

**Files:**
- Modify: `services/auth/internal/service/auth.go`
- Test: `services/auth/internal/service/auth_session_test.go`

- [ ] **Step 1: Rewrite the service test to non-rotating expectations**

In `services/auth/internal/service/auth_session_test.go`:

**Delete** `TestRefreshToken_LegacyJWT_UpgradesToSession` entirely (the legacy upgrade path is being removed).

**Replace** `TestRefreshToken_PersistentPath_RotatesAndReturnsNewRT` with:

```go
func TestRefreshToken_NonRotating_SameTokenFreshAccess(t *testing.T) {
	svc, _, sRepo := newTestService(t)
	ctx := context.Background()
	sc := service.SessionContext{UserAgent: "go-test", IP: "1.2.3.4"}

	username := "u_" + time.Now().Format("150405") + "_a"
	resp, err := svc.Register(ctx, &domain.RegisterRequest{Username: username, Password: "password123"}, sc)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(resp.RefreshToken, "rt_"))
	first := resp.RefreshToken

	// Refresh: same refresh token returned, brand-new access token.
	resp2, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, service.SessionContext{UserAgent: "go-test", IP: "5.6.7.8"})
	require.NoError(t, err)
	require.NotEmpty(t, resp2.AccessToken)
	require.NotEqual(t, resp.AccessToken, resp2.AccessToken, "a fresh access token should be minted")

	// Refresh again with the SAME token — still works (no rotation, no race).
	resp3, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, sc)
	require.NoError(t, err)
	require.NotEmpty(t, resp3.AccessToken)

	// IP was updated by Touch on the 2nd refresh.
	sessions, err := sRepo.ListAlive(ctx, resp.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "1.2.3.4", sessions[0].IP, "last refresh sc.IP wins")
}

func TestRefreshToken_RevokedSession_Fails(t *testing.T) {
	svc, _, sRepo := newTestService(t)
	ctx := context.Background()
	sc := service.SessionContext{UserAgent: "go-test", IP: "1.2.3.4"}

	username := "u_" + time.Now().Format("150405") + "_b"
	resp, err := svc.Register(ctx, &domain.RegisterRequest{Username: username, Password: "password123"}, sc)
	require.NoError(t, err)

	sessions, err := sRepo.ListAlive(ctx, resp.User.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, sRepo.Revoke(ctx, sessions[0].ID, resp.User.ID))

	_, err = svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: resp.RefreshToken}, sc)
	require.Error(t, err, "refresh on a revoked session must fail")
}
```

(Note: `resp.User.ID` is available because `Register` returns the user. If `AuthResponse.User` is a pointer, this already works as written.)

- [ ] **Step 2: Run the service test — expect COMPILE FAILURE (signature still returns 3 values)**

Run: `cd /data/animeenigma && INTEGRATION=1 go test -tags=integration ./services/auth/internal/service/... -run NonRotating -v`
Expected: build error — `assignment mismatch: 2 variables but svc.RefreshToken returns 3 values`. Confirms the test drives the signature change.

- [ ] **Step 3: Replace `SessionTTL` with a far-future sentinel**

In `services/auth/internal/service/auth.go`, replace:

```go
// SessionTTL is the sliding-window length. Every refresh extends a session
// to now+SessionTTL. 30 days = "user opens the site at least once a month".
const SessionTTL = 30 * 24 * time.Hour
```

with:

```go
// SessionExpirySentinel makes a session effectively non-expiring. We keep the
// expires_at column (so no schema migration) but set it ~100 years out and
// never let it lapse for an active session. A session ends only on revoke.
const SessionExpirySentinel = 100 * 365 * 24 * time.Hour
```

- [ ] **Step 4: Rewrite `RefreshToken` (non-rotating, single return, no legacy path)**

Replace the entire `RefreshToken` method (the persistent-path + legacy-path + observability block) with:

```go
func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *domain.RefreshRequest,
	sc SessionContext,
) (*domain.AuthResponse, error) {
	hash := hashRefreshToken(req.RefreshToken)

	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err != nil {
		// Not alive / unknown / revoked — generic auth failure.
		return nil, errors.Unauthorized("invalid refresh token")
	}

	// Non-rotating: stamp activity, keep the same refresh token.
	now := time.Now()
	if terr := s.sessionRepo.Touch(ctx, session.ID, sc.IP, now, now.Add(SessionExpirySentinel)); terr != nil {
		return nil, terr
	}

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, session.ID)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	metrics.AuthEventsTotal.WithLabelValues("refresh_token", "success").Inc()
	return &domain.AuthResponse{
		AccessToken: pair.AccessToken,
		ExpiresAt:   pair.ExpiresAt,
		User:        user,
		// RefreshToken intentionally empty: the cookie value is unchanged.
	}, nil
}
```

This removes the use of `generateRefreshToken`/`hashRefreshToken(newRT)`/`Rotate`, `ValidateRefreshToken`, the Redis `blacklist:` reads/writes, and the `refresh_cas_miss` / `session_legacy_upgraded` / `refresh_grace_lapsed` metrics.

- [ ] **Step 5: Simplify `Logout` (drop legacy blacklist fallback)**

Replace the `Logout` method body with:

```go
// Logout revokes the session that owns this refresh token. Unknown tokens are
// a no-op (the cookie is cleared by the handler regardless).
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	hash := hashRefreshToken(refreshToken)
	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err != nil {
		return nil // unknown/already-dead token: nothing to revoke
	}
	if rerr := s.sessionRepo.Revoke(ctx, session.ID, session.UserID); rerr != nil {
		return rerr
	}
	metrics.AuthEventsTotal.WithLabelValues("session_revoked", "logout").Inc()
	return nil
}
```

- [ ] **Step 6: Point `createSessionAndAuthResponse` at the sentinel**

In `createSessionAndAuthResponse`, change:

```go
		ExpiresAt:        now.Add(SessionTTL),
```

to:

```go
		ExpiresAt:        now.Add(SessionExpirySentinel),
```

- [ ] **Step 7: Build the service package — fix any now-unused imports**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: no output. If the compiler reports `"crypto/sha256"`/`cache` imported and not used: `hashRefreshToken` still uses sha256 (keep), and `cache` is still used by the telegram-auth methods (keep). If `generateRefreshToken` is now unused (only `createSessionAndAuthResponse` uses it — it still does), keep it. Only remove an import the compiler actually flags.

- [ ] **Step 8: Run the service tests — expect PASS**

Run: `cd /data/animeenigma && INTEGRATION=1 go test -tags=integration ./services/auth/internal/service/... -v`
Expected: PASS (`NonRotating`, `RevokedSession_Fails`).

- [ ] **Step 9: Commit**

```bash
git add services/auth/internal/service/auth.go services/auth/internal/service/auth_session_test.go
git commit -m "refactor(auth): non-rotating RefreshToken, sentinel expiry, drop legacy JWT path"
```

---

## Task 4: Handler — new signature, 10yr cookie, always re-set

**Files:**
- Modify: `services/auth/internal/handler/auth.go`

- [ ] **Step 1: Bump the refresh-cookie max age to ~10 years**

In `services/auth/internal/handler/auth.go`, replace:

```go
	// refreshTokenMaxAge MUST match service.SessionTTL — the cookie expiring
	// before the DB session leaves an orphaned row that the user can't reclaim.
	refreshTokenMaxAge    = 30 * 24 * time.Hour
```

with:

```go
	// refreshTokenMaxAge is effectively "never" for the browser cookie. The DB
	// session is non-expiring (revoke-only); we re-set this cookie on every
	// refresh so it keeps sliding ~10 years out and never ages out client-side.
	refreshTokenMaxAge    = 10 * 365 * 24 * time.Hour
```

- [ ] **Step 2: Update the `RefreshToken` handler to the single-return signature + always re-set cookie**

Replace the handler body from the `req := ...` line through the `setRefreshTokenCookie` conditional. Find:

```go
	sc := sessionContextFromReq(r)
	req := &domain.RefreshRequest{RefreshToken: cookie.Value}
	resp, rotated, err := h.authService.RefreshToken(r.Context(), req, sc)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("refresh_token", "error").Inc()
		// Clear invalid cookie
		h.clearRefreshTokenCookie(w)
		httputil.Error(w, err)
		return
	}

	// Only set a new refresh-token cookie when the token was actually rotated.
	// On the grace path (rotated=false), the existing cookie remains valid.
	if rotated {
		h.setRefreshTokenCookie(w, resp.RefreshToken)
	}
```

Replace with:

```go
	sc := sessionContextFromReq(r)
	req := &domain.RefreshRequest{RefreshToken: cookie.Value}
	resp, err := h.authService.RefreshToken(r.Context(), req, sc)
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("refresh_token", "error").Inc()
		// Clear invalid cookie
		h.clearRefreshTokenCookie(w)
		httputil.Error(w, err)
		return
	}

	// Non-rotating: the refresh token is unchanged. Re-set the same cookie value
	// so its 10-year max-age slides forward and the browser never drops it.
	h.setRefreshTokenCookie(w, cookie.Value)
```

- [ ] **Step 3: Build the whole auth service**

Run: `cd /data/animeenigma && go build ./services/auth/...`
Expected: no output.

- [ ] **Step 4: Run the full auth unit + integration suite**

Run: `cd /data/animeenigma && go test ./services/auth/... && INTEGRATION=1 go test -tags=integration ./services/auth/... `
Expected: PASS / ok for every package.

- [ ] **Step 5: Commit**

```bash
git add services/auth/internal/handler/auth.go
git commit -m "refactor(auth): refresh handler re-sets stable cookie, 10yr max-age"
```

---

## Task 5: Config — 1h access JWT

**Files:**
- Modify: `services/auth/internal/config/config.go`
- Modify: `libs/authz/jwt.go`
- Modify: `docker/docker-compose.yml`

- [ ] **Step 1: Raise the auth-service default**

In `services/auth/internal/config/config.go`, change:

```go
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
```

to:

```go
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", time.Hour),
```

- [ ] **Step 2: Raise the library default to match**

In `libs/authz/jwt.go`, in `DefaultJWTConfig` (around line 46), change:

```go
		AccessTokenTTL:  15 * time.Minute,
```

to:

```go
		AccessTokenTTL:  time.Hour,
```

- [ ] **Step 3: Document the env on the auth service in compose**

In `docker/docker-compose.yml`, under the `auth` service `environment:` block, add (near other JWT_* vars; if none, add after `JWT_SECRET`):

```yaml
      # Access-token lifetime. 1h: sessions are non-rotating + revoke-only, so a
      # short access token only bounds revoke latency (<=1h). See
      # docs/superpowers/specs/2026-06-05-simplify-auth-sessions-design.md
      JWT_ACCESS_TTL: ${JWT_ACCESS_TTL:-1h}
```

- [ ] **Step 4: Build libs/authz and verify compose parses**

Run: `cd /data/animeenigma && go build ./libs/authz/... && docker compose -f docker/docker-compose.yml config -q && echo OK`
Expected: `OK` (compose config validates; no Go build output).

- [ ] **Step 5: Commit**

```bash
git add services/auth/internal/config/config.go libs/authz/jwt.go docker/docker-compose.yml
git commit -m "feat(auth): default access JWT to 1h"
```

---

## Task 6: Frontend — remove cross-tab Web Locks

**Files:**
- Modify: `frontend/web/src/api/client.ts`

- [ ] **Step 1: Simplify `doTokenRefresh` (delete the Web Locks block)**

In `frontend/web/src/api/client.ts`, replace the inner async IIFE of `doTokenRefresh`. Find:

```ts
  isRefreshing = true
  refreshPromise = (async () => {
    try {
      if (typeof navigator !== 'undefined' && 'locks' in navigator) {
        return await navigator.locks.request('auth-refresh', async () => {
          // Another tab may have refreshed while we waited on the lock.
          // If localStorage now holds a still-valid token, use it instead
          // of burning our refresh-token round-trip.
          const stored = localStorage.getItem('token')
          if (stored && !isTokenExpired(stored)) {
            processQueue(null, stored)
            return stored
          }
          return await performRefresh()
        })
      }
      return await performRefresh()
    } finally {
      isRefreshing = false
      refreshPromise = null
    }
  })()

  return refreshPromise
```

Replace with:

```ts
  isRefreshing = true
  refreshPromise = (async () => {
    try {
      // Non-rotating refresh tokens: concurrent refreshes (other tabs, the
      // gateway admin middleware) all present the SAME stable token and all
      // succeed, so no cross-tab lock is needed. A still-valid token written by
      // another tab is reused to skip a redundant round-trip.
      const stored = localStorage.getItem('token')
      if (stored && !isTokenExpired(stored)) {
        processQueue(null, stored)
        return stored
      }
      return await performRefresh()
    } finally {
      isRefreshing = false
      refreshPromise = null
    }
  })()

  return refreshPromise
```

Also update the block comment above `doTokenRefresh` so it no longer claims a cross-tab lock. Find the comment starting `// Shared token refresh — single-flight both within a tab ...` and replace it with:

```ts
// Shared token refresh — single-flight within a tab via the module-level
// refreshPromise. Refresh tokens are non-rotating, so two tabs refreshing at
// once both succeed; no cross-tab coordination is required.
```

- [ ] **Step 2: Type-check and lint the frontend**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx eslint src/api/client.ts`
Expected: no errors.

- [ ] **Step 3: Run the frontend unit tests (sanity)**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/api 2>/dev/null || bunx vitest run --reporter=dot`
Expected: PASS (no client.ts-specific spec is expected; this confirms nothing else broke). If there are zero tests for `api/`, the broader run should still be green.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/api/client.ts
git commit -m "refactor(web): drop cross-tab Web Locks (non-rotating refresh is race-free)"
```

---

## Task 7: Deploy & verify end-to-end

**Files:** none (operational)

- [ ] **Step 1: Redeploy auth + web, check health**

Run: `cd /data/animeenigma && make redeploy-auth && make redeploy-web && make health`
Expected: both services rebuild and report healthy.

- [ ] **Step 2: Manual API smoke — refresh returns same cookie, fresh access**

Run (capture cookies from a login, then refresh twice):

```bash
cd /data/animeenigma
J=$(mktemp)
curl -s -c "$J" -X POST http://localhost:8000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"ui_audit_bot","password":"'"${UI_AUDIT_PASSWORD:-changeme}"'"}' >/dev/null
RT1=$(grep refresh_token "$J" | awk '{print $7}')
curl -s -b "$J" -c "$J" -X POST http://localhost:8000/api/auth/refresh >/dev/null
RT2=$(grep refresh_token "$J" | awk '{print $7}')
echo "RT unchanged across refresh: $([ "$RT1" = "$RT2" ] && echo YES || echo NO)"
```

Expected: `RT unchanged across refresh: YES`. (If the bot password isn't known, skip to Step 3's browser smoke instead.)

- [ ] **Step 3: Browser smoke (the real verification)**

Using the browser tools (or manually), with the dev site:
1. Log in. Confirm you land authenticated.
2. In DevTools console, force an expired access token to trigger a refresh:
   `localStorage.setItem('token', localStorage.getItem('token').replace(/.$/, 'x'))` then navigate — confirm you are NOT logged out (a transparent refresh occurs).
3. Open a second tab to the same site and hard-reload both near-simultaneously — confirm **both stay logged in** (the old rotation race would log one out).
4. Go to **Profile → Settings → Active Sessions**: confirm the list renders, the current device shows the "this device" badge, "revoke" on another row works, and "revoke others" works.

Expected: no logout in steps 2–3; sessions card fully functional in step 4.

- [ ] **Step 4: Confirm the random-logout metric is gone and refresh succeeds**

Run: `curl -s http://localhost:8081/metrics | grep -E 'refresh_grace_lapsed|refresh_cas_miss' ; echo "---"; curl -s http://localhost:8080/metrics | grep 'auth_events_total' | grep refresh_token`
Expected: no `refresh_grace_lapsed` / `refresh_cas_miss` lines (those code paths are deleted); `refresh_token{...success}` present.

- [ ] **Step 5: Run `/animeenigma-after-update`**

Per project policy, invoke the after-update skill to redeploy (idempotent), write the Russian Trump-mode changelog entry, commit, and push.

---

## Self-Review

**Spec coverage:**
- Non-rotating refresh → Task 3 (service) + Task 2 (repo `Touch`/`FindAliveByHash`). ✔
- Delete grace/CAS/previous-hash → Task 2. ✔
- `IsAlive = RevokedAt == nil` / no expiry → Task 1 + sentinel in Task 3. ✔
- No DB migration (dormant columns) → Task 1 keeps columns; Task 2/3 stop reading them. ✔
- Remove legacy JWT path + blacklist + `refresh_grace_lapsed` → Task 3. ✔
- 1h access JWT → Task 5. ✔
- 10yr cookie, always re-set → Task 4. ✔
- Frontend Web Locks removal → Task 6. ✔
- Revoke-from-settings unchanged + verified → Task 7 Step 3. ✔
- Deploy order auth→web, browser smoke → Task 7. ✔

**Placeholder scan:** No "TBD"/"handle errors"/"similar to" — every code step shows full code. ✔

**Type consistency:** `Touch(ctx, sessionID, ip string, lastSeen, expiresAt time.Time) error` defined in Task 2 and called identically in Task 3. `RefreshToken(ctx, req, sc) (*domain.AuthResponse, error)` defined in Task 3 and called with that arity in Task 4 and the rewritten test. `SessionExpirySentinel` defined once (Task 3) and reused in `createSessionAndAuthResponse`. `padHash`/`ptrTime` helpers defined in Task 2's test file. ✔

**Known assumptions:** integration tests need `make dev` up; the `ui_audit_bot` API smoke (Task 7 Step 2) is optional if the password isn't to hand — the browser smoke is the authoritative check.
