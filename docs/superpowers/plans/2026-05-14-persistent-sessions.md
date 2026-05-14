# Persistent Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 7-day stateless refresh-JWT with a sliding-30-day, DB-backed session model that lets users stay logged in indefinitely AND lets them list/revoke active devices from settings.

**Architecture:** Add a `user_sessions` table; opaque `rt_*` refresh tokens that hash into rows; CAS rotation with a 30-second grace window for cross-tab races; access JWT gains a `sid` claim. Legacy 7-day refresh-JWTs keep working via a fallback path that auto-upgrades to a session row on next refresh. New `/api/auth/sessions` REST endpoints back a new "Active Sessions" card in Profile → Settings.

**Tech Stack:** Go 1.22, GORM, chi, golang-jwt/jwt v5, Vue 3, Pinia, Tailwind, axios.

**Spec:** `docs/superpowers/specs/2026-05-14-persistent-sessions-design.md`

---

## File Structure

**Backend (auth service):**
- `services/auth/internal/domain/session.go` — new `UserSession` model (replaces empty stub in `user.go`)
- `services/auth/internal/repo/session.go` — new `SessionRepository`
- `services/auth/internal/service/auth.go` — refresh / login / logout / session ops rewritten
- `services/auth/internal/handler/auth.go` — handler tweaks for UA/IP capture, no-Set-Cookie on grace, list/revoke endpoints
- `services/auth/internal/handler/sessions.go` — new file: list/revoke handlers (keeps `auth.go` focused)
- `services/auth/internal/transport/router.go` — register new routes
- `services/auth/cmd/auth-api/main.go` — `AutoMigrate(&UserSession{})` + cleanup goroutine
- `libs/authz/jwt.go` — add `SessionID` (JWT claim `sid`) to `Claims`, accept it in `GenerateTokenPair`

**Frontend:**
- `frontend/web/src/api/sessions.ts` — new API client wrapper
- `frontend/web/src/composables/useSessions.ts` — load/revoke composable
- `frontend/web/src/utils/userAgent.ts` — tiny UA → "Chrome on Linux" parser
- `frontend/web/src/components/profile/ActiveSessionsCard.vue` — new card
- `frontend/web/src/views/Profile.vue` — mount the card in Settings tab
- `frontend/web/src/locales/{en,ru}.json` — strings

**Tests:**
- `services/auth/internal/repo/session_test.go` — repo CRUD + CAS
- `services/auth/internal/service/auth_session_test.go` — service-level flows
- `frontend/web/tests/e2e/sessions.spec.ts` — Playwright

---

## Task 1: Add `UserSession` domain model

**Files:**
- Create: `services/auth/internal/domain/session.go`
- Modify: `services/auth/internal/domain/user.go` (delete the unused `Session` stub at lines 26-34)

- [ ] **Step 1: Delete the unused `Session` stub from `user.go`**

Open `services/auth/internal/domain/user.go`. Find this block:

```go
// Session represents a user session
type Session struct {
	ID           string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID       string    `gorm:"type:uuid;index" json:"user_id"`
	RefreshToken string    `json:"-"`
	UserAgent    string    `json:"user_agent"`
	IP           string    `json:"ip"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}
```

Delete it. The new model below is incompatible (different fields, different table name).

- [ ] **Step 2: Create `session.go` with the new model**

```go
package domain

import "time"

// UserSession is one persistent login. Created on login/register, rotated on
// every /auth/refresh, revoked on logout or via the settings UI.
//
// `RefreshTokenHash` and `PreviousRefreshTokenHash` are sha256-hex of the
// opaque `rt_<64-hex>` refresh token. The previous hash is accepted during
// the grace window to absorb cross-tab refresh races.
type UserSession struct {
	ID                       string     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID                   string     `gorm:"type:uuid;not null;index:idx_user_sessions_user_id" json:"user_id"`
	RefreshTokenHash         string     `gorm:"type:char(64);not null;uniqueIndex:idx_user_sessions_rt_hash" json:"-"`
	PreviousRefreshTokenHash *string    `gorm:"type:char(64);index:idx_user_sessions_prev_rt_hash" json:"-"`
	GraceUntil               *time.Time `json:"-"`
	UserAgent                string     `gorm:"type:text;not null;default:''" json:"user_agent"`
	IP                       string     `gorm:"type:text;default:''" json:"ip"` // text not inet — keeps GORM portable; valid IPv4/IPv6 strings only
	CreatedAt                time.Time  `json:"created_at"`
	LastSeenAt               time.Time  `json:"last_seen_at"`
	ExpiresAt                time.Time  `gorm:"not null;index:idx_user_sessions_expires_at" json:"expires_at"`
	RevokedAt                *time.Time `json:"revoked_at,omitempty"`
}

func (UserSession) TableName() string { return "user_sessions" }

// IsAlive reports whether the session can be used for refresh.
func (s *UserSession) IsAlive(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}
```

- [ ] **Step 3: Build to verify the model compiles**

```bash
cd /data/animeenigma/services/auth && go build ./...
```

Expected: no errors. Compile failure → re-check imports / GORM tags.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add services/auth/internal/domain/session.go services/auth/internal/domain/user.go
git commit -m "$(cat <<'EOF'
feat(auth): add UserSession domain model for persistent sessions

Replaces the unused Session stub in user.go. New model carries the
fields needed for the sliding-30d session: rt hash, previous-rt hash
+ grace window for cross-tab race absorption, UA, IP, last_seen.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 2: Add `sid` claim to access JWTs

**Files:**
- Modify: `libs/authz/jwt.go`

- [ ] **Step 1: Add `SessionID` to the `Claims` struct**

In `libs/authz/jwt.go`, find the `Claims` struct (near the top) and add a `SessionID` field. The struct should look like this after edit:

```go
// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	UserID    string `json:"uid"`
	Username  string `json:"username"`
	Role      Role   `json:"role"`
	SessionID string `json:"sid,omitempty"` // empty for legacy tokens minted before persistent sessions
}
```

- [ ] **Step 2: Update `GenerateTokenPair` to accept and embed a session ID**

Find the existing signature:

```go
func (m *JWTManager) GenerateTokenPair(userID, username string, role Role) (*TokenPair, error) {
```

Change it to:

```go
func (m *JWTManager) GenerateTokenPair(userID, username string, role Role, sessionID string) (*TokenPair, error) {
```

Inside the function, find the `accessClaims` literal and add `SessionID: sessionID,` to it. The block becomes:

```go
accessClaims := Claims{
    RegisteredClaims: jwt.RegisteredClaims{
        Issuer:    m.config.Issuer,
        Subject:   userID,
        IssuedAt:  jwt.NewNumericDate(now),
        ExpiresAt: jwt.NewNumericDate(now.Add(m.config.AccessTokenTTL)),
    },
    UserID:    userID,
    Username:  username,
    Role:      role,
    SessionID: sessionID,
}
```

- [ ] **Step 3: Build the workspace to find every caller**

```bash
cd /data/animeenigma && go build ./...
```

Expected output: compile errors at every call site of `GenerateTokenPair` (different arg count). The next step fixes them.

- [ ] **Step 4: Update every caller**

Search for all callers:

```bash
grep -rn "GenerateTokenPair" /data/animeenigma --include="*.go"
```

There should be one production caller in `services/auth/internal/service/auth.go` (`generateAuthResponse`). Update it temporarily to pass `""` for `sessionID` — Task 4 will rewrite this method to pass a real session ID. The line becomes:

```go
tokenPair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, "")
```

If grep finds any test files passing 3 args, add `, ""` to those too.

- [ ] **Step 5: Build again to confirm green**

```bash
cd /data/animeenigma && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add libs/authz/jwt.go services/auth/internal/service/auth.go
git commit -m "$(cat <<'EOF'
feat(authz): add sid (session id) claim to access JWTs

Required for per-request session lookups (revocation, future Redis
allowlist). Existing call sites pass "" for now; the auth service
will fill it in once persistent sessions land.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 3: Build `SessionRepository`

**Files:**
- Create: `services/auth/internal/repo/session.go`
- Test: `services/auth/internal/repo/session_test.go`

This repo encapsulates all DB access for sessions. CAS rotation lives here.

- [ ] **Step 1: Write the repo**

Create `services/auth/internal/repo/session.go`:

```go
package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

// GraceWindow is how long the previous refresh-token hash stays accepted
// after a successful rotation. Long enough to absorb cross-tab races,
// short enough that a stolen previous-token can't be reused indefinitely.
const GraceWindow = 30 * time.Second

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create inserts a new session.
func (r *SessionRepository) Create(ctx context.Context, s *domain.UserSession) error {
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// FindAliveByHash returns the alive session whose current OR previous
// (within grace) hash equals `hash`. Returns NotFound if none.
func (r *SessionRepository) FindAliveByHash(ctx context.Context, hash string) (*domain.UserSession, error) {
	now := time.Now()
	var s domain.UserSession
	err := r.db.WithContext(ctx).
		Where("revoked_at IS NULL AND expires_at > ?", now).
		Where("refresh_token_hash = ? OR (previous_refresh_token_hash = ? AND grace_until > ?)", hash, hash, now).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("session")
		}
		return nil, fmt.Errorf("find session by hash: %w", err)
	}
	return &s, nil
}

// RotateResult tells the caller which path the rotation took so it knows
// whether to set a new refresh-token cookie.
type RotateResult struct {
	Session   *domain.UserSession
	Rotated   bool // true: caller should Set-Cookie with new RT. false: grace-path, leave cookie alone.
}

// Rotate performs the CAS rotation. If `oldHash` matches `refresh_token_hash`,
// it swaps in `newHash` (also moves the old hash into previous + sets grace).
// If `oldHash` matches `previous_refresh_token_hash` (within grace), it does
// NOT rotate — caller mints a fresh access token and reuses the existing RT.
//
// `extendUntil` is the new expires_at the rotating writer should set.
// `ip` is recorded for audit.
func (r *SessionRepository) Rotate(
	ctx context.Context,
	sessionID, oldHash, newHash, ip string,
	extendUntil time.Time,
) (RotateResult, error) {
	now := time.Now()
	graceUntil := now.Add(GraceWindow)

	// Try CAS swap first.
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND refresh_token_hash = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, oldHash, now).
		Updates(map[string]any{
			"previous_refresh_token_hash": oldHash,
			"refresh_token_hash":          newHash,
			"grace_until":                 graceUntil,
			"last_seen_at":                now,
			"expires_at":                  extendUntil,
			"ip":                          ip,
		})
	if res.Error != nil {
		return RotateResult{}, fmt.Errorf("rotate session: %w", res.Error)
	}

	if res.RowsAffected == 1 {
		// Re-read to return current state.
		var s domain.UserSession
		if err := r.db.WithContext(ctx).First(&s, "id = ?", sessionID).Error; err != nil {
			return RotateResult{}, fmt.Errorf("re-read rotated session: %w", err)
		}
		return RotateResult{Session: &s, Rotated: true}, nil
	}

	// CAS missed — concurrent rotation already happened. Confirm we're on
	// the grace path: the row's previous_hash should equal oldHash within
	// grace_until. Bump last_seen but DO NOT mint a new RT.
	var s domain.UserSession
	err := r.db.WithContext(ctx).
		Where("id = ? AND previous_refresh_token_hash = ? AND grace_until > ? AND revoked_at IS NULL AND expires_at > ?",
			sessionID, oldHash, now, now).
		First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RotateResult{}, liberrors.NotFound("session")
		}
		return RotateResult{}, fmt.Errorf("read grace session: %w", err)
	}

	// Best-effort touch of last_seen + ip; ignore conflicts.
	_ = r.db.WithContext(ctx).
		Model(&s).
		Where("id = ?", s.ID).
		Updates(map[string]any{"last_seen_at": now, "ip": ip}).Error

	return RotateResult{Session: &s, Rotated: false}, nil
}

// Revoke marks one session revoked iff it belongs to userID and is alive.
// Returns ErrRecordNotFound-equivalent if the row is missing or not the user's.
func (r *SessionRepository) Revoke(ctx context.Context, sessionID, userID string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
		Update("revoked_at", now)
	if res.Error != nil {
		return fmt.Errorf("revoke session: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return liberrors.NotFound("session")
	}
	return nil
}

// RevokeOthers revokes every alive session for `userID` except `keepID`.
// Returns the count revoked.
func (r *SessionRepository) RevokeOthers(ctx context.Context, userID, keepID string) (int64, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&domain.UserSession{}).
		Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, keepID).
		Update("revoked_at", now)
	if res.Error != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ListAlive returns all alive sessions for a user, newest last_seen first.
func (r *SessionRepository) ListAlive(ctx context.Context, userID string) ([]*domain.UserSession, error) {
	now := time.Now()
	var sessions []*domain.UserSession
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).
		Order("last_seen_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("list alive sessions: %w", err)
	}
	return sessions, nil
}

// Cleanup removes sessions revoked >7d ago or expired >7d ago.
// Returns the count deleted.
func (r *SessionRepository) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	res := r.db.WithContext(ctx).
		Where("(revoked_at IS NOT NULL AND revoked_at < ?) OR expires_at < ?", cutoff, cutoff).
		Delete(&domain.UserSession{})
	if res.Error != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", res.Error)
	}
	return res.RowsAffected, nil
}
```

- [ ] **Step 2: Build to verify**

```bash
cd /data/animeenigma && go build ./services/auth/...
```

Expected: green.

- [ ] **Step 3: Write the repo test**

This needs a real Postgres. The auth service uses GORM directly; existing repo tests in this codebase generally rely on testcontainers or a docker-compose-up-postgres pattern. If you don't see existing testcontainer setup in `services/auth/internal/repo/` (run `ls services/auth/internal/repo/*_test.go`), use a `t.Skip` guard with `if os.Getenv("INTEGRATION") != "1"` so CI without DB doesn't fail, then run it locally with `INTEGRATION=1 go test`.

Create `services/auth/internal/repo/session_test.go`:

```go
//go:build integration

package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// dbForTest connects to the dev postgres. Run via `make dev` first, then:
//   INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/...
func dbForTest(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost port=5432 user=postgres password=postgres dbname=animeenigma sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.UserSession{}))
	return db
}

// seedUser inserts a minimal users row and returns its ID. user_sessions has
// an FK on users(id), so we need a real user.
func seedUser(t *testing.T, db *gorm.DB) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, username, password_hash, public_id, role, public_statuses) VALUES (?, ?, '', ?, 'user', '{}')",
		id, "test_"+id[:8], "pub_"+id[:8],
	).Error)
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", id) })
	return id
}

func TestSessionRepo_CreateAndFindByHash(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: "hash_" + uuid.NewString()[:32] + "padding____________________",
		UserAgent:        "go-test",
		ExpiresAt:        time.Now().Add(time.Hour),
		LastSeenAt:       time.Now(),
	}
	require.NoError(t, r.Create(context.Background(), s))
	require.NotEmpty(t, s.ID)

	found, err := r.FindAliveByHash(context.Background(), s.RefreshTokenHash)
	require.NoError(t, err)
	require.Equal(t, s.ID, found.ID)
}

func TestSessionRepo_RotateCASWinAndGracePath(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	oldHash := "old_" + uuid.NewString()[:32] + "padding_____________________"
	newHash := "new_" + uuid.NewString()[:32] + "padding_____________________"
	s := &domain.UserSession{
		UserID:           userID,
		RefreshTokenHash: oldHash,
		ExpiresAt:        time.Now().Add(time.Hour),
		LastSeenAt:       time.Now(),
	}
	require.NoError(t, r.Create(context.Background(), s))

	// Winner CAS — rotates.
	res1, err := r.Rotate(context.Background(), s.ID, oldHash, newHash, "1.2.3.4", time.Now().Add(30*24*time.Hour))
	require.NoError(t, err)
	require.True(t, res1.Rotated)
	require.Equal(t, newHash, res1.Session.RefreshTokenHash)

	// Loser CAS with the same oldHash — should hit the grace path, not rotate.
	res2, err := r.Rotate(context.Background(), s.ID, oldHash, "loser_"+uuid.NewString()[:32]+"padding____________________", "5.6.7.8", time.Now().Add(30*24*time.Hour))
	require.NoError(t, err)
	require.False(t, res2.Rotated, "expected grace path (no rotation) on second CAS with same oldHash")
	require.Equal(t, newHash, res2.Session.RefreshTokenHash, "row hash should still be the winner's")
}

func TestSessionRepo_RevokeAndRevokeOthers(t *testing.T) {
	db := dbForTest(t)
	r := repo.NewSessionRepository(db)
	userID := seedUser(t, db)

	mk := func(tag string) *domain.UserSession {
		s := &domain.UserSession{
			UserID:           userID,
			RefreshTokenHash: tag + "_" + uuid.NewString()[:32] + "padding____________________",
			ExpiresAt:        time.Now().Add(time.Hour),
			LastSeenAt:       time.Now(),
		}
		require.NoError(t, r.Create(context.Background(), s))
		return s
	}

	a, b, c := mk("a"), mk("b"), mk("c")

	// Revoke single
	require.NoError(t, r.Revoke(context.Background(), a.ID, userID))
	alive, err := r.ListAlive(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, alive, 2)

	// Revoke single by wrong user → NotFound
	otherUser := seedUser(t, db)
	err = r.Revoke(context.Background(), b.ID, otherUser)
	require.Error(t, err)

	// Revoke others (keep c)
	count, err := r.RevokeOthers(context.Background(), userID, c.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count) // b only; a was already revoked

	alive, err = r.ListAlive(context.Background(), userID)
	require.NoError(t, err)
	require.Len(t, alive, 1)
	require.Equal(t, c.ID, alive[0].ID)
}
```

- [ ] **Step 4: Run the integration test**

```bash
make dev    # if not already running
cd /data/animeenigma && INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -run TestSessionRepo -v
```

Expected: 3 PASS. If GORM auto-migration fails on `INET` (only matters if you switched the IP column type), the model uses `text` so this should not happen.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/auth/internal/repo/session.go services/auth/internal/repo/session_test.go
git commit -m "$(cat <<'EOF'
feat(auth): SessionRepository with CAS rotation + grace window

Encapsulates user_sessions DB access:
- Create/FindAliveByHash for login + refresh
- Rotate: compare-and-swap on refresh_token_hash, falls back to a
  30s grace path when concurrent rotations land — the loser sees the
  winner's row instead of a 401
- Revoke / RevokeOthers / ListAlive back the settings UI
- Cleanup goroutine removes sessions revoked or expired >7d ago

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 4: Rewrite AuthService — login, refresh, logout, session ops

**Files:**
- Modify: `services/auth/internal/service/auth.go`

The service gets bigger. Several changes are coupled and must land together to keep the build green.

- [ ] **Step 1: Update the constructor and struct to accept SessionRepository**

In `services/auth/internal/service/auth.go`:

Change the struct:

```go
type AuthService struct {
	userRepo         *repo.UserRepository
	sessionRepo      *repo.SessionRepository
	cache            *cache.RedisCache
	jwtManager       *authz.JWTManager
	telegramBotToken string
	log              *logger.Logger
}
```

Change `NewAuthService`:

```go
func NewAuthService(
	userRepo *repo.UserRepository,
	sessionRepo *repo.SessionRepository,
	cache *cache.RedisCache,
	jwtConfig authz.JWTConfig,
	telegramBotToken string,
	log *logger.Logger,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		sessionRepo:      sessionRepo,
		cache:            cache,
		jwtManager:       authz.NewJWTManager(jwtConfig),
		telegramBotToken: telegramBotToken,
		log:              log,
	}
}
```

- [ ] **Step 2: Add helpers for opaque RT generation**

Add at the bottom of `auth.go`:

```go
const refreshTokenPrefix = "rt_"

// generateRefreshToken returns a fresh opaque refresh token like "rt_<64-hex>".
func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return refreshTokenPrefix + hex.EncodeToString(b), nil
}

// hashRefreshToken returns the sha256-hex of a refresh token. Used as the
// row's refresh_token_hash. Never store the raw token.
func hashRefreshToken(rt string) string {
	sum := sha256.Sum256([]byte(rt))
	return hex.EncodeToString(sum[:])
}
```

(`crypto/rand`, `crypto/sha256`, `encoding/hex` are already imported via the API key code.)

- [ ] **Step 3: Replace `generateAuthResponse` with a session-aware version**

Find the existing function:

```go
func (s *AuthService) generateAuthResponse(user *domain.User) (*domain.AuthResponse, error) {
```

Replace its entire body with this. The new version takes a `SessionContext` so callers can pass UA/IP from the HTTP layer:

```go
// SessionContext carries per-request context the service needs to create a
// session row. Login/Register/Telegram-confirm all populate this from
// the HTTP layer.
type SessionContext struct {
	UserAgent string
	IP        string
}

// SessionTTL is the sliding-window length. Every refresh extends a session
// to now+SessionTTL. 30 days = "user opens the site at least once a month".
const SessionTTL = 30 * 24 * time.Hour

// createSessionAndAuthResponse mints a fresh session row + tokens.
// Used by Login, Register, telegram-confirm — anywhere that's NOT a refresh.
func (s *AuthService) createSessionAndAuthResponse(
	ctx context.Context,
	user *domain.User,
	sc SessionContext,
) (*domain.AuthResponse, error) {
	rt, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	session := &domain.UserSession{
		UserID:           user.ID,
		RefreshTokenHash: hashRefreshToken(rt),
		UserAgent:        truncateUA(sc.UserAgent),
		IP:               sc.IP,
		LastSeenAt:       now,
		ExpiresAt:        now.Add(SessionTTL),
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, session.ID)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	s.log.Infow("session created", "user_id", user.ID, "session_id", session.ID)
	metrics.AuthEventsTotal.WithLabelValues("session_created", "success").Inc()

	return &domain.AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: rt,
		ExpiresAt:    pair.ExpiresAt,
		User:         user,
	}, nil
}

// truncateUA caps user-agent length at 1024 to avoid pathological headers
// blowing up DB rows.
func truncateUA(ua string) string {
	const maxUA = 1024
	if len(ua) > maxUA {
		return ua[:maxUA]
	}
	return ua
}
```

(You'll need to add `"github.com/ILITA-hub/animeenigma/libs/metrics"` to the import block — pattern matches the handler file.)

- [ ] **Step 4: Update Login / Register / LoginWithTelegram signatures to take SessionContext**

For each of `Login`, `Register`, `LoginWithTelegram`:
- Add `, sc SessionContext` as the last parameter.
- Replace the trailing `return s.generateAuthResponse(user)` with `return s.createSessionAndAuthResponse(ctx, user, sc)`.

For example, `Login` becomes:

```go
func (s *AuthService) Login(ctx context.Context, req *domain.LoginRequest, sc SessionContext) (*domain.AuthResponse, error) {
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok && appErr.Code == errors.CodeNotFound {
			return nil, errors.Unauthorized("invalid credentials")
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.Unauthorized("invalid credentials")
	}
	return s.createSessionAndAuthResponse(ctx, user, sc)
}
```

Apply the same pattern to `Register` and `LoginWithTelegram`. **Also update `CheckDeepLinkToken`** — it calls `LoginWithTelegram`. Add a `SessionContext` parameter to `CheckDeepLinkToken`'s signature and forward it. (Telegram check polls don't always have a useful UA — passing whatever the polling browser sent is fine.)

After this, the old `generateAuthResponse` is dead. Delete it.

- [ ] **Step 5: Rewrite `RefreshToken`**

Replace the existing `RefreshToken` method entirely with the dual-path version:

```go
func (s *AuthService) RefreshToken(
	ctx context.Context,
	req *domain.RefreshRequest,
	sc SessionContext,
) (*domain.AuthResponse, bool, error) {
	rt := req.RefreshToken
	hash := hashRefreshToken(rt)

	// 1) Try persistent-session path.
	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err == nil {
		newRT, err := generateRefreshToken()
		if err != nil {
			return nil, false, err
		}
		newHash := hashRefreshToken(newRT)

		result, err := s.sessionRepo.Rotate(ctx, session.ID, hash, newHash, sc.IP, time.Now().Add(SessionTTL))
		if err != nil {
			return nil, false, err
		}

		// Re-load user (cheap; could cache later)
		user, err := s.userRepo.GetByID(ctx, result.Session.UserID)
		if err != nil {
			return nil, false, err
		}

		pair, err := s.jwtManager.GenerateTokenPair(user.ID, user.Username, user.Role, result.Session.ID)
		if err != nil {
			return nil, false, fmt.Errorf("generate tokens: %w", err)
		}

		resp := &domain.AuthResponse{
			AccessToken: pair.AccessToken,
			ExpiresAt:   pair.ExpiresAt,
			User:        user,
		}
		if result.Rotated {
			resp.RefreshToken = newRT
			metrics.AuthEventsTotal.WithLabelValues("refresh_token", "success").Inc()
		} else {
			// Grace path — caller must NOT issue a Set-Cookie for refresh.
			metrics.AuthEventsTotal.WithLabelValues("refresh_cas_miss", "grace_hit").Inc()
		}
		return resp, result.Rotated, nil
	}

	// 2) Legacy JWT path (transition window).
	userID, jwtErr := s.jwtManager.ValidateRefreshToken(rt)
	if jwtErr == nil {
		// Check legacy blacklist
		blacklistKey := cache.PrefixSession + "blacklist:" + rt
		if exists, _ := s.cache.Exists(ctx, blacklistKey); exists {
			return nil, false, errors.Unauthorized("token has been revoked")
		}
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return nil, false, err
		}
		// Blacklist the legacy RT so it can't be reused.
		_ = s.cache.Set(ctx, blacklistKey, true, 7*24*time.Hour)

		// Upgrade — mint a real session.
		resp, err := s.createSessionAndAuthResponse(ctx, user, sc)
		if err != nil {
			return nil, false, err
		}
		metrics.AuthEventsTotal.WithLabelValues("session_legacy_upgraded", "success").Inc()
		s.log.Infow("upgraded legacy refresh JWT to persistent session", "user_id", user.ID)
		return resp, true, nil
	}

	// 3) Both paths failed.
	return nil, false, errors.Unauthorized("invalid refresh token")
}
```

The new `bool` return value is `rotated` — handler uses it to decide whether to set the refresh cookie.

- [ ] **Step 6: Rewrite `Logout`**

Replace the existing `Logout`:

```go
// Logout revokes the session that owns this refresh token. If the cookie
// is a legacy JWT, fall back to the old blacklist behavior.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	hash := hashRefreshToken(refreshToken)
	session, err := s.sessionRepo.FindAliveByHash(ctx, hash)
	if err == nil {
		if rerr := s.sessionRepo.Revoke(ctx, session.ID, session.UserID); rerr != nil {
			return rerr
		}
		metrics.AuthEventsTotal.WithLabelValues("session_revoked", "logout").Inc()
		return nil
	}
	// Legacy: blacklist the JWT.
	blacklistKey := cache.PrefixSession + "blacklist:" + refreshToken
	return s.cache.Set(ctx, blacklistKey, true, 7*24*time.Hour)
}
```

- [ ] **Step 7: Add session listing / revocation methods**

Append to `auth.go`:

```go
// SessionListItem is the public-facing shape returned to /api/auth/sessions.
type SessionListItem struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsCurrent  bool      `json:"is_current"`
}

func (s *AuthService) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionListItem, error) {
	rows, err := s.sessionRepo.ListAlive(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SessionListItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionListItem{
			ID:         r.ID,
			UserAgent:  r.UserAgent,
			IP:         r.IP,
			CreatedAt:  r.CreatedAt,
			LastSeenAt: r.LastSeenAt,
			ExpiresAt:  r.ExpiresAt,
			IsCurrent:  r.ID == currentSessionID,
		})
	}
	return out, nil
}

func (s *AuthService) RevokeSession(ctx context.Context, userID, sessionID string) error {
	if err := s.sessionRepo.Revoke(ctx, sessionID, userID); err != nil {
		return err
	}
	metrics.AuthEventsTotal.WithLabelValues("session_revoked", "user_action").Inc()
	return nil
}

func (s *AuthService) RevokeOtherSessions(ctx context.Context, userID, currentSessionID string) (int64, error) {
	n, err := s.sessionRepo.RevokeOthers(ctx, userID, currentSessionID)
	if err != nil {
		return 0, err
	}
	metrics.AuthEventsTotal.WithLabelValues("session_revoked", "revoke_others").Add(float64(n))
	return n, nil
}

// CleanupExpiredSessions is called from a goroutine in main.go.
func (s *AuthService) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	n, err := s.sessionRepo.Cleanup(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		metrics.AuthEventsTotal.WithLabelValues("session_expired", "cleanup").Add(float64(n))
	}
	return n, nil
}
```

- [ ] **Step 8: Add the metric label values**

Open `libs/metrics/metrics.go` (or wherever `AuthEventsTotal` is defined). Confirm the label set is `("event", "outcome")` — if so, no schema change needed; the labels we use (`session_created`, `session_revoked`, etc.) are values, not new dimensions. If the metric doesn't exist or has a different shape, fix the label calls in this task to match the existing pattern.

```bash
grep -n "AuthEventsTotal" /data/animeenigma/libs/metrics/*.go
```

- [ ] **Step 9: Build**

```bash
cd /data/animeenigma && go build ./services/auth/...
```

Expected: green. Compile errors here probably mean a Login/Register caller in tests or telegram-bot code wasn't updated — fix them inline (`SessionContext{}` is fine for tests).

- [ ] **Step 10: Commit**

```bash
cd /data/animeenigma
git add services/auth/internal/service/auth.go
git commit -m "$(cat <<'EOF'
feat(auth): rewrite AuthService for persistent sessions

- createSessionAndAuthResponse mints a session row + opaque RT on every
  Login / Register / Telegram-confirm
- RefreshToken uses DB lookup first, falls back to legacy JWT path
  (which auto-upgrades to a session on success)
- Grace path on CAS miss: returns rotated=false so handler skips
  Set-Cookie, eliminating cross-tab false-401s
- Logout revokes the session row (or legacy-blacklists the RT)
- ListSessions / RevokeSession / RevokeOtherSessions back the
  forthcoming settings UI

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 5: Update AuthHandler — pass UA/IP, honor grace path

**Files:**
- Modify: `services/auth/internal/handler/auth.go`

- [ ] **Step 1: Add a small helper to extract `SessionContext` from a request**

At the top of `auth.go` (near the cookie helpers), add:

```go
func sessionContextFromReq(r *http.Request) service.SessionContext {
	return service.SessionContext{
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	}
}

// clientIP returns the best-effort client IP. The router already runs
// chi's middleware.RealIP, which rewrites RemoteAddr based on
// X-Real-IP / X-Forwarded-For when present.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}
```

(Add `"strings"` to the import block.)

- [ ] **Step 2: Update Login, Register, CheckDeepLink to pass `SessionContext`**

In `Register`:
```go
resp, err := h.authService.Register(r.Context(), &req, sessionContextFromReq(r))
```

In `Login`:
```go
resp, err := h.authService.Login(r.Context(), &req, sessionContextFromReq(r))
```

In `CheckDeepLink`, the underlying call is to `CheckDeepLinkToken`. Update both:

```go
checkResp, authResp, err := h.authService.CheckDeepLinkToken(r.Context(), token, sessionContextFromReq(r))
```

(And forward `sc` from `CheckDeepLinkToken` into its `LoginWithTelegram` call inside the service.)

- [ ] **Step 3: Update `RefreshToken` handler to honor the grace path**

Replace the existing `RefreshToken` handler:

```go
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		httputil.Error(w, errors.Unauthorized("refresh token not found"))
		return
	}

	req := &domain.RefreshRequest{RefreshToken: cookie.Value}
	resp, rotated, err := h.authService.RefreshToken(r.Context(), req, sessionContextFromReq(r))
	if err != nil {
		metrics.AuthEventsTotal.WithLabelValues("refresh_token", "error").Inc()
		h.clearRefreshTokenCookie(w)
		h.clearAccessTokenCookie(w)
		httputil.Error(w, err)
		return
	}

	// On grace path, do NOT issue a new refresh-token Set-Cookie — the
	// other tab that won the rotation has the live RT, and our cross-tab
	// `storage` listener will sync it. Rewriting it here with our (now
	// stale) raw RT would clobber the winner's cookie.
	if rotated {
		h.setRefreshTokenCookie(w, resp.RefreshToken)
	}
	h.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)

	httputil.OK(w, resp.ToPublicResponse())
}
```

- [ ] **Step 4: Build**

```bash
cd /data/animeenigma && go build ./services/auth/...
```

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/auth/internal/handler/auth.go services/auth/internal/service/auth.go
git commit -m "$(cat <<'EOF'
feat(auth): handler captures UA/IP and honors grace path

Refresh handler no longer issues a Set-Cookie when the service reports
rotated=false (grace path) — the winning tab's RT stays authoritative.
Login / Register / Telegram-confirm now pass SessionContext so the
new session row records what device created it.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 6: Add session list/revoke HTTP handlers + routes

**Files:**
- Create: `services/auth/internal/handler/sessions.go`
- Modify: `services/auth/internal/transport/router.go`

- [ ] **Step 1: Create the handler file**

```go
package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// SessionsHandler exposes /api/auth/sessions endpoints.
type SessionsHandler struct {
	authService *service.AuthService
	log         *logger.Logger
}

func NewSessionsHandler(authService *service.AuthService, log *logger.Logger) *SessionsHandler {
	return &SessionsHandler{authService: authService, log: log}
}

func (h *SessionsHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	items, err := h.authService.ListSessions(r.Context(), claims.UserID, claims.SessionID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, items)
}

func (h *SessionsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.authService.RevokeSession(r.Context(), claims.UserID, id); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

func (h *SessionsHandler) RevokeOthers(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok {
		httputil.Unauthorized(w)
		return
	}
	n, err := h.authService.RevokeOtherSessions(r.Context(), claims.UserID, claims.SessionID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int64{"revoked": n})
}
```

- [ ] **Step 2: Wire routes in `transport/router.go`**

Add `sessionsHandler *handler.SessionsHandler` to `NewRouter`'s parameters. Inside the protected `r.Group(...)` block (where the API Key routes are), add:

```go
r.Get("/auth/sessions", sessionsHandler.List)
r.Delete("/auth/sessions/{id}", sessionsHandler.Revoke)
r.Post("/auth/sessions/revoke-others", sessionsHandler.RevokeOthers)
```

- [ ] **Step 3: Build**

```bash
cd /data/animeenigma && go build ./services/auth/...
```

(Will fail because `main.go` doesn't yet construct `SessionsHandler` — Task 7 fixes it.)

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add services/auth/internal/handler/sessions.go services/auth/internal/transport/router.go
git commit -m "$(cat <<'EOF'
feat(auth): add /api/auth/sessions endpoints

GET    /api/auth/sessions               — list alive sessions
DELETE /api/auth/sessions/{id}          — revoke one
POST   /api/auth/sessions/revoke-others — revoke all except current

All protected by AuthMiddleware. Current session is identified by the
sid claim in the access JWT.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 7: Wire it all up in `main.go` + cleanup goroutine

**Files:**
- Modify: `services/auth/cmd/auth-api/main.go`

- [ ] **Step 1: Add session model to AutoMigrate**

Find:
```go
if err := db.AutoMigrate(&domain.User{}); err != nil {
```
Change to:
```go
if err := db.AutoMigrate(&domain.User{}, &domain.UserSession{}); err != nil {
```

- [ ] **Step 2: Construct the new repo + handler, pass to wiring**

After `userRepo := repo.NewUserRepository(db.DB)` add:
```go
sessionRepo := repo.NewSessionRepository(db.DB)
```

Update the AuthService construction:
```go
authService := service.NewAuthService(userRepo, sessionRepo, redisCache, cfg.JWT, cfg.Telegram.BotToken, log)
```

After `userHandler := ...` add:
```go
sessionsHandler := handler.NewSessionsHandler(authService, log)
```

Update the router call:
```go
router := transport.NewRouter(authHandler, telegramBotHandler, userHandler, sessionsHandler, cfg.JWT, log, metricsCollector)
```

- [ ] **Step 3: Add the cleanup goroutine**

After `// Start server` block, before the `// Graceful shutdown` block, add:

```go
// Periodic session cleanup — drops rows revoked or expired >7 days ago.
go func() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-quit:
			return
		case <-t.C:
			n, err := authService.CleanupExpiredSessions(context.Background())
			if err != nil {
				log.Warnw("session cleanup failed", "error", err)
				continue
			}
			if n > 0 {
				log.Infow("session cleanup", "deleted", n)
			}
		}
	}
}()
```

Note: the goroutine reads `quit`. Move the `quit := make(chan os.Signal, 1)` and `signal.Notify(...)` lines BEFORE the goroutine if they're currently after it. Otherwise, replace the cleanup goroutine's `<-quit` with a fresh ticker-only loop and rely on `srv.Shutdown` to close it indirectly (the goroutine is harmless on shutdown — it just sits in the ticker).

- [ ] **Step 4: Build and run**

```bash
cd /data/animeenigma && go build ./services/auth/...
make redeploy-auth
make logs-auth | head -30
```

Expected log line: `"GORM auto-migrate"` or no errors mentioning `user_sessions`. The table should now exist:

```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d user_sessions"
```

Expected: schema printout matching the spec (8 columns, 4 indexes).

- [ ] **Step 5: Smoke-test refresh end-to-end**

```bash
# Login as ui_audit_bot with password
COOKIE_JAR=$(mktemp)
curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" \
  -H "Content-Type: application/json" \
  -d '{"username":"ui_audit_bot","password":"audit_bot_test_password_2026"}' \
  http://localhost:8000/api/auth/login | jq .

# Look for an rt_-prefixed cookie
grep refresh_token "$COOKIE_JAR"

# Then refresh
curl -s -c "$COOKIE_JAR" -b "$COOKIE_JAR" -X POST http://localhost:8000/api/auth/refresh | jq .

# Check that DB now has a row
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "SELECT id, user_agent, ip, created_at, last_seen_at, expires_at FROM user_sessions ORDER BY created_at DESC LIMIT 5;"
```

Expected: cookie value starts with `rt_`, refresh returns 200 with a fresh access_token, DB row exists with `user_agent` populated.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add services/auth/cmd/auth-api/main.go
git commit -m "$(cat <<'EOF'
feat(auth): wire SessionRepository + cleanup goroutine in main.go

AutoMigrate creates user_sessions on startup. Hourly goroutine drops
rows revoked or expired >7d ago.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 8: Frontend — API client + composable + UA parser

**Files:**
- Create: `frontend/web/src/api/sessions.ts`
- Create: `frontend/web/src/composables/useSessions.ts`
- Create: `frontend/web/src/utils/userAgent.ts`

- [ ] **Step 1: Create the API client wrapper**

`frontend/web/src/api/sessions.ts`:

```ts
import { apiClient } from './client'

export interface ApiSession {
  id: string
  user_agent: string
  ip: string
  created_at: string
  last_seen_at: string
  expires_at: string
  is_current: boolean
}

export const sessionsApi = {
  async list(): Promise<ApiSession[]> {
    const res = await apiClient.get('/auth/sessions')
    return res.data?.data ?? res.data ?? []
  },

  async revoke(id: string): Promise<void> {
    await apiClient.delete(`/auth/sessions/${encodeURIComponent(id)}`)
  },

  async revokeOthers(): Promise<number> {
    const res = await apiClient.post('/auth/sessions/revoke-others')
    return Number((res.data?.data ?? res.data)?.revoked ?? 0)
  },
}
```

- [ ] **Step 2: Create the composable**

`frontend/web/src/composables/useSessions.ts`:

```ts
import { ref } from 'vue'
import { sessionsApi, type ApiSession } from '@/api/sessions'

export function useSessions() {
  const sessions = ref<ApiSession[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      sessions.value = await sessionsApi.list()
    } catch (e: unknown) {
      error.value = (e as Error)?.message ?? 'load_failed'
    } finally {
      loading.value = false
    }
  }

  async function revoke(id: string) {
    await sessionsApi.revoke(id)
    sessions.value = sessions.value.filter(s => s.id !== id)
  }

  async function revokeOthers() {
    const n = await sessionsApi.revokeOthers()
    sessions.value = sessions.value.filter(s => s.is_current)
    return n
  }

  return { sessions, loading, error, refresh, revoke, revokeOthers }
}
```

- [ ] **Step 3: Create the UA parser**

`frontend/web/src/utils/userAgent.ts`:

```ts
// Tiny UA → "Browser on OS" parser. We don't need precision; just enough
// for the user to recognize "this is my phone vs my work laptop". For
// anything pathological, we fall back to the raw UA string.

export function parseUserAgent(ua: string): string {
  if (!ua) return 'Unknown device'

  let browser = ''
  if (/Edg\//.test(ua)) browser = 'Edge'
  else if (/OPR\//.test(ua) || /Opera/.test(ua)) browser = 'Opera'
  else if (/YaBrowser/.test(ua)) browser = 'Yandex'
  else if (/Firefox\//.test(ua)) browser = 'Firefox'
  else if (/Chrome\//.test(ua)) browser = 'Chrome'
  else if (/Safari\//.test(ua) && /Version\//.test(ua)) browser = 'Safari'

  let os = ''
  if (/Windows NT/.test(ua)) os = 'Windows'
  else if (/Android/.test(ua)) os = 'Android'
  else if (/iPhone|iPad|iPod/.test(ua)) os = 'iOS'
  else if (/Macintosh|Mac OS X/.test(ua)) os = 'macOS'
  else if (/Linux/.test(ua)) os = 'Linux'

  if (browser && os) return `${browser} on ${os}`
  if (browser) return browser
  if (os) return os
  return ua.length > 60 ? ua.slice(0, 57) + '...' : ua
}
```

- [ ] **Step 4: Type-check**

```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit
```

Expected: green.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/api/sessions.ts frontend/web/src/composables/useSessions.ts frontend/web/src/utils/userAgent.ts
git commit -m "$(cat <<'EOF'
feat(web): sessionsApi + useSessions composable + UA parser

Frontend plumbing for the upcoming Active Sessions card:
- sessionsApi.{list,revoke,revokeOthers}
- useSessions() exposes reactive list + actions
- parseUserAgent() turns raw UA strings into "Chrome on Linux" labels

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 9: Frontend — Active Sessions card component

**Files:**
- Create: `frontend/web/src/components/profile/ActiveSessionsCard.vue`

- [ ] **Step 1: Build the card**

```vue
<script setup lang="ts">
import { onMounted } from 'vue'
import { useSessions } from '@/composables/useSessions'
import { parseUserAgent } from '@/utils/userAgent'
import { useI18n } from 'vue-i18n'
import Button from '@/components/ui/Button.vue'

const { t } = useI18n()
const { sessions, loading, error, refresh, revoke, revokeOthers } = useSessions()

onMounted(refresh)

function relative(iso: string): string {
  const diffSec = (Date.now() - new Date(iso).getTime()) / 1000
  if (diffSec < 60) return t('profile.settings.sessions.justNow')
  if (diffSec < 3600) return t('profile.settings.sessions.minutesAgo', { n: Math.floor(diffSec / 60) })
  if (diffSec < 86400) return t('profile.settings.sessions.hoursAgo', { n: Math.floor(diffSec / 3600) })
  return t('profile.settings.sessions.daysAgo', { n: Math.floor(diffSec / 86400) })
}

async function onRevokeOthers() {
  if (!confirm(t('profile.settings.sessions.confirmRevokeOthers'))) return
  await revokeOthers()
}
</script>

<template>
  <section class="rounded-xl bg-white/5 border border-white/10 p-5 space-y-4">
    <header class="flex items-center justify-between">
      <h3 class="text-base font-semibold text-white">
        {{ $t('profile.settings.sessions.title') }}
      </h3>
      <button
        class="text-xs text-white/60 hover:text-white"
        :disabled="loading"
        @click="refresh"
      >
        {{ $t('profile.settings.sessions.refresh') }}
      </button>
    </header>

    <p class="text-sm text-white/60">
      {{ $t('profile.settings.sessions.description') }}
    </p>

    <div v-if="loading && sessions.length === 0" class="text-sm text-white/40">
      {{ $t('profile.settings.sessions.loading') }}
    </div>

    <div v-else-if="error" class="text-sm text-red-400">
      {{ error }}
    </div>

    <ul v-else class="space-y-2">
      <li
        v-for="s in sessions"
        :key="s.id"
        class="flex items-start gap-3 rounded-lg bg-black/20 border border-white/5 p-3"
      >
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <span class="text-sm font-medium text-white truncate">
              {{ parseUserAgent(s.user_agent) }}
            </span>
            <span
              v-if="s.is_current"
              class="text-[10px] uppercase tracking-wide text-emerald-400 bg-emerald-400/10 border border-emerald-400/30 rounded px-1.5 py-0.5"
            >
              {{ $t('profile.settings.sessions.thisDevice') }}
            </span>
          </div>
          <div class="text-xs text-white/50 mt-1">
            {{ s.ip || $t('profile.settings.sessions.unknownIp') }} ·
            {{ $t('profile.settings.sessions.lastSeen') }} {{ relative(s.last_seen_at) }}
          </div>
        </div>
        <Button
          v-if="!s.is_current"
          variant="secondary"
          size="sm"
          @click="revoke(s.id)"
        >
          {{ $t('profile.settings.sessions.revoke') }}
        </Button>
      </li>
    </ul>

    <footer v-if="sessions.length > 1" class="pt-2 border-t border-white/5">
      <Button variant="secondary" size="sm" @click="onRevokeOthers">
        {{ $t('profile.settings.sessions.revokeAllOthers') }}
      </Button>
    </footer>
  </section>
</template>
```

If `Button` lives at a different path, run `grep -rln "import Button" frontend/web/src/views/Profile.vue` to find the correct one and adjust the import.

- [ ] **Step 2: Type-check**

```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/profile/ActiveSessionsCard.vue
git commit -m "$(cat <<'EOF'
feat(web): ActiveSessionsCard component

Lists alive sessions with parsed UA + relative last-seen, lets the
user revoke any non-current session, and "Sign out everywhere else"
nukes all but the current one.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 10: Mount the card in Profile.vue Settings tab

**Files:**
- Modify: `frontend/web/src/views/Profile.vue`

- [ ] **Step 1: Import the new card**

In the `<script setup>` block of `Profile.vue`, add the import (place near other component imports — `grep -n "from '@/components/profile" frontend/web/src/views/Profile.vue` shows where they cluster):

```ts
import ActiveSessionsCard from '@/components/profile/ActiveSessionsCard.vue'
```

- [ ] **Step 2: Insert the card in the Settings tab**

In the Settings tab template, find the `<!-- API Key -->` block (around line 748 per the codebase scan). Place `<ActiveSessionsCard class="mt-6" />` immediately AFTER the API Key card's closing tag — adjacent and visually consistent. If you can't tell which closing `</section>` or `</div>` belongs to the API Key card, use the indentation and the `v-else` branch boundaries as your guide.

- [ ] **Step 3: Add i18n strings**

Open `frontend/web/src/locales/en.json`. Find the existing `profile.settings` block. Add:

```json
"sessions": {
  "title": "Active Sessions",
  "description": "Devices currently signed in to your account. Revoke any you don't recognize.",
  "refresh": "Refresh",
  "loading": "Loading…",
  "thisDevice": "this device",
  "lastSeen": "last seen",
  "unknownIp": "unknown IP",
  "justNow": "just now",
  "minutesAgo": "{n}m ago",
  "hoursAgo": "{n}h ago",
  "daysAgo": "{n}d ago",
  "revoke": "Revoke",
  "revokeAllOthers": "Sign out everywhere else",
  "confirmRevokeOthers": "Sign out of all other devices?"
}
```

Do the same in `frontend/web/src/locales/ru.json` with translations:

```json
"sessions": {
  "title": "Активные сессии",
  "description": "Устройства, на которых сейчас выполнен вход. Отзывайте те, которые не узнаёте.",
  "refresh": "Обновить",
  "loading": "Загрузка…",
  "thisDevice": "это устройство",
  "lastSeen": "был(а)",
  "unknownIp": "IP неизвестен",
  "justNow": "только что",
  "minutesAgo": "{n} мин назад",
  "hoursAgo": "{n} ч назад",
  "daysAgo": "{n} дн назад",
  "revoke": "Отозвать",
  "revokeAllOthers": "Выйти на всех других устройствах",
  "confirmRevokeOthers": "Выйти со всех остальных устройств?"
}
```

- [ ] **Step 4: Build the frontend bundle**

```bash
cd /data/animeenigma/frontend/web && bun run build
```

Expected: clean build, no missing-key warnings.

- [ ] **Step 5: Visual smoke-test**

```bash
make redeploy-web
```

Open `https://animeenigma.ru/profile`, switch to Settings tab, scroll to find the new "Active Sessions" card. Should show one row with "this device" badge and the parsed UA.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/views/Profile.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "$(cat <<'EOF'
feat(web): mount ActiveSessionsCard in Profile Settings tab

Card sits adjacent to the API Key card. Adds en/ru i18n strings.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 11: Service-level test for legacy upgrade + grace path

**Files:**
- Create: `services/auth/internal/service/auth_session_test.go`

- [ ] **Step 1: Write the test**

```go
//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// Reuses the dev DB. Run via:
//   make dev
//   INTEGRATION=1 go test -tags=integration ./services/auth/internal/service/... -v
func newTestService(t *testing.T) (*service.AuthService, *repo.UserRepository, *repo.SessionRepository) {
	t.Helper()
	db := dbForTest(t) // helper from Task 3's test file — copy it inline if not shared
	require.NoError(t, db.AutoMigrate(&domain.User{}, &domain.UserSession{}))

	c, err := cache.New(cache.Config{Host: "localhost", Port: 6379})
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	uRepo := repo.NewUserRepository(db)
	sRepo := repo.NewSessionRepository(db)
	jwtCfg := authz.JWTConfig{
		Secret:          "test-secret",
		Issuer:          "test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	logr, _ := /* your logger pkg */ logger.New(...)  // adjust to your logger constructor
	return service.NewAuthService(uRepo, sRepo, c, jwtCfg, "", logr), uRepo, sRepo
}

func TestRefreshToken_PersistentPath_RotatesAndReturnsNewRT(t *testing.T) {
	svc, uRepo, _ := newTestService(t)
	ctx := context.Background()
	sc := service.SessionContext{UserAgent: "go-test", IP: "1.2.3.4"}

	user, err := svc.Register(ctx, &domain.RegisterRequest{Username: "u_" + time.Now().Format("150405"), Password: "password123"}, sc)
	require.NoError(t, err)
	require.NotEmpty(t, user.RefreshToken)
	first := user.RefreshToken

	// Refresh once → new RT issued, rotated=true.
	resp, rotated, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, sc)
	require.NoError(t, err)
	require.True(t, rotated)
	require.NotEqual(t, first, resp.RefreshToken)

	// Reuse old RT within grace → grace path, rotated=false.
	resp2, rotated2, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: first}, sc)
	require.NoError(t, err)
	require.False(t, rotated2)
	require.Empty(t, resp2.RefreshToken, "grace path should not return a new RT")
}

func TestRefreshToken_LegacyJWT_UpgradesToSession(t *testing.T) {
	svc, uRepo, sRepo := newTestService(t)
	ctx := context.Background()

	// Make a user
	user := &domain.User{Username: "legacy_" + time.Now().Format("150405"), PasswordHash: "x", Role: authz.RoleUser}
	require.NoError(t, uRepo.Create(ctx, user))

	// Mint a legacy 7-day refresh JWT directly via JWTManager.
	jm := authz.NewJWTManager(authz.JWTConfig{
		Secret: "test-secret", Issuer: "test",
		AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour,
	})
	pair, err := jm.GenerateTokenPair(user.ID, user.Username, user.Role, "")
	require.NoError(t, err)

	// Refresh using the legacy JWT — should upgrade.
	resp, rotated, err := svc.RefreshToken(ctx, &domain.RefreshRequest{RefreshToken: pair.RefreshToken}, service.SessionContext{UserAgent: "legacy", IP: "9.9.9.9"})
	require.NoError(t, err)
	require.True(t, rotated, "legacy upgrade should return rotated=true so handler sets cookie")
	require.NotEmpty(t, resp.RefreshToken)
	require.True(t, len(resp.RefreshToken) > 5 && resp.RefreshToken[:3] == "rt_", "upgraded token should be opaque rt_*")

	// Confirm a session row now exists for the user.
	sessions, err := sRepo.ListAlive(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}
```

(The exact `logger.New(...)` call depends on your codebase — replace with what's already used in other auth tests; if no auth tests use a logger, the Default() should be fine: `logger.Default()`.)

- [ ] **Step 2: Run**

```bash
cd /data/animeenigma && INTEGRATION=1 go test -tags=integration ./services/auth/internal/service/... -v -run TestRefreshToken
```

Expected: 2 PASS.

- [ ] **Step 3: Commit**

```bash
cd /data/animeenigma
git add services/auth/internal/service/auth_session_test.go
git commit -m "$(cat <<'EOF'
test(auth): persistent path rotation + grace path + legacy upgrade

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 12: Playwright e2e — list + revoke

**Files:**
- Create: `frontend/web/tests/e2e/sessions.spec.ts`

- [ ] **Step 1: Write the test**

```ts
import { test, expect } from '@playwright/test'

const UI_AUDIT_API_KEY = process.env.UI_AUDIT_API_KEY!
const PASSWORD = 'audit_bot_test_password_2026'

test.describe('Active Sessions', () => {
  test('lists current session and revokes others', async ({ browser }) => {
    // First context — "primary device"
    const ctxA = await browser.newContext()
    const pageA = await ctxA.newPage()
    await pageA.goto('https://animeenigma.ru/login')
    await pageA.fill('input[name="username"]', 'ui_audit_bot')
    await pageA.fill('input[type="password"]', PASSWORD)
    await Promise.all([
      pageA.waitForURL(/\/$|\/profile|\/home/),
      pageA.click('button[type="submit"]'),
    ])

    // Second context — "other device"
    const ctxB = await browser.newContext()
    const pageB = await ctxB.newPage()
    await pageB.goto('https://animeenigma.ru/login')
    await pageB.fill('input[name="username"]', 'ui_audit_bot')
    await pageB.fill('input[type="password"]', PASSWORD)
    await Promise.all([
      pageB.waitForURL(/\/$|\/profile|\/home/),
      pageB.click('button[type="submit"]'),
    ])

    // Open Settings tab on context A
    await pageA.goto('https://animeenigma.ru/profile')
    await pageA.click('text=Settings')
    const card = pageA.locator('section', { hasText: 'Active Sessions' })
    await expect(card).toBeVisible()

    // Two sessions present, one tagged "this device"
    const items = card.locator('li')
    await expect(items).toHaveCount(2)
    await expect(card.locator('text=this device')).toHaveCount(1)

    // Revoke the other one
    await card.locator('button:has-text("Revoke")').first().click()
    await expect(items).toHaveCount(1)

    // Context B's next access-token expiry should boot it; force the issue
    // by calling /api/auth/refresh and expecting 401.
    const refreshResp = await pageB.request.post('https://animeenigma.ru/api/auth/refresh')
    expect(refreshResp.status()).toBe(401)

    await ctxA.close()
    await ctxB.close()
  })
})
```

- [ ] **Step 2: Run**

```bash
cd /data/animeenigma/frontend/web && bunx playwright test sessions.spec.ts --reporter=list
```

Expected: PASS. If the login form selectors don't match, run an existing e2e to see the canonical login flow and copy from there.

- [ ] **Step 3: Commit**

```bash
cd /data/animeenigma
git add frontend/web/tests/e2e/sessions.spec.ts
git commit -m "$(cat <<'EOF'
test(e2e): two-context session list + revoke flow

Logs in twice, revokes the second context from the first, asserts the
second context's /auth/refresh returns 401.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 13: Run after-update skill

- [ ] **Step 1: Invoke `/animeenigma-after-update`**

Per CLAUDE.md, after any implementation work the after-update skill must run. It will:
- Lint + build the affected services
- Redeploy auth + web
- Health-check
- Append entries to `frontend/web/public/changelog.json` (the user-facing changelog)
- Commit + push everything

When prompted for changelog entries, use these (they're enthusiastic, emoji-friendly per CLAUDE.md, and user-facing — describe the experience, not the implementation):

```
- 🔐 Stay signed in indefinitely — no more random log-outs
- 📱 New "Active Sessions" panel in Settings: see every device that's signed in to your account, and kick any of them out with one click
- 🚪 "Sign out everywhere else" button for when you forgot to sign out at a friend's place
```

---

## Self-review (already done; included for reference)

- **Spec coverage:** every section in the spec maps to a task above. Sliding 30d → SessionTTL constant in Task 4. CAS+grace → repo Rotate in Task 3 + handler grace handling in Task 5. Legacy migration → dual-path RefreshToken in Task 4. Settings UI → Tasks 8-10. Cleanup goroutine → Task 7.
- **Placeholder scan:** No TBDs. The two soft notes ("adjust Button import path", "adjust logger.New") are explicit fallbacks with grep commands.
- **Type consistency:** `SessionID` (Go field) ↔ `sid` (JWT claim) ↔ `is_current` (JSON to frontend) ↔ `is_current` (TS type). `SessionContext` consistent across service.go and handler.go. Repo method names (`Create`, `FindAliveByHash`, `Rotate`, `Revoke`, `RevokeOthers`, `ListAlive`, `Cleanup`) used identically in service.

---

## Follow-up (NOT in this plan, do after 7+ days in production)

After enough time has passed that no legacy 7-day refresh-JWT can still be valid, drop the legacy fallback path from `RefreshToken` (Task 4 Step 5, the `// 2) Legacy JWT path` block) and the related Redis blacklist write. One small commit; spec section "Migration" already says this is the plan.
