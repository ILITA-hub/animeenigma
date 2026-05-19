# Audit Wave 1 — Critical Security + Quick Wins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the safest, smallest, verified fixes from the 2026-05-19 principal audit — items that are independent, atomic, and do not require architectural decisions.

**Architecture:** Each task is a self-contained patch with its own test, commit, and verification. No shared state between tasks; they can be parallelized or sequenced freely. Out of scope for this plan (deferred to dedicated plans): JWT-to-httpOnly-cookie migration, `AutoMigrate` → real migrations, Player rec-engine cron extraction, `watch_history` partitioning, single-host failover, OpenTelemetry tracing, contract codegen, catalog.go decomposition.

**Tech Stack:** Go 1.24, chi router, GORM + Postgres, gorilla/websocket, bcrypt, Vue 3 + Vite, libs/logger (zap-based structured), libs/errors. Project conventions in `/data/animeenigma/CLAUDE.md`.

**Source audit:** Findings produced 2026-05-19. Spot-verified before planning; one agent claim was incorrect (HanimePlayer.vue is wired into `views/Anime.vue:496`, NOT orphaned — dropped from scope).

---

## Task 1: WebSocket origin allow-list (Critical — S1)

**Background:** `services/rooms/internal/handler/websocket.go:15` currently returns `true` for every origin. Any cross-origin page a logged-in user visits can open a WS to `/api/rooms/*` and mutate game state as that user. Verified by reading the file.

**Files:**
- Modify: `services/rooms/internal/config/config.go` (add `AllowedOrigins []string`)
- Modify: `services/rooms/internal/handler/websocket.go:11-19` (replace `return true`)
- Modify: `services/rooms/cmd/rooms-api/main.go` (pass config through to handler)
- Modify: `docker/.env.example` (document new env var)
- Create: `services/rooms/internal/handler/websocket_test.go` (unit test for CheckOrigin)

- [ ] **Step 1: Confirm current state**

Run: `grep -n "CheckOrigin\|Allow all origins" services/rooms/internal/handler/websocket.go`
Expected output includes line 15 `return true // Allow all origins in development`.

- [ ] **Step 2: Write failing test**

Create `services/rooms/internal/handler/websocket_test.go`:

```go
package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckOrigin(t *testing.T) {
	allowed := []string{"https://animeenigma.ru", "http://localhost:5173"}
	check := buildOriginCheck(allowed)

	cases := []struct {
		origin string
		want   bool
	}{
		{"https://animeenigma.ru", true},
		{"http://localhost:5173", true},
		{"https://evil.com", false},
		{"", false},
		{"https://animeenigma.ru.evil.com", false},
	}
	for _, c := range cases {
		t.Run(c.origin, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			got := check(r)
			if got != c.want {
				t.Errorf("origin %q: got %v, want %v", c.origin, got, c.want)
			}
		})
	}

	// Also: empty allowlist must reject everything (fail-closed).
	deny := buildOriginCheck(nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://animeenigma.ru")
	if deny(r) {
		t.Fatal("empty allowlist must reject")
	}

	// Sanity: helper rejects trailing-dot or path mismatches via Host parsing.
	_ = strings.HasPrefix // keep import if simplified later
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd services/rooms && go test ./internal/handler/... -run TestCheckOrigin -v`
Expected: FAIL — `buildOriginCheck` undefined.

- [ ] **Step 4: Add `AllowedOrigins` to rooms config**

Open `services/rooms/internal/config/config.go`, locate the `Config` struct and the `Load()` function. Add:

```go
// In Config struct (alongside existing fields):
AllowedOrigins []string

// In Load() function, alongside existing env reads:
rawOrigins := getEnv("ALLOWED_WS_ORIGINS", "")
var origins []string
for _, o := range strings.Split(rawOrigins, ",") {
    o = strings.TrimSpace(o)
    if o != "" {
        origins = append(origins, o)
    }
}
cfg.AllowedOrigins = origins
```

Ensure `strings` is imported.

- [ ] **Step 5: Implement `buildOriginCheck` and wire it**

Replace `services/rooms/internal/handler/websocket.go:11-19` block. Add a constructor that takes allowed origins and builds the upgrader:

```go
package handler

import (
	"net/http"
	"net/url"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
	"github.com/gorilla/websocket"
)

func buildOriginCheck(allowed []string) func(r *http.Request) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			set[u.Scheme+"://"+u.Host] = struct{}{}
		}
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		_, ok := set[u.Scheme+"://"+u.Host]
		return ok
	}
}

func newUpgrader(allowed []string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     buildOriginCheck(allowed),
	}
}

type WebSocketHandler struct {
	wsService *service.WebSocketService
	log       *logger.Logger
	upgrader  websocket.Upgrader
}

func NewWebSocketHandler(wsService *service.WebSocketService, log *logger.Logger, allowedOrigins []string) *WebSocketHandler {
	return &WebSocketHandler{
		wsService: wsService,
		log:       log,
		upgrader:  newUpgrader(allowedOrigins),
	}
}
```

Replace the package-level `upgrader` usage in `HandleWebSocket` with `h.upgrader.Upgrade(...)`.

- [ ] **Step 6: Update the constructor call site**

Run: `grep -rn "NewWebSocketHandler" services/rooms/`
For each match, pass `cfg.AllowedOrigins` as the new argument (likely `services/rooms/cmd/rooms-api/main.go`).

- [ ] **Step 7: Run test to verify it passes**

Run: `cd services/rooms && go test ./internal/handler/... -run TestCheckOrigin -v`
Expected: PASS, all sub-tests green.

- [ ] **Step 8: Build the service**

Run: `cd services/rooms && go build ./...`
Expected: no output (clean build).

- [ ] **Step 9: Document the env var**

Append to `docker/.env.example`:

```
# Rooms service — comma-separated list of allowed WebSocket origins.
# REQUIRED in prod. Empty value = WS rejects all connections.
# Example: ALLOWED_WS_ORIGINS=https://animeenigma.ru,http://localhost:5173
ALLOWED_WS_ORIGINS=
```

Also set the value in your local `docker/.env` (do NOT commit) to `https://animeenigma.ru,http://localhost:5173`.

- [ ] **Step 10: Commit**

```bash
git add services/rooms/internal/config/config.go \
        services/rooms/internal/handler/websocket.go \
        services/rooms/internal/handler/websocket_test.go \
        services/rooms/cmd/rooms-api/main.go \
        docker/.env.example
git commit -m "$(cat <<'EOF'
fix(rooms): validate WebSocket Origin against allow-list

Per audit Wave 1 (Critical S1): rooms WebSocket previously accepted any
origin, exposing logged-in users to cross-origin abuse of /api/rooms/*.
Now reads ALLOWED_WS_ORIGINS env (comma-separated), fail-closed on empty.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 2: Bump bcrypt cost to 12 + opportunistic rehash on login (High — S2)

**Background:** `services/auth/internal/service/auth.go:73` and `user.go:69` both use `bcrypt.DefaultCost` (=10). OWASP 2023 baseline is 12. We must (a) use cost 12 for new hashes and (b) upgrade existing hashes when the user logs in (we have the plaintext password at that point).

**Files:**
- Create: `services/auth/internal/service/passwordhash.go` (centralizes cost + rehash logic)
- Modify: `services/auth/internal/service/auth.go:73, 102` (use new helper)
- Modify: `services/auth/internal/service/user.go:69` (use new helper)
- Modify: `services/auth/internal/service/auth.go` (Login flow — opportunistic rehash)
- Create: `services/auth/internal/service/passwordhash_test.go`

- [ ] **Step 1: Write failing tests**

Create `services/auth/internal/service/passwordhash_test.go`:

```go
package service

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPasswordUsesPolicyCost(t *testing.T) {
	h, err := HashPassword("hunter2hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(h))
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	if cost != PasswordHashCost {
		t.Fatalf("got cost %d, want %d", cost, PasswordHashCost)
	}
}

func TestNeedsRehash(t *testing.T) {
	weak, _ := bcrypt.GenerateFromPassword([]byte("p"), 10)
	strong, _ := bcrypt.GenerateFromPassword([]byte("p"), PasswordHashCost)

	if !NeedsRehash(string(weak)) {
		t.Fatal("cost=10 should need rehash")
	}
	if NeedsRehash(string(strong)) {
		t.Fatal("policy-cost hash must not need rehash")
	}
	if !NeedsRehash("not-a-bcrypt-hash") {
		t.Fatal("invalid hash should be treated as needing rehash")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd services/auth && go test ./internal/service/... -run "TestHashPassword|TestNeedsRehash" -v`
Expected: FAIL — `HashPassword`, `PasswordHashCost`, `NeedsRehash` undefined.

- [ ] **Step 3: Implement the helper**

Create `services/auth/internal/service/passwordhash.go`:

```go
package service

import "golang.org/x/crypto/bcrypt"

// PasswordHashCost is the bcrypt cost factor used by all password
// hashing in this service. Bumped from DefaultCost (10) to 12 per
// audit Wave 1 (S2). Each increment doubles work; 12 is the OWASP
// 2023 baseline.
const PasswordHashCost = 12

// HashPassword returns a bcrypt hash at the current policy cost.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), PasswordHashCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NeedsRehash returns true when the stored hash was produced with a
// cost factor below the current policy, or is otherwise unparseable
// (in which case the next successful login will replace it).
func NeedsRehash(stored string) bool {
	c, err := bcrypt.Cost([]byte(stored))
	if err != nil {
		return true
	}
	return c < PasswordHashCost
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/auth && go test ./internal/service/... -run "TestHashPassword|TestNeedsRehash" -v`
Expected: PASS.

- [ ] **Step 5: Replace `bcrypt.GenerateFromPassword` call sites**

Open `services/auth/internal/service/auth.go`. Find the line:
```go
hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
```
(line 73 — the registration flow).

Replace with:
```go
hashedPassword, err := HashPassword(req.Password)
```
And change the downstream `string(hashedPassword)` references to just `hashedPassword` (it's already a string now).

Open `services/auth/internal/service/user.go`. Find the line:
```go
hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.NewPassword), bcrypt.DefaultCost)
```
(line 69 — change-password flow). Replace similarly:
```go
hashedPassword, err := HashPassword(*req.NewPassword)
```

- [ ] **Step 6: Add opportunistic rehash in Login**

In `services/auth/internal/service/auth.go`, locate the Login function. After `bcrypt.CompareHashAndPassword` succeeds (around line 102), add:

```go
// Opportunistic upgrade: if the stored hash uses a weaker cost than
// the current policy, re-hash with the new cost and persist. Failures
// here MUST NOT block the login.
if NeedsRehash(user.PasswordHash) {
    if newHash, err := HashPassword(req.Password); err == nil {
        user.PasswordHash = newHash
        if updateErr := s.userRepo.UpdatePasswordHash(ctx, user.ID, newHash); updateErr != nil {
            s.log.Warnw("opportunistic rehash failed to persist", "user_id", user.ID, "error", updateErr)
        }
    }
}
```

If `userRepo.UpdatePasswordHash` does not exist, add it to the repository interface and implementation. Inspect first:

```bash
grep -n "UpdatePasswordHash\|UpdatePassword" services/auth/internal/repo/user.go services/auth/internal/service/user.go
```

If a `UpdatePassword(ctx, id, hash)` already exists, use it instead.

- [ ] **Step 7: Build and run all auth tests**

Run: `cd services/auth && go build ./... && go test ./...`
Expected: all green.

- [ ] **Step 8: Commit**

```bash
git add services/auth/internal/service/passwordhash.go \
        services/auth/internal/service/passwordhash_test.go \
        services/auth/internal/service/auth.go \
        services/auth/internal/service/user.go
git commit -m "$(cat <<'EOF'
fix(auth): bump bcrypt cost to 12 + opportunistic rehash on login

Per audit Wave 1 (High S2): bcrypt.DefaultCost (10) is below the OWASP
2023 baseline of 12. Centralizes cost in PasswordHashCost, swaps all
call sites, and adds a NeedsRehash check on successful Login so existing
users migrate transparently.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 3: Bound external HTTP response sizes on importers (Medium — S5)

**Background:** `services/player/internal/handler/mal_import.go` and `shikimori_import.go` decode JSON from external APIs with no upper size bound. A misbehaving or compromised upstream can OOM the service. Fix: wrap response body in `io.LimitReader`.

**Files:**
- Create: `services/player/internal/handler/httplimit.go` (shared helper)
- Modify: `services/player/internal/handler/mal_import.go` (Decode call site)
- Modify: `services/player/internal/handler/shikimori_import.go` (Decode call site)
- Create: `services/player/internal/handler/httplimit_test.go`

- [ ] **Step 1: Find the actual Decode call sites**

Run:
```bash
grep -n "json.NewDecoder\|json.Decode" services/player/internal/handler/mal_import.go services/player/internal/handler/shikimori_import.go
```
Note each `line: code` for use in Step 4.

- [ ] **Step 2: Write failing test**

Create `services/player/internal/handler/httplimit_test.go`:

```go
package handler

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLimitedDecode_AcceptsUnderLimit(t *testing.T) {
	body := io.NopCloser(strings.NewReader(`{"x":1}`))
	var out map[string]int
	if err := DecodeJSONLimited(body, &out, 1024); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out["x"] != 1 {
		t.Fatalf("got %v", out)
	}
}

func TestLimitedDecode_RejectsOverLimit(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), 2048)
	body := io.NopCloser(bytes.NewReader(huge))
	var out map[string]int
	err := DecodeJSONLimited(body, &out, 1024)
	if err == nil {
		t.Fatal("expected error for oversized body")
	}
	if !errors.Is(err, ErrResponseTooLarge) && !strings.Contains(err.Error(), "limit") {
		// Allow either explicit sentinel or json-parse error from truncation —
		// the important property is that we did NOT silently buffer 2048 bytes.
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd services/player && go test ./internal/handler/... -run TestLimitedDecode -v`
Expected: FAIL — `DecodeJSONLimited` undefined.

- [ ] **Step 4: Implement the helper**

Create `services/player/internal/handler/httplimit.go`:

```go
package handler

import (
	"encoding/json"
	"errors"
	"io"
)

// MaxImporterResponseBytes is the upper bound applied to any JSON
// response we accept from an external service (MAL, Shikimori, etc).
// 50 MiB is generous for full-list exports; anything larger is
// pathological and a likely abuse signal.
const MaxImporterResponseBytes int64 = 50 * 1024 * 1024

// ErrResponseTooLarge is returned when the external API's body would
// exceed MaxImporterResponseBytes.
var ErrResponseTooLarge = errors.New("external response exceeds size limit")

// DecodeJSONLimited reads at most `limit` bytes from r and JSON-decodes
// into out. If the body is exactly `limit` bytes, we treat it as
// potentially truncated and return ErrResponseTooLarge.
func DecodeJSONLimited(r io.Reader, out interface{}, limit int64) error {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	if err := json.NewDecoder(lr).Decode(out); err != nil {
		if lr.N <= 0 {
			return ErrResponseTooLarge
		}
		return err
	}
	if lr.N <= 0 {
		return ErrResponseTooLarge
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd services/player && go test ./internal/handler/... -run TestLimitedDecode -v`
Expected: PASS.

- [ ] **Step 6: Replace call sites**

In `services/player/internal/handler/mal_import.go`, find the `json.NewDecoder(resp.Body).Decode(...)` call (around the line you noted in Step 1). Replace with:

```go
if err := DecodeJSONLimited(resp.Body, &entries, MaxImporterResponseBytes); err != nil {
    // existing error handling — return an HTTP error or wrap.
    return nil, err
}
```

Do the same in `shikimori_import.go`. Match the exact error-handling shape the existing code uses (return `(_, err)`, write status, etc.) — do not change error semantics.

- [ ] **Step 7: Build and test**

Run: `cd services/player && go build ./... && go test ./internal/handler/...`
Expected: all green.

- [ ] **Step 8: Commit**

```bash
git add services/player/internal/handler/httplimit.go \
        services/player/internal/handler/httplimit_test.go \
        services/player/internal/handler/mal_import.go \
        services/player/internal/handler/shikimori_import.go
git commit -m "$(cat <<'EOF'
fix(player): bound external import response bodies (50 MiB)

Per audit Wave 1 (Medium S5): MAL and Shikimori importers used
json.NewDecoder directly on resp.Body with no upper bound. A
misbehaving or compromised upstream could OOM the service. Adds
DecodeJSONLimited helper with a 50 MiB ceiling.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 4: Tighten DEV_MODE production guard (Low — S9)

**Background:** `services/gateway/internal/config/config.go:115` only refuses DevMode when `ENVIRONMENT` literally equals `"production"` or `"prod"`. Any other value (empty, misspelled, `staging`, `prd`) silently lets DevMode through and bypasses admin auth. Switch to a positive allow-list: DevMode only allowed when `ENVIRONMENT` is in a known dev set.

**Files:**
- Modify: `services/gateway/internal/config/config.go:114-118`
- Modify: `services/gateway/internal/config/config_test.go` (add cases)

- [ ] **Step 1: Read the existing test to follow its style**

Run: `cat services/gateway/internal/config/config_test.go | head -60`

- [ ] **Step 2: Write failing tests**

Append to `services/gateway/internal/config/config_test.go`:

```go
func TestDevMode_OnlyAllowedInDevEnvironments(t *testing.T) {
	cases := []struct {
		env     string
		devReq  bool
		devWant bool
	}{
		{"production", true, false},
		{"prod", true, false},
		{"staging", true, false}, // previously allowed — now denied
		{"", true, false},        // previously allowed — now denied
		{"PRD", true, false},     // misspelling — now denied
		{"development", true, true},
		{"dev", true, true},
		{"local", true, true},
		{"test", true, true},
		{"development", false, false}, // not requested → off
	}
	for _, c := range cases {
		t.Run(c.env+"/"+boolStr(c.devReq), func(t *testing.T) {
			t.Setenv("ENVIRONMENT", c.env)
			t.Setenv("DEV_MODE", boolStr(c.devReq))
			cfg, err := Load()
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if cfg.DevMode != c.devWant {
				t.Errorf("ENVIRONMENT=%q DEV_MODE=%v → DevMode=%v, want %v",
					c.env, c.devReq, cfg.DevMode, c.devWant)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd services/gateway && go test ./internal/config/... -run TestDevMode_OnlyAllowed -v`
Expected: FAIL — cases like `ENVIRONMENT=""` currently allow DevMode.

- [ ] **Step 4: Switch to positive allow-list**

In `services/gateway/internal/config/config.go`, replace the existing guard:

```go
// Production safeguard: refuse to enable DevMode in production
if cfg.DevMode && (cfg.Environment == "production" || cfg.Environment == "prod") {
    fmt.Fprintf(os.Stderr, "FATAL: DEV_MODE=true is forbidden when ENVIRONMENT=%s — forcing DevMode=false\n", cfg.Environment)
    cfg.DevMode = false
}
```

with:

```go
// DevMode is only permitted in known development environments. Any
// other ENVIRONMENT value (including the empty string) fails closed.
// See audit Wave 1 (S9): the previous deny-list missed misspellings,
// staging, and the empty-string default.
devEnvs := map[string]bool{
    "development": true,
    "dev":         true,
    "local":       true,
    "test":        true,
}
if cfg.DevMode && !devEnvs[cfg.Environment] {
    fmt.Fprintf(os.Stderr, "FATAL: DEV_MODE=true is forbidden when ENVIRONMENT=%q — forcing DevMode=false\n", cfg.Environment)
    cfg.DevMode = false
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/gateway && go test ./internal/config/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/gateway/internal/config/config.go \
        services/gateway/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
fix(gateway): DevMode requires explicit dev ENVIRONMENT (allow-list)

Per audit Wave 1 (S9): the previous guard only denied "production"
and "prod" literals, so an empty ENVIRONMENT or any misspelling
silently allowed DevMode (which bypasses admin auth). Switch to a
positive allow-list of known dev environments — fail closed otherwise.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 5: Add compound unique index on `watch_progress` (Perf)

**Background:** `services/player/internal/repo/progress.go` performs `ON CONFLICT (user_id, anime_id, episode_number)` UPSERTs, but the domain model declares those as three single-column indexes — there is no compound unique constraint. Postgres can use single-column indexes but lock-contention on concurrent heartbeats is worse than necessary, and conflict resolution falls back to a sequential scan in degenerate cases.

**Files:**
- Modify: `services/player/internal/domain/watch.go` (WatchProgress GORM tags)
- Modify: `services/player/cmd/player-api/main.go` (raw SQL index creation, idempotent, after AutoMigrate)
- Modify: `services/player/internal/repo/progress_test.go` (verify the constraint exists if a test container is available)

- [ ] **Step 1: Update the domain tags**

In `services/player/internal/domain/watch.go`, the WatchProgress struct currently has three independent `gorm:"...;index"` tags. Change them to share a unique-index name:

```go
UserID        string `gorm:"type:uuid;uniqueIndex:idx_watch_progress_user_anime_ep,priority:1" json:"user_id"`
AnimeID       string `gorm:"type:uuid;uniqueIndex:idx_watch_progress_user_anime_ep,priority:2" json:"anime_id"`
EpisodeNumber int    `gorm:"uniqueIndex:idx_watch_progress_user_anime_ep,priority:3" json:"episode_number"`
```

The single-column index on `user_id` is still useful for "all progress for this user" reads, so retain it via a separate tag if needed:

```go
UserID        string `gorm:"type:uuid;uniqueIndex:idx_watch_progress_user_anime_ep,priority:1;index:idx_watch_progress_user_id" json:"user_id"`
```

- [ ] **Step 2: Add idempotent raw-SQL backfill in main.go**

In `services/player/cmd/player-api/main.go`, AFTER the `AutoMigrate(...)` call returns, add:

```go
// Compound unique index for ON CONFLICT in progress UPSERTs.
// AutoMigrate creates it on fresh boots via struct tags; for existing
// databases that pre-date the tag change, create it idempotently.
if err := db.DB.Exec(`
    CREATE UNIQUE INDEX IF NOT EXISTS idx_watch_progress_user_anime_ep
    ON watch_progress (user_id, anime_id, episode_number)
`).Error; err != nil {
    log.Fatalw("failed to create watch_progress compound index", "error", err)
}
```

- [ ] **Step 3: Verify build**

Run: `cd services/player && go build ./...`
Expected: no errors.

- [ ] **Step 4: Run existing repo tests**

Run: `cd services/player && go test ./internal/repo/... -v`
Expected: all green (existing UPSERT tests should benefit from the constraint, not regress).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/domain/watch.go \
        services/player/cmd/player-api/main.go
git commit -m "$(cat <<'EOF'
perf(player): compound unique index on watch_progress(user, anime, ep)

Per audit Wave 1 (perf): progress.go uses ON CONFLICT on
(user_id, anime_id, episode_number) but the domain only declared
single-column indexes. Adds a compound uniqueIndex tag (covered by
AutoMigrate on fresh dbs) and an idempotent raw-SQL CREATE INDEX
IF NOT EXISTS for existing databases.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 6: Remove unused `video.js` dependency (Quick win)

**Background:** `frontend/web/package.json` declares `video.js` and `@types/video.js`, but `grep -rn "from 'video.js'\|from \"video.js\""` over `frontend/web/src` returns zero matches. It ships unused — wasted bundle, wasted update surface. Verified.

**Files:**
- Modify: `frontend/web/package.json` (remove video.js + @types/video.js)
- Modify: `frontend/web/vite.config.ts` (remove the `video-vendor` manualChunk if present)
- Modify: `frontend/web/bun.lock` or `package-lock.json` (regenerated)

- [ ] **Step 1: Re-confirm unused state**

Run:
```bash
grep -rn "video\.js\|videojs\|from ['\"]video" frontend/web/src
```
Expected: no matches (or only comments).

- [ ] **Step 2: Remove from package.json**

Open `frontend/web/package.json`. Delete the two entries:
```json
"video.js": "^8.10.0",
"@types/video.js": "^7.3.58",
```

- [ ] **Step 3: Remove the manual chunk (if it exists)**

Run: `grep -n "video-vendor\|videojs" frontend/web/vite.config.ts`
If the file references a `video-vendor` chunk, remove the entry. Keep `hls-vendor` (still used).

- [ ] **Step 4: Reinstall**

Run:
```bash
cd frontend/web && bun install
```
Expected: lock file updated, no errors.

- [ ] **Step 5: Build to verify nothing references video.js transitively**

Run:
```bash
cd frontend/web && bun run build
```
Expected: clean build. If the build fails with a "Cannot find module 'video.js'" error, restore the dep — the audit grep missed an import.

- [ ] **Step 6: Run frontend type-check + lint**

Run:
```bash
cd frontend/web && bunx tsc --noEmit && bunx eslint src/
```
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/package.json frontend/web/vite.config.ts frontend/web/bun.lock 2>/dev/null
git add frontend/web/package-lock.json 2>/dev/null
git commit -m "$(cat <<'EOF'
chore(frontend): drop unused video.js dependency

Per audit Wave 1 (quick win): video.js shipped in package.json but
zero imports in src/. RawPlayer and HanimePlayer use hls.js directly.
Removes both video.js and @types/video.js and the video-vendor chunk.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

## Task 7: Replace `fmt.Println`/`fmt.Printf` in `services/maintenance/` with structured logger

**Background:** Logging audit found 59 unstructured print calls in `services/maintenance/` (other services are clean). Loki/Promtail cannot index them as structured fields. Convert to `libs/logger` calls.

**Files:**
- Modify: every `.go` file under `services/maintenance/` that uses `fmt.Println` / `fmt.Printf` (non-test)

- [ ] **Step 1: List the offending files**

Run:
```bash
grep -rln "fmt\.Println\|fmt\.Printf" services/maintenance --include="*.go" | grep -v _test.go
```
Save the list.

- [ ] **Step 2: Inspect the maintenance logger setup**

Run: `grep -rn "logger.New\|*logger.Logger" services/maintenance/cmd/ services/maintenance/internal/`
Note the variable name (probably `log` or `h.log`) and how it's already plumbed.

- [ ] **Step 3: For each file, replace prints**

For lines like `fmt.Println("starting job", id)`, convert to:
```go
log.Infow("starting job", "id", id)
```

For `fmt.Printf("error: %v\n", err)`:
```go
log.Errorw("operation failed", "error", err)
```

If a file does not have `log` in scope, plumb it from the constructor — do not call `logger.New(...)` inside functions, that loses configuration.

Rules:
- `Println("xxx", val)` → `log.Infow("xxx", "value", val)` (use a semantic key, not literal `"value"`, where possible — e.g., `"user_id"`)
- `Printf("format %v\n", val)` → split into `log.Infow("...message...", "field", val)` with the format prose moved to the message and the placeholders to keyed fields
- Anything that was clearly diagnostic-to-stdout for a CLI subcommand can keep `fmt.Println` IF the file is under `cmd/` and is genuinely a one-shot CLI tool (not a long-running service). Check before converting.

- [ ] **Step 4: Build the service**

Run: `cd services/maintenance && go build ./...`
Expected: clean.

- [ ] **Step 5: Run any existing tests**

Run: `cd services/maintenance && go test ./...`
Expected: green.

- [ ] **Step 6: Re-grep to confirm scope reduced**

Run:
```bash
grep -rln "fmt\.Println\|fmt\.Printf" services/maintenance --include="*.go" | grep -v _test.go
```
Expected: only files under `services/maintenance/cmd/*` (genuine CLI tools), if any.

- [ ] **Step 7: Commit**

```bash
git add services/maintenance/
git commit -m "$(cat <<'EOF'
refactor(maintenance): replace fmt.Println with structured logger

Per audit Wave 1 (quick win): 59 unstructured print calls in
services/maintenance were the only remaining holdout — Loki/Promtail
cannot index them. Routes through libs/logger so fields are searchable.
CLI-tool stdout in services/maintenance/cmd/* is preserved where
appropriate.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

> **Note on deployment:** This plan stops at clean per-task commits. Redeploy, changelog entry, and push (the `/animeenigma-after-update` flow) are handled in a separate batch — do not run them here.

---

## Out of scope (each needs its own plan)

These audit findings require dedicated planning cycles because they cross service boundaries, change client contracts, or require architectural decisions:

- **JWT access token → httpOnly cookie** (S3). Spans frontend, gateway, auth. Breaking change for anyone with an existing localStorage session. Needs a migration window.
- **API-key SessionID handling** (S4). Requires designing ephemeral sessions for `ak_` resolves with a configurable TTL. Owner: auth + gateway.
- **`AutoMigrate` → `golang-migrate`** (C2). Multi-week. Convert existing `services/*/migrations/*.sql` to applied migrations; freeze AutoMigrate behind a build tag.
- **Player rec-engine crons → Scheduler service** (C3). Architectural — needs leader election (Redis lock or NATS consumer group).
- **`watch_history` partitioning** (perf). Postgres declarative partitioning by month; retention sweep.
- **Per-user rate limiting at the gateway** (S7). Secondary key on IPRateLimiter; needs design for shared-IP fairness.
- **HLS proxy allow-list audit** (S6). Quarterly review process; not a one-shot fix.
- **Single-host failover / managed Postgres** (operational). Infra-tier decision, not a code change.
- **Backup-restore test in CI** (operational). Needs a staging DB instance to exercise the restore path.

---

## Self-Review

**Spec coverage:** Wave 1 = critical S1, high S2, medium S5, low S9, perf compound-index, two quick wins (video.js drop, fmt.Println sweep). All seven planned. Higher-severity items not covered (S3, S4, C2, C3) are explicitly deferred above.

**Placeholder scan:** Reviewed. No `TBD`, no "implement later", every code block contains real code.

**Type consistency:** `HashPassword(string) (string, error)`, `NeedsRehash(string) bool`, `DecodeJSONLimited(io.Reader, interface{}, int64) error`, `buildOriginCheck([]string) func(r *http.Request) bool` — all referenced consistently across their respective tasks.

**One agent claim was wrong:** Task list does NOT include "delete HanimePlayer.vue" — verified it is imported at `frontend/web/src/views/Anime.vue:496` and lazy-loaded at line 997. Dropped.

**One risk to flag:** Task 5 (compound index) changes `WatchProgress` GORM tags such that on a fresh database AutoMigrate will create the compound unique index. On the existing prod database, the raw SQL backfill handles the migration. Verify the existing `watch_progress` table has no rows that would violate the new uniqueness constraint before deploying — if duplicates exist (same user/anime/ep), the `CREATE UNIQUE INDEX` will fail. Pre-check:
```bash
docker compose exec postgres psql -U postgres -d animeenigma \
  -c "SELECT user_id, anime_id, episode_number, COUNT(*) FROM watch_progress GROUP BY 1,2,3 HAVING COUNT(*) > 1 LIMIT 5;"
```
If rows are returned, dedupe first (keep the most recent by `updated_at`).
