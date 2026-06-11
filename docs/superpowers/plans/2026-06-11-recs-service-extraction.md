# Recs Service Extraction + Quality Ride-Alongs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the recommendation engine from `services/player` into a new `services/recs` microservice (port 8094) with byte-identical behavior, then ship three quality improvements: ISS-026 watched-event instrumentation, the S7 dropped-penalty signal, and the S12 diversification re-rank.

**Architecture:** Shared `animeenigma` Postgres DB means the new service reads `anime_list`/`watch_history`/`animes`/`anime_genres`/`anime_tags`/`anime_studios` directly via GORM; only the on-write recompute trigger crosses the service boundary, as a fire-and-forget internal HTTP hint (clone of the GachaCreditProducer pattern). Gateway re-points the three existing URL families — zero frontend changes in Phase 1.

**Tech Stack:** Go 1.25 / chi / GORM / Redis (`libs/cache`), Vue 3 + TS frontend (bun), Docker Compose deploy on this production server.

**Spec:** `docs/superpowers/specs/2026-06-11-recs-service-extraction-design.md`

**Scores per phase** (per `.planning/CONVENTIONS.md`):
| Phase | UXΔ | CDI | MVQ |
|---|---|---|---|
| 1 Extraction | 0 (Ambiguous) | 0.06 * 21 | Basilisk 90%/85% |
| 2 ISS-026 | 0 (Ambiguous) | 0.01 * 3 | Sprite 80%/85% |
| 3 S7 | +2 (Better) | 0.02 * 5 | Griffin 85%/80% |
| 4 S12 | +3 (Better) | 0.02 * 8 | Phoenix 85%/75% |

**House rules that bind every task:**
- Commit ONLY to `main`, path-scoped (`git add <paths>` — NEVER `git add -A`; the shared tree has other agents' work). Push after every commit. If the shared tree is off main, land via worktree cherry-pick (see memory `feedback_always_work_on_main_worktrees`).
- Co-authors on every commit:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- This server IS production. `make redeploy-<svc>` deploys live.
- After each PHASE completes, invoke the `/animeenigma-after-update` skill (lint, redeploy, health, changelog in Russian Trump-mode, commit, push). Do not invoke it per-task.
- Tests: handwritten fakes, table-driven, no testify/mock for new code (existing moved tests keep whatever they use).

---

# PHASE 1 — Mechanical extraction (Tasks 1–11)

Behavior must be byte-identical. Task 1 captures the baseline BEFORE any code changes; Task 11 diffs against it.

### Task 1: Capture the byte-identical baseline

**Files:** none (produces `/tmp/recs-baseline-anon.json`, `/tmp/recs-baseline-user.json`)

- [ ] **Step 1: Get the ui_audit_bot API key and user id**

```bash
UI_KEY=$(grep '^UI_AUDIT_API_KEY=' docker/.env | cut -d= -f2)
UID=$(docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -tAc "SELECT id FROM users WHERE username='ui_audit_bot';")
echo "key=${UI_KEY:0:8}... uid=$UID"
```
Expected: non-empty key prefix and a UUID.

- [ ] **Step 2: Flush the recs cache keys so we capture a FRESH compute (not a cached envelope)**

```bash
docker compose -f docker/docker-compose.yml exec -T redis redis-cli DEL "recs:public:trending:topN" "recs:user:${UID}:topN:v2"
```

- [ ] **Step 3: Capture anonymous + logged-in rows, normalizing the volatile fields away**

```bash
curl -s http://localhost:8000/api/users/recs | jq 'del(.data.generated_at, .data.cache_hit)' > /tmp/recs-baseline-anon.json
curl -s http://localhost:8000/api/users/recs -H "Authorization: Bearer $UI_KEY" | jq 'del(.data.generated_at, .data.cache_hit)' > /tmp/recs-baseline-user.json
jq '.data.total' /tmp/recs-baseline-anon.json /tmp/recs-baseline-user.json
```
Expected: two non-zero totals. These files are the Phase-1 acceptance oracle — do not delete them.

- [ ] **Step 4: No commit** (nothing changed).

### Task 2: Scaffold `services/recs` (module + Dockerfile + go.work + Dockerfile fan-out)

**Files:**
- Create: `services/recs/go.mod`, `services/recs/Dockerfile`
- Modify: `go.work` (add `./services/recs`)
- Modify: EVERY service Dockerfile that copies the full go.work module set (all of: auth, catalog, streaming, player, rooms, scraper, scheduler, gateway, themes, notifications, watch-together, analytics, maintenance, library, gacha — i.e. every `services/*/Dockerfile` that already contains a `COPY services/gacha/go.mod` line; sidecars animepahe-resolver / megacloud-extractor are exempt)

- [ ] **Step 1: Create `services/recs/go.mod`**

```
module github.com/ILITA-hub/animeenigma/services/recs

go 1.25.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	gorm.io/gorm v1.30.0
)

replace (
	github.com/ILITA-hub/animeenigma/libs/authz => ../../libs/authz
	github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache
	github.com/ILITA-hub/animeenigma/libs/database => ../../libs/database
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
)
```
(`go mod tidy` in Task 6 will pull transitive deps — gorm postgres driver comes via `libs/database`. If the moved tests import `stretchr/testify`, tidy adds it automatically.)

- [ ] **Step 2: Create `services/recs/Dockerfile`** — copy `services/notifications/Dockerfile` verbatim, then apply exactly these substitutions: every `services/notifications` → `services/recs` in the `RUN cd` / final `COPY services/notifications/` / build lines (NOT in the go.mod COPY list — keep all existing module COPY lines, they are required by go.work), `notifications-api` → `recs-api`, `EXPOSE 8090` → `EXPOSE 8094`. Then add this line into the services go.mod COPY block (alphabetical placement is fine):

```dockerfile
COPY services/recs/go.mod services/recs/go.sum* ./services/recs/
```

- [ ] **Step 3: Add the same COPY line to every other service Dockerfile**

```bash
for f in $(grep -rl "COPY services/gacha/go.mod" services/*/Dockerfile); do
  grep -q "services/recs/go.mod" "$f" || sed -i '\|COPY services/gacha/go.mod|a COPY services/recs/go.mod services/recs/go.sum* ./services/recs/' "$f"
done
grep -rl "services/recs/go.mod" services/*/Dockerfile | wc -l
```
Expected: count equals the number of go.work-based service Dockerfiles (run `grep -rl "COPY go.work" services/*/Dockerfile | wc -l` to confirm they match). This is mandatory — a module listed in go.work but missing from any Dockerfile breaks EVERY service build (memory `Adding New libs/ Module`, same rule applies to services).

- [ ] **Step 4: Add to `go.work`** — insert `./services/recs` in the `use (...)` block next to `./services/player`, then run:

```bash
go work sync
```

- [ ] **Step 5: Commit**

```bash
git add services/recs/go.mod services/recs/Dockerfile go.work go.work.sum services/*/Dockerfile
git commit -m "feat(recs): scaffold services/recs module (port 8094)" # + co-author trailer
git push
```

### Task 3: Move the recs code from player to recs (git mv + import rewrite)

**Files:**
- Move: `services/player/internal/service/recs/**` → `services/recs/internal/service/recs/**`
- Move: `services/player/internal/handler/{recs.go,recs_test.go,recs_s6_pin_test.go,admin_recs.go,admin_recs_test.go,rec_events.go,rec_events_test.go}` → `services/recs/internal/handler/`
- Move: recs domain + repo files → `services/recs/internal/{domain,repo}/`

- [ ] **Step 1: Identify the recs domain files** (RecEvent lives in its own file — confirm):

```bash
ls services/player/internal/domain/ | grep -i rec
ls services/player/internal/repo/ | grep -i rec
```
Expected: `recs.go` (+ a `rec_event*.go` file) in domain; `recs.go`, `recs_test.go`, `rec_events.go`, `rec_events_test.go` in repo. Move ALL files these commands list.

- [ ] **Step 2: git mv everything**

```bash
mkdir -p services/recs/internal/{service,handler,domain,repo}
git mv services/player/internal/service/recs services/recs/internal/service/recs
git mv services/player/internal/handler/recs.go services/player/internal/handler/recs_test.go \
       services/player/internal/handler/recs_s6_pin_test.go \
       services/player/internal/handler/admin_recs.go services/player/internal/handler/admin_recs_test.go \
       services/player/internal/handler/rec_events.go services/player/internal/handler/rec_events_test.go \
       services/recs/internal/handler/
git mv services/player/internal/domain/recs.go services/recs/internal/domain/recs.go
git mv services/player/internal/repo/recs.go services/player/internal/repo/recs_test.go \
       services/player/internal/repo/rec_events.go services/player/internal/repo/rec_events_test.go \
       services/recs/internal/repo/
# plus the rec_event domain file(s) found in Step 1, e.g.:
# git mv services/player/internal/domain/rec_event.go services/recs/internal/domain/rec_event.go
```

- [ ] **Step 3: Rewrite import paths**

```bash
find services/recs -name '*.go' -exec sed -i 's|animeenigma/services/player/internal|animeenigma/services/recs/internal|g' {} +
grep -rn "services/player" services/recs/ && echo "LEFTOVERS — fix manually" || echo CLEAN
```
Expected: CLEAN.

- [ ] **Step 4: Check what the moved code pulled along.** The moved handler/domain/repo files may reference player-only packages. Run:

```bash
cd services/recs && go vet ./... 2>&1 | head -30
```
Expected failures at this point: missing `config`/`transport`/`main` (created in Tasks 4–6). If any moved file imports a player-internal package that did NOT move (e.g. `domain.AnimeListEntry`), the recs code must NOT import player internals — re-declare the minimal struct locally in `services/recs/internal/domain/` (the tables are shared; only the Go type is needed). Inspect each such error individually.

- [ ] **Step 5: Commit** (build is still red until Task 6 — that's fine, this commit is the pure move so the diff stays reviewable):

```bash
git add services/recs services/player/internal
git commit -m "refactor(recs): move recs engine code from player to services/recs (verbatim + import rewrite)"
git push
```

### Task 4: recs config + transport (new files)

**Files:**
- Create: `services/recs/internal/config/config.go`
- Create: `services/recs/internal/transport/router.go`

- [ ] **Step 1: Write `services/recs/internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig

	// CatalogURL is the catalog service base URL used by the S6 combo-pin
	// Shikimori /similar fallback (HTTPShikimoriSimilarClient).
	CatalogURL string
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnv("SERVER_PORT", "8094"))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: port,
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Name:     getEnv("DB_NAME", "animeenigma"),
		},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "localhost"),
			Port: getEnv("REDIS_PORT", "6379"),
		},
		JWT:        authz.JWTConfig{Secret: jwtSecret},
		CatalogURL: getEnv("CATALOG_URL", "http://catalog:8081"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```
**Verify field names against `services/notifications/internal/config/config.go`** — `database.Config` / `cache.Config` / `authz.JWTConfig` field shapes must match what notifications uses (e.g. if `database.Config.Port` is an int there, mirror it). Copy notifications' exact field construction; the listing above shows intent, notifications shows truth.

- [ ] **Step 2: Write `services/recs/internal/transport/router.go`** — port the three recs route groups verbatim from `services/player/internal/transport/router.go` (lines ~167–215) plus the standard middleware/health/metrics scaffold. Copy `OptionalAuthMiddleware` / `AuthMiddleware` / `AdminRoleMiddleware` from player's transport package (check `services/player/internal/transport/` for the middleware file and bring those functions along — they are small JWT wrappers around `libs/authz`):

```go
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/handler"
)

func NewRouter(
	recsHandler *handler.RecsHandler,
	adminRecsHandler *handler.AdminRecsHandler,
	recEventsHandler *handler.RecEventsHandler,
	internalHintHandler *handler.InternalHintHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(metricsCollector.Middleware)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Handle("/metrics", metricsCollector.Handler())

	r.Route("/api", func(r chi.Router) {
		// Same shapes the gateway already proxies — URL contract unchanged.
		r.Route("/users/recs", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/", recsHandler.GetRecs)
		})

		r.Route("/admin/recs", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Get("/{user_id}", adminRecsHandler.GetAdminRecs)
			r.Post("/{user_id}/recompute", adminRecsHandler.ForceRecompute)
		})

		r.Route("/events", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Post("/rec", recEventsHandler.PostRecEvent)
		})
	})

	// Docker-network-only producer endpoint — the gateway does NOT proxy
	// /internal/* (same rule as notifications' /internal/notifications).
	r.Post("/internal/recs/recompute-hint", internalHintHandler.PostRecomputeHint)

	return r
}
```
**Match the exact middleware/health/metrics construction of player's router** (`services/player/internal/transport/router.go` top of file) — if player uses different metrics wiring (e.g. `metricsCollector.Middleware` vs a function), mirror player exactly. Bring `OptionalAuthMiddleware`, `AuthMiddleware`, `AdminRoleMiddleware` over by copying their definitions from player's transport package into `services/recs/internal/transport/middleware.go` (do NOT git-mv them — player still needs its copies).

- [ ] **Step 3: Commit**

```bash
git add services/recs/internal/config services/recs/internal/transport
git commit -m "feat(recs): config + router for services/recs"
git push
```

### Task 5: Internal recompute-hint endpoint (the trigger seam, recs side)

**Files:**
- Create: `services/recs/internal/handler/internal_hint.go`
- Test: `services/recs/internal/handler/internal_hint_test.go`

This endpoint replaces BOTH in-process couplings that `ListService.MarkEpisodeWatched` had (`services/player/internal/service/list.go:404-450`): (a) the debounced `userOrchestrator.TriggerForUser`, and (b) the synchronous S6 seed update + cache bust on qualifying completion.

- [ ] **Step 1: Write the failing test** `internal_hint_test.go`:

```go
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeHintDeps records calls; handwritten fake per house style.
type fakeHintDeps struct {
	triggered   []string
	seedUpdates []string // "userID/animeID"
	cacheDels   []string
	listEntry   *hintListEntry // what lookupCompletion returns
}

func (f *fakeHintDeps) TriggerForUser(_ context.Context, userID string) error {
	f.triggered = append(f.triggered, userID)
	return nil
}
func (f *fakeHintDeps) LookupCompletion(_ context.Context, userID, animeID string) (*hintListEntry, error) {
	return f.listEntry, nil
}
func (f *fakeHintDeps) UpdateS6Seed(_ context.Context, userID, animeID string, _ time.Time, _ int) error {
	f.seedUpdates = append(f.seedUpdates, userID+"/"+animeID)
	return nil
}
func (f *fakeHintDeps) DeleteCache(_ context.Context, keys ...string) error {
	f.cacheDels = append(f.cacheDels, keys...)
	return nil
}

func postHint(t *testing.T, h *InternalHintHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/recs/recompute-hint", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.PostRecomputeHint(w, req)
	return w
}

func TestHint_TriggersDebounceAndSkipsSeedWhenNotQualifying(t *testing.T) {
	now := time.Now()
	f := &fakeHintDeps{listEntry: &hintListEntry{Status: "watching", Score: 0, CompletedAt: &now}}
	h := NewInternalHintHandler(f, testLogger(t))

	w := postHint(t, h, `{"user_id":"u1","anime_id":"a1"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(f.triggered) != 1 || f.triggered[0] != "u1" {
		t.Fatalf("triggered = %v, want [u1]", f.triggered)
	}
	if len(f.seedUpdates) != 0 {
		t.Fatalf("seedUpdates = %v, want empty (status != completed)", f.seedUpdates)
	}
}

func TestHint_QualifyingCompletionUpdatesSeedAndBustsCache(t *testing.T) {
	now := time.Now()
	f := &fakeHintDeps{listEntry: &hintListEntry{Status: "completed", Score: 8, CompletedAt: &now}}
	h := NewInternalHintHandler(f, testLogger(t))

	postHint(t, h, `{"user_id":"u1","anime_id":"a1"}`)

	if len(f.seedUpdates) != 1 || f.seedUpdates[0] != "u1/a1" {
		t.Fatalf("seedUpdates = %v, want [u1/a1]", f.seedUpdates)
	}
	if len(f.cacheDels) != 1 {
		t.Fatalf("cacheDels = %v, want one key", f.cacheDels)
	}
}

func TestHint_BadBodyIs400(t *testing.T) {
	f := &fakeHintDeps{}
	h := NewInternalHintHandler(f, testLogger(t))
	if w := postHint(t, h, `{`); w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if w := postHint(t, h, `{"anime_id":"a1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing user_id: status = %d, want 400", w.Code)
	}
}
```
(`testLogger(t)` — reuse the existing helper from the moved handler tests; grep `func testLogger` in `services/recs/internal/handler/`. If none exists, use `logger.Default()`.)

- [ ] **Step 2: Run to verify it fails**

```bash
cd services/recs && go test ./internal/handler/ -run TestHint -count=1
```
Expected: compile error — `InternalHintHandler` undefined.

- [ ] **Step 3: Implement `internal_hint.go`**

```go
// Package handler — internal_hint.go: POST /internal/recs/recompute-hint.
//
// Phase 1 of the recs extraction (spec 2026-06-11). Replaces the two
// in-process couplings player's ListService.MarkEpisodeWatched used to have:
//
//  1. Debounced user-signal recompute (was userOrchestrator.TriggerForUser).
//  2. S6 seed update + per-user cache bust on qualifying completion
//     (status='completed' AND score>=7 AND completed_at set) — was a
//     synchronous repo call inside the player request path.
//
// Docker-network-only: the gateway does not proxy /internal/*.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
)

// hintListEntry is the narrow anime_list projection the seed check needs.
type hintListEntry struct {
	Status      string
	Score       int
	CompletedAt *time.Time
}

// hintDeps is the narrow surface the handler depends on; production wires
// gormHintDeps (below), tests inject a fake.
type hintDeps interface {
	TriggerForUser(ctx context.Context, userID string) error
	LookupCompletion(ctx context.Context, userID, animeID string) (*hintListEntry, error)
	UpdateS6Seed(ctx context.Context, userID, animeID string, completedAt time.Time, score int) error
	DeleteCache(ctx context.Context, keys ...string) error
}

// InternalHintHandler serves POST /internal/recs/recompute-hint.
type InternalHintHandler struct {
	deps hintDeps
	log  *logger.Logger
}

func NewInternalHintHandler(deps hintDeps, log *logger.Logger) *InternalHintHandler {
	return &InternalHintHandler{deps: deps, log: log}
}

type hintBody struct {
	UserID  string `json:"user_id"`
	AnimeID string `json:"anime_id"`
}

func (h *InternalHintHandler) PostRecomputeHint(w http.ResponseWriter, r *http.Request) {
	var body hintBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.BadRequest(w, "invalid body")
		return
	}
	if body.UserID == "" {
		httputil.BadRequest(w, "user_id is required")
		return
	}
	ctx := r.Context()

	// 1. Debounced recompute — TriggerForUser owns the SetNX debounce and
	//    always returns nil (best-effort contract carried over from Phase 11).
	_ = h.deps.TriggerForUser(ctx, body.UserID)

	// 2. S6 seed update on qualifying completion. Mirrors the exact gate the
	//    player-side code used (list.go Phase 13): completed + score>=7 +
	//    completed_at set. anime_id may be empty for generic hints — skip.
	if body.AnimeID != "" {
		entry, err := h.deps.LookupCompletion(ctx, body.UserID, body.AnimeID)
		if err != nil {
			h.log.Warnw("hint completion lookup failed (non-fatal)",
				"user_id", body.UserID, "anime_id", body.AnimeID, "error", err)
		} else if entry != nil && entry.Status == "completed" && entry.Score >= 7 && entry.CompletedAt != nil {
			if err := h.deps.UpdateS6Seed(ctx, body.UserID, body.AnimeID, *entry.CompletedAt, entry.Score); err != nil {
				h.log.Errorw("hint s6 seed update failed (non-fatal)",
					"user_id", body.UserID, "anime_id", body.AnimeID, "error", err)
			} else if err := h.deps.DeleteCache(ctx, recs.UserTopNKey(recs.UserID(body.UserID))); err != nil {
				h.log.Warnw("hint cache bust failed (non-fatal)", "user_id", body.UserID, "error", err)
			}
		}
	}

	httputil.OK(w, map[string]bool{"ok": true})
}
```

- [ ] **Step 4: Implement the production `gormHintDeps`** in the same file (below the handler):

```go
// gormHintDeps is the production hintDeps implementation: GORM reads of the
// shared anime_list table + the recs repo + Redis cache + user orchestrator.
type gormHintDeps struct {
	db       *gorm.DB
	repo     *repo.RecsRepository
	cache    hintCache
	userOrch *recs.UserOrchestrator
}

type hintCache interface {
	Delete(ctx context.Context, keys ...string) error
}

func NewGormHintDeps(db *gorm.DB, recsRepo *repo.RecsRepository, cache hintCache, userOrch *recs.UserOrchestrator) hintDeps {
	return &gormHintDeps{db: db, repo: recsRepo, cache: cache, userOrch: userOrch}
}

func (g *gormHintDeps) TriggerForUser(ctx context.Context, userID string) error {
	return g.userOrch.TriggerForUser(ctx, recs.UserID(userID))
}

func (g *gormHintDeps) LookupCompletion(ctx context.Context, userID, animeID string) (*hintListEntry, error) {
	var row hintListEntry
	res := g.db.WithContext(ctx).
		Table("anime_list").
		Select("status, score, completed_at").
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		Limit(1).
		Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

func (g *gormHintDeps) UpdateS6Seed(ctx context.Context, userID, animeID string, completedAt time.Time, score int) error {
	return g.repo.UpdateS6Seed(ctx, userID, animeID, completedAt, score)
}

func (g *gormHintDeps) DeleteCache(ctx context.Context, keys ...string) error {
	return g.cache.Delete(ctx, keys...)
}
```
Add the needed imports (`gorm.io/gorm`, `services/recs/internal/repo`). Check `recs.UserID` is a string alias (`services/recs/internal/service/recs/types.go`) — adjust the conversion if it's a defined type (it is: `recs.UserID(userID)`).

- [ ] **Step 5: Run the tests**

```bash
cd services/recs && go test ./internal/handler/ -run TestHint -count=1 -race
```
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add services/recs/internal/handler/internal_hint.go services/recs/internal/handler/internal_hint_test.go
git commit -m "feat(recs): /internal/recs/recompute-hint endpoint (trigger seam)"
git push
```

### Task 6: recs main.go + green build

**Files:**
- Create: `services/recs/cmd/recs-api/main.go`

- [ ] **Step 1: Write main.go.** Boilerplate (logger/config/tracing/db/gormtrace/db-pool-collector/redis/server/graceful-shutdown) is copied from `services/notifications/cmd/notifications-api/main.go`; the recs wiring is ported from `services/player/cmd/player-api/main.go` lines ~300–360 (orchestrators) and ~471–505 (handlers). Resulting wiring section:

```go
	// Service-owned tables ONLY — anime_list / watch_history / animes stay
	// owned by player + catalog; recs reads them without migrating them.
	if err := db.AutoMigrate(
		&domain.RecUserSignals{},
		&domain.RecPopulationSignals{},
		&domain.RecCompletionCoOccurrence{},
		&domain.RecEvent{},
	); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "error", err)
	}
	defer redisCache.Close()

	cronCtx, cronCancel := context.WithCancel(context.Background())
	defer cronCancel()

	recsRepo := repo.NewRecsRepository(db.DB)

	// Population signals (S3 trending / S4 recency) — 60-minute cron.
	s3 := signals.NewS3Trending(db.DB, recsRepo)
	s4 := signals.NewS4Recency(db.DB)
	popOrch := recs.NewPopulationOrchestrator([]recs.SignalModule{s3, s4}, db.DB, recsRepo, log)
	popOrch.Start(tracing.SeedBaggage(cronCtx, "scheduled_job:recs-population", ""), 60*time.Minute)

	// User signals (S1 k-NN / S5 TF-IDF) — 6h cron + hint-driven debounce.
	s1 := signals.NewS1ScoreCluster(db.DB, recsRepo)
	s2 := signals.NewS2Metadata(db.DB)
	s5 := signals.NewS5Attribute(db.DB, recsRepo)
	userPrecompute := recs.NewOrchestrator([]recs.SignalModule{s1, s2, s5})
	userOrch := recs.NewUserOrchestrator(userPrecompute, db.DB, redisCache, log)
	userOrch.Start(tracing.SeedBaggage(cronCtx, "scheduled_job:recs-user-precompute", ""), 6*time.Hour)

	// S6 co-occurrence materializer — nightly.
	coOccOrch := recs.NewCoOccurrenceOrchestrator(db.DB, log)
	coOccOrch.Start(tracing.SeedBaggage(cronCtx, "scheduled_job:recs-cooccurrence", ""), 24*time.Hour)

	// Handlers.
	shikimoriSimilarClient := signals.NewHTTPShikimoriSimilarClient(cfg.CatalogURL, log)
	s6 := signals.NewS6ComboPin(db.DB, recsRepo, shikimoriSimilarClient, log)
	recsHandler := handler.NewRecsHandler(db.DB, recsRepo, redisCache, s6, log)
	adminRecsHandler := handler.NewAdminRecsHandler(db.DB, recsRepo, redisCache, s6, userPrecompute, log)
	recEventsRepo := repo.NewRecEventsRepository(db.DB)
	recEventsHandler := handler.NewRecEventsHandler(recEventsRepo, log)
	hintDeps := handler.NewGormHintDeps(db.DB, recsRepo, redisCache, userOrch)
	internalHintHandler := handler.NewInternalHintHandler(hintDeps, log)

	metricsCollector := metrics.NewCollector("recs")
	router := transport.NewRouter(recsHandler, adminRecsHandler, recEventsHandler, internalHintHandler, cfg.JWT, log, metricsCollector)
```
**Anchor everything on the player original:** the exact constructor signatures (`NewPopulationOrchestrator`, `NewS4Recency`, `metrics.NewCollector`, ENV-gating like `if cfg.X` around orchestrators if player has it) MUST be copied from `services/player/cmd/player-api/main.go` — read those line ranges first and preserve any conditional gating (e.g. `RECS_*_ENABLED` flags) verbatim. The shutdown sequence: `cronCancel()` before `srv.Shutdown` (player does this — mirror it).

- [ ] **Step 2: Build until green**

```bash
cd services/recs && go mod tidy && go build ./... && go vet ./...
```
Expected: clean. Iterate on missing imports/types — every error should be resolvable by copying the corresponding wiring detail from player's main.go.

- [ ] **Step 3: Run ALL moved tests**

```bash
cd services/recs && go test ./... -count=1 -race 2>&1 | tail -20
```
Expected: PASS everywhere. Moved tests that fail because of import-path assumptions get fixed (mechanical), NOT skipped.

- [ ] **Step 4: Commit**

```bash
git add services/recs go.work.sum
git commit -m "feat(recs): service entrypoint — orchestrators + handlers wired, green build"
git push
```

### Task 7: Player side — hint producer replaces in-process recs coupling

**Files:**
- Create: `services/player/internal/service/recs_hint.go`
- Test: `services/player/internal/service/recs_hint_test.go`
- Modify: `services/player/internal/service/list.go` (struct, constructor, `MarkEpisodeWatched` lines ~404–450)
- Modify: `services/player/cmd/player-api/main.go` (remove recs wiring, add producer)
- Modify: `services/player/internal/transport/router.go` (remove 3 recs route groups + handler params)

- [ ] **Step 1: Write the failing producer test** `recs_hint_test.go` (httptest server records POSTs; drop-on-full check):

```go
package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestRecsHintProducer_PostsHint(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		got = append(got, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewRecsHintProducer(srv.URL, true, logger.Default())
	p.Start()
	p.Hint("u1", "a1")
	p.Stop() // Stop drains the channel before returning

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0]["user_id"] != "u1" || got[0]["anime_id"] != "a1" {
		t.Fatalf("got %v, want one hint u1/a1", got)
	}
}

func TestRecsHintProducer_NilAndDisabledAreNoops(t *testing.T) {
	var p *RecsHintProducer
	p.Hint("u1", "a1") // nil receiver must not panic
	p2 := NewRecsHintProducer("http://recs:8094", false, logger.Default())
	p2.Start()
	p2.Hint("u1", "a1")
	p2.Stop()
	// no assertion — absence of panic/network is the contract
	_ = time.Now()
}
```

- [ ] **Step 2: Run to verify failure**

```bash
cd services/player && go test ./internal/service/ -run TestRecsHintProducer -count=1
```
Expected: compile error — `NewRecsHintProducer` undefined.

- [ ] **Step 3: Implement `recs_hint.go`** — clone the `GachaCreditProducer` shape (`services/player/internal/service/gacha_credit.go`): buffered channel cap 256, single worker goroutine, drop-on-full WARN, 3s client timeout, nil-receiver-safe, Start/Stop with WaitGroup:

```go
package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// recsHintMsg is the work item queued on the producer's channel.
type recsHintMsg struct {
	UserID  string `json:"user_id"`
	AnimeID string `json:"anime_id"`
}

// RecsHintProducer is a fire-and-forget producer that POSTs watch-activity
// hints to the recs service's /internal/recs/recompute-hint endpoint.
// Extraction Phase 1 (spec 2026-06-11): replaces the in-process
// userOrchestrator.TriggerForUser + synchronous S6 seed update that lived in
// ListService.MarkEpisodeWatched before the recs engine moved out of player.
//
// Contract (mirrors GachaCreditProducer):
//   - Buffered channel (cap 256) + single worker goroutine.
//   - Channel full or recs outage => event DROPPED with WARN (drop-on-full).
//     Worst case the 6h recs cron refreshes the user instead.
//   - 3-second HTTP timeout; no retries.
//   - Nil-receiver safe; all methods no-op when p == nil or !p.enabled.
type RecsHintProducer struct {
	url     string
	ch      chan recsHintMsg
	client  *http.Client
	log     *logger.Logger
	wg      sync.WaitGroup
	enabled bool
}

func NewRecsHintProducer(url string, enabled bool, log *logger.Logger) *RecsHintProducer {
	return &RecsHintProducer{
		url:     url,
		ch:      make(chan recsHintMsg, 256),
		client:  &http.Client{Timeout: 3 * time.Second},
		log:     log,
		enabled: enabled,
	}
}

func (p *RecsHintProducer) Start() {
	if p == nil || !p.enabled {
		return
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for msg := range p.ch {
			p.send(msg)
		}
	}()
}

func (p *RecsHintProducer) Stop() {
	if p == nil || !p.enabled {
		return
	}
	close(p.ch)
	p.wg.Wait()
}

// Hint enqueues a recompute hint. Non-blocking: a full channel drops the
// hint with a WARN — a recs outage must never slow MarkEpisodeWatched.
func (p *RecsHintProducer) Hint(userID, animeID string) {
	if p == nil || !p.enabled {
		return
	}
	select {
	case p.ch <- recsHintMsg{UserID: userID, AnimeID: animeID}:
	default:
		p.log.Warnw("recs hint channel full; dropping hint", "user_id", userID, "anime_id", animeID)
	}
}

func (p *RecsHintProducer) send(msg recsHintMsg) {
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	resp, err := p.client.Post(fmt.Sprintf("%s/internal/recs/recompute-hint", p.url), "application/json", bytes.NewReader(body))
	if err != nil {
		p.log.Warnw("recs hint post failed (dropped)", "user_id", msg.UserID, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.log.Warnw("recs hint rejected (dropped)", "user_id", msg.UserID, "status", resp.StatusCode)
	}
}
```
**Verify against `gacha_credit.go`'s actual Start/Stop/worker internals and mirror any divergence** (e.g. if Stop guards double-close, copy that).

- [ ] **Step 4: Run producer tests** — expect PASS.

- [ ] **Step 5: Rewire `list.go`.**
  1. Delete the `recsRepoForListService` interface (lines ~15–25), the `userOrchestrator`/`recsRepo` struct fields, and the `cache` field IF its only use is the recs bust — verify with `grep -n "s.cache" services/player/internal/service/list.go`; if other usages exist, keep the field and only delete the recs-related call.
  2. Constructor: `NewListService(listRepo, activityRepo, prefRepo, progressRepo, recsHint *RecsHintProducer, gachaProducer *GachaCreditProducer, log *logger.Logger)` — userOrch/recsRepo/cache params removed, `recsHint` added. Update the struct accordingly.
  3. Replace the two blocks at lines ~404–450 (the `if s.userOrchestrator != nil` goroutine AND the entire `if s.recsRepo != nil` S6-seed block) with:

```go
		// Recs extraction Phase 1 (spec 2026-06-11): one fire-and-forget hint
		// replaces both the in-process debounce trigger and the synchronous S6
		// seed update — the recs service derives the seed from anime_list on
		// receipt. Drop-on-full; never blocks or fails this request.
		s.recsHint.Hint(userID, animeID)
```
  Place it where the old `userOrchestrator` block was (inside the same watch-history branch). Remove the now-unused `recs` import.
  4. **Behavior note for the executor:** the old S6 path was synchronous; the hint is async (~ms over Docker network). The byte-identical gate (Task 11) compares ranked output, not seed-update latency — this is an accepted, spec'd deviation.

- [ ] **Step 6: Clean `services/player/cmd/player-api/main.go`.** Remove: the two recs imports (lines ~22–23), the `&domain.Rec*{}` AutoMigrate entries (4 lines, ~92–95), the popOrch/userOrch/coOccOrch blocks (~lines 300–360), the recsRepo hoist + recs handler constructions (~406, 471–505 recs parts), the recs/admin-recs/rec-events args from `transport.NewRouter(...)`, and the shutdown comment block for the recs cron. Add the producer next to the gacha producer (~line 426):

```go
	recsHintProducer := service.NewRecsHintProducer(
		getEnvDefault("RECS_INTERNAL_URL", "http://recs:8094"),
		getEnvBool("RECS_HINT_ENABLED", true),
		log,
	)
	recsHintProducer.Start()
	defer recsHintProducer.Stop()
```
(Use whatever env-helper functions player's main/config already has — grep `getEnv` in `services/player/internal/config/config.go` and follow the local pattern; if config centralizes env there, add `RecsInternalURL` to player's config struct instead of reading env in main. Mirror how `GACHA_*` URLs are loaded.) Update the `NewListService` call to the new signature. **Check `redisCache` usages after removal** — if recs was the only consumer (`grep -n redisCache services/player/cmd/player-api/main.go`), remove `cache.New` + import as well; if anything else uses it, keep it.

- [ ] **Step 7: Clean `services/player/internal/transport/router.go`.** Remove the three route groups (`/users/recs` at ~174, `/admin/recs` at ~183, `/events` at ~209) and the `recsHandler`/`adminRecsHandler`/`recEventsHandler` params from `NewRouter`. Player keeps `OptionalAuthMiddleware` etc. (other routes use them).

- [ ] **Step 8: Fix player tests that referenced recs.** Expected casualties: `list_test.go`, `list_mark_completed_test.go`, anything constructing `NewListService` with the old signature. Update constructor calls (pass `nil` for `recsHint` — nil-receiver-safe by design). Then:

```bash
cd services/player && go build ./... && go vet ./... && go test ./... -count=1 -race 2>&1 | tail -15
```
Expected: PASS; zero references to `service/recs` remain (`grep -rn "service/recs\|RecsHandler\|AdminRecsHandler\|RecEventsHandler" services/player/ --include='*.go'` returns nothing).

- [ ] **Step 9: Commit**

```bash
git add services/player
git commit -m "refactor(player): recs engine removed — fire-and-forget hint producer to recs:8094"
git push
```

### Task 8: Gateway re-point

**Files:**
- Modify: `services/gateway/internal/config/config.go` (RecsService URL)
- Modify: `services/gateway/internal/service/proxy.go` (getServiceURL case)
- Modify: `services/gateway/internal/handler/proxy.go` (ProxyToRecs)
- Modify: `services/gateway/internal/transport/router.go` (4 route lines, ~364–385)

- [ ] **Step 1: Config** — in `config.go`, add below `PlayerService string` (line ~57): `RecsService string` with a doc comment `// RecsService — recs engine extraction (spec 2026-06-11). Port 8094.`; in the loader (line ~126 area): `RecsService: getEnv("RECS_SERVICE_URL", "http://recs:8094"),`.

- [ ] **Step 2: Proxy service** — in `services/gateway/internal/service/proxy.go` `getServiceURL` switch (~line 233), add:

```go
	case "recs":
		return s.serviceURLs.RecsService, nil
```

- [ ] **Step 3: Proxy handler** — in `services/gateway/internal/handler/proxy.go` next to `ProxyToPlayer` (line 37):

```go
// ProxyToRecs proxies requests to the recs service (extraction Phase 1).
func (h *ProxyHandler) ProxyToRecs(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "recs")
}
```

- [ ] **Step 4: Router** — in `services/gateway/internal/transport/router.go`, change exactly these handlers from `proxyHandler.ProxyToPlayer` to `proxyHandler.ProxyToRecs` (middleware stacks and route ORDER stay untouched — the `/users/recs` group MUST remain registered before the protected `/users/*` group, the existing comments stay):
  - `r.HandleFunc("/users/recs", ...)` (~line 364)
  - `r.HandleFunc("/users/recs/", ...)` (~line 365)
  - `r.HandleFunc("/admin/recs/*", ...)` (~line 376)
  - `r.HandleFunc("/events/rec", ...)` (~line 385)
  Update the three group comments to say the routes now proxy to the recs service.

- [ ] **Step 5: Build + test**

```bash
cd services/gateway && go build ./... && go test ./... -count=1 2>&1 | tail -5
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/gateway
git commit -m "feat(gateway): re-point /users/recs, /admin/recs, /events/rec to recs:8094"
git push
```

### Task 9: Compose + Prometheus + Makefile check

**Files:**
- Modify: `docker/docker-compose.yml` (new `recs` service; player + gateway env)
- Modify: `docker/prometheus/prometheus.yml` (scrape job)

- [ ] **Step 1: Add the recs service block** (next to `notifications:` at line ~768; mirror its depends_on/healthcheck shape exactly — read the full notifications block first including the lines below `depends_on:`):

```yaml
  recs:
    build:
      context: ..
      dockerfile: services/recs/Dockerfile
    container_name: animeenigma-recs
    restart: unless-stopped
    environment:
      SERVER_PORT: 8094
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: postgres
      DB_PASSWORD: postgres
      DB_NAME: animeenigma
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
      REDIS_HOST: redis
      # S6 combo-pin Shikimori /similar fallback.
      CATALOG_URL: http://catalog:8081
      TRACING_ENABLED: "true"
    ports:
      - "127.0.0.1:8094:8094"
    depends_on:
      # copy the exact depends_on/condition block from the notifications service
```

- [ ] **Step 2: Player env** — in the player service block (`environment:` near line ~661 where `NOTIFICATIONS_INTERNAL_URL` lives), add:

```yaml
      # Recs extraction Phase 1 — fire-and-forget recompute hints.
      RECS_INTERNAL_URL: http://recs:8094
```

- [ ] **Step 3: Gateway env** — in the gateway block (next to `GACHA_SERVICE_URL` line ~475): `RECS_SERVICE_URL: http://recs:8094`.

- [ ] **Step 4: Prometheus** — in `docker/prometheus/prometheus.yml` after the `analytics` job (line ~68):

```yaml
  - job_name: 'recs'
    static_configs:
      - targets: ['recs:8094']
```
Also check Grafana panel job pinning: `grep -rn 'job="player"' infra/grafana/dashboards/ docker/grafana* 2>/dev/null | grep -i rec` — any rec-metric panel pinned to `job="player"` gets updated to `job="recs"` (or the job matcher dropped).

- [ ] **Step 5: Makefile** — no change needed (`redeploy-%` pattern target at Makefile:284 covers `make redeploy-recs`). Verify: `make -n redeploy-recs` prints `./deploy/scripts/redeploy.sh recs`.

- [ ] **Step 6: Commit**

```bash
git add docker/docker-compose.yml docker/prometheus/prometheus.yml infra/grafana 2>/dev/null
git commit -m "feat(recs): compose service, prometheus scrape, env wiring"
git push
```

### Task 10: Docs

**Files:**
- Modify: `CLAUDE.md` (Service Ports table, Gateway Routing list, env-vars section)

- [ ] **Step 1: CLAUDE.md** — add `| recs | 8094 | /metrics | Recommendation engine (extracted from player, spec 2026-06-11) |` to the Service Ports table; add to Gateway Routing: `- /api/users/recs, /api/events/rec → recs:8094 (optional JWT); /api/admin/recs/* → recs:8094 (admin)`; add a recs env block (`CATALOG_URL`, and player's `RECS_INTERNAL_URL`) near the notifications env section.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: recs service (8094) in ports/routing/env reference"
git push
```

### Task 11: Deploy + byte-identical gate + phase close

- [ ] **Step 1: Deploy the three changed services** (recs first so the hint endpoint exists before player starts emitting):

```bash
make redeploy-recs && make redeploy-player && make redeploy-gateway
make health
```
Expected: all green, including the new recs service. Watch boot: `make logs-recs` should show the population-orchestrator boot tick completing.

- [ ] **Step 2: Byte-identical diff** (same protocol as Task 1 — flush keys, fetch fresh, normalize, diff):

```bash
UI_KEY=$(grep '^UI_AUDIT_API_KEY=' docker/.env | cut -d= -f2)
UID=$(docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -tAc "SELECT id FROM users WHERE username='ui_audit_bot';")
docker compose -f docker/docker-compose.yml exec -T redis redis-cli DEL "recs:public:trending:topN" "recs:user:${UID}:topN:v2"
curl -s http://localhost:8000/api/users/recs | jq 'del(.data.generated_at, .data.cache_hit)' > /tmp/recs-after-anon.json
curl -s http://localhost:8000/api/users/recs -H "Authorization: Bearer $UI_KEY" | jq 'del(.data.generated_at, .data.cache_hit)' > /tmp/recs-after-user.json
diff /tmp/recs-baseline-anon.json /tmp/recs-after-anon.json && diff /tmp/recs-baseline-user.json /tmp/recs-after-user.json && echo "BYTE-IDENTICAL ✓"
```
Expected: `BYTE-IDENTICAL ✓`. **Caveat:** S3 trending counts use a 30-day window — if hours passed between Task 1 and now, tiny count drift can reorder ties. If the diff shows only adjacent-rank swaps with near-equal `final`, re-capture the baseline from a player-image rollback is NOT warranted; instead verify per-signal equality via the admin endpoint or accept after manual inspection of the final-score deltas (must be < 0.001).

- [ ] **Step 3: Trigger-seam smoke** — as ui_audit_bot, mark an episode watched, then confirm the hint arrived:

```bash
curl -s -X POST http://localhost:8000/api/users/progress/watched -H "Authorization: Bearer $UI_KEY" -H "Content-Type: application/json" -d '{"anime_id":"<any anime id from the baseline json>","episode":1,"player":"kodik","language":"ru","watch_type":"sub"}' | head -c 200
make logs-recs 2>&1 | grep -i "debounce\|trigger" | tail -3
```
(Adjust the progress endpoint path/body to whatever `userApi.markEpisodeWatched` in `frontend/web/src/api/client.ts:429` actually calls — read that line first.) Expected: player 200s; recs log shows the debounced trigger firing.

- [ ] **Step 4: Telemetry smoke** — `curl -s -X POST http://localhost:8000/api/events/rec -H "Content-Type: application/json" -d '{"event_type":"rec_click","anime_id":"x","signal_id":"s3","pinned":false}'` → `{"ok":true}` and `curl -s http://localhost:8094/metrics | grep rec_click_total` shows the counter on the NEW service.

- [ ] **Step 5: Run `/animeenigma-after-update`** for Phase 1 (it handles lint, changelog entry, final commit, push). Note for the changelog: extraction is invisible to users — entry can be a short "engine moved to its own service, faster and safer" item.

---

# PHASE 2 — ISS-026: rec_watched instrumentation (Tasks 12–13)

Reality check from recon: `emitRecWatched` + `findRecentClick` already exist (`frontend/web/src/utils/recsAnalytics.ts`) and are wired in `KodikPlayer.vue` + `AnimeLibPlayer.vue` **auto-mark only**. Gaps: (a) HanimePlayer / Anime18Player / KodikAdFreePlayer never emit; (b) the MANUAL mark-watched paths never emit in any player; (c) 1h TTL is too narrow for real watch behavior. Fix all three with one shared helper.

### Task 12: Shared helper + TTL + wire all five players

**Files:**
- Modify: `frontend/web/src/utils/recsAnalytics.ts`
- Test: `frontend/web/src/utils/__tests__/recsAnalytics.spec.ts` (create if missing; check for an existing spec first)
- Modify: `frontend/web/src/components/player/{KodikPlayer,AnimeLibPlayer,HanimePlayer,Anime18Player,KodikAdFreePlayer}.vue`

- [ ] **Step 1: Write failing Vitest** for the new helper:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({ apiClient: { post: vi.fn().mockResolvedValue({}) } }))
import { apiClient } from '@/api/client'
import { emitRecWatchedIfRecent } from '../recsAnalytics'

describe('emitRecWatchedIfRecent', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('emits rec_watched and removes the click (fire-once)', async () => {
    localStorage.setItem('recentRecClicks', JSON.stringify([
      { anime_id: 'a1', signal_id: 's1', pinned: false, timestamp: Date.now() },
    ]))
    await emitRecWatchedIfRecent('a1', 'player')
    expect(apiClient.post).toHaveBeenCalledWith('/events/rec', expect.objectContaining({
      event_type: 'rec_watched', anime_id: 'a1', signal_id: 's1', source_route: 'player',
    }))
    // fire-once: second call must NOT emit again
    await emitRecWatchedIfRecent('a1', 'player')
    expect(apiClient.post).toHaveBeenCalledTimes(1)
  })

  it('does nothing without a recent click', async () => {
    await emitRecWatchedIfRecent('a2', 'player')
    expect(apiClient.post).not.toHaveBeenCalled()
  })

  it('honors the 7-day window', async () => {
    localStorage.setItem('recentRecClicks', JSON.stringify([
      { anime_id: 'a1', signal_id: 's1', pinned: false, timestamp: Date.now() - 8 * 24 * 3600 * 1000 },
    ]))
    await emitRecWatchedIfRecent('a1', 'player')
    expect(apiClient.post).not.toHaveBeenCalled()
  })
})
```

Run: `cd frontend/web && bunx vitest run src/utils/__tests__/recsAnalytics.spec.ts` → FAIL (export missing).

- [ ] **Step 2: Implement in `recsAnalytics.ts`:**
  - Change `const TTL_MS = 60 * 60 * 1000 // 1 hour` → `const TTL_MS = 7 * 24 * 60 * 60 * 1000 // 7 days — ISS-026: 1h missed most real watch sessions` .
  - Add:

```ts
/**
 * removeClick deletes all stored clicks for an anime — called after a
 * successful rec_watched emit so each click converts at most once (ISS-026).
 */
function removeClick(animeId: string): void {
  writeStore(readStore().filter((c) => c.anime_id !== animeId))
}

/**
 * emitRecWatchedIfRecent is the one call players make on mark-watched
 * (auto or manual): looks up the most recent rec click for this anime
 * within the TTL window, emits rec_watched with the originating signal_id,
 * and removes the click (fire-once). No click → no-op. ISS-026.
 */
export async function emitRecWatchedIfRecent(animeId: string, sourceRoute: string): Promise<void> {
  const recent = findRecentClick(animeId)
  if (!recent) return
  removeClick(animeId)
  await emitRecWatched({
    event_type: 'rec_watched',
    anime_id: animeId,
    signal_id: recent.signal_id,
    pinned: recent.pinned,
    pin_source: recent.pin_source,
    pin_seed_anime_id: recent.pin_seed_anime_id,
    source_route: sourceRoute,
    rank: recent.rank,
  })
}
```

  Run the spec → PASS.

- [ ] **Step 3: Wire the players.** In each of the five components, find EVERY `userApi.markEpisodeWatched(...)` success path (both the manual `markEpisodeWatched`-style function and the `autoMarkEpisodeWatched` function — grep each file). After the success of each, add:

```ts
void emitRecWatchedIfRecent(props.animeId, 'player')
```
with the import `import { emitRecWatchedIfRecent } from '@/utils/recsAnalytics'`. In **KodikPlayer.vue** (~line 803) and **AnimeLibPlayer.vue** (~line 764), REPLACE the existing inline `findRecentClick` + `emitRecWatched` blocks with the same one-liner and drop the now-unused imports. Note `props.animeId` naming may differ per component (check each — some use `props.animeId`, others a local `animeId`); also Anime18Player/HanimePlayer may gate marking behind auth — add the emit inside the same guarded block where `markEpisodeWatched` succeeds.

- [ ] **Step 4: Frontend gates**

```bash
cd frontend/web && bunx vitest run src/utils/__tests__/recsAnalytics.spec.ts && bunx tsc --noEmit && bunx eslint src/utils/recsAnalytics.ts src/components/player/KodikPlayer.vue src/components/player/AnimeLibPlayer.vue src/components/player/HanimePlayer.vue src/components/player/Anime18Player.vue src/components/player/KodikAdFreePlayer.vue
```
Expected: all clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/recsAnalytics.ts frontend/web/src/utils/__tests__/recsAnalytics.spec.ts frontend/web/src/components/player/
git commit -m "fix(web): emit rec_watched on mark-watched in all players, 7d window, fire-once (ISS-026)"
git push
```

### Task 13: Deploy + verify + close ISS-026

- [ ] **Step 1: Deploy web SAFELY.** `make redeploy-web` ships the shared tree's uncommitted state (memory hazard: shipped a TDZ crash 2026-06-10). Deploy from a clean HEAD worktree instead:

```bash
git worktree add /tmp/wt-web-deploy HEAD
cp docker/.env /tmp/wt-web-deploy/docker/.env
cd /tmp/wt-web-deploy && make redeploy-web; cd /data/animeenigma
git worktree remove /tmp/wt-web-deploy --force
```
(If `make redeploy-web` in the worktree complains about compose project naming, run the underlying script with the `docker` compose project as memory `project_trunk_divergence` describes.)

- [ ] **Step 2: Browser smoke (DS-NF-06)** — open the site, home page: click a rec-row card, play an episode to the auto-mark threshold (or use the manual "mark watched" button), then:

```bash
curl -s http://localhost:8094/metrics | grep rec_watched_total
```
Expected: a non-zero `rec_watched_total{signal_id=...}` series exists. Check the 3 Grafana panels (per-signal CTR / watch-rate / pin CTR) now render data points. Also check browser console for errors on /anime + home (post-deploy smoke per memory).

- [ ] **Step 3: Close ISS-026** in `docs/issues/README.md` (move to Resolved with date 2026-06-11 + fix summary) and update `docs/issues/issues.json` if it tracks the same IDs (inspect its schema first — other agents touch this file; path-scope the commit).

- [ ] **Step 4: Run `/animeenigma-after-update`** for Phase 2.

---

# PHASE 3 — S7 dropped-penalty signal (Tasks 14–15)

### Task 14: S7 signal (TDD)

**Files:**
- Create: `services/recs/internal/service/recs/signals/s7_dropped_penalty.go`
- Test: `services/recs/internal/service/recs/signals/s7_dropped_penalty_test.go`

S7 mirrors S2's stateless request-time pattern (`s2_metadata.go`), inverted: seeds are DROPPED anime, and similarity is Jaccard over the namespaced union of genre IDs + tag IDs.

- [ ] **Step 1: Write the failing test.** Look at `s2_metadata_test.go` first and reuse its DB-fake pattern (it's the closest analog — if it uses a sqlite in-memory GORM handle, do the same; player's go.mod carried `gorm.io/driver/sqlite`, recs inherits via tidy):

```go
package signals

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
)

// Seed helper: insert anime_list rows + anime_genres/anime_tags rows into the
// test DB the same way s2_metadata_test.go does. The cases below assume:
//   user "u1" dropped anime "d1" (genres g1,g2; no score) and "d2" (genre g1, tag t1; score 4)
//   user "u1" dropped anime "d3" with score 8 (LIKED drop — must be excluded)
//   candidates: "c1" (genres g1,g2 — high overlap), "c2" (genre g9 — no overlap)

func TestS7_ScoresSimilarityToDroppedSeeds(t *testing.T) {
	db := newTestDB(t) // same helper s2_metadata_test.go uses; copy it if file-local
	seedS7Fixtures(t, db)
	s7 := NewS7DroppedPenalty(db)

	got, err := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"c1", "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if got["c1"] == 0 {
		t.Fatalf("c1 overlaps dropped seeds; want > 0, got %v", got["c1"])
	}
	if _, ok := got["c2"]; ok {
		t.Fatalf("c2 has no overlap; must be omitted, got %v", got["c2"])
	}
}

func TestS7_ColdStartUnderTwoSeeds(t *testing.T) {
	db := newTestDB(t)
	// only ONE eligible dropped seed
	seedOneDrop(t, db)
	s7 := NewS7DroppedPenalty(db)
	got, err := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"c1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cold-start (<2 seeds) must return empty map, got %v", got)
	}
}

func TestS7_LikedDropsExcluded(t *testing.T) {
	db := newTestDB(t)
	seedS7Fixtures(t, db) // includes d3 score=8
	s7 := NewS7DroppedPenalty(db)
	// candidate c3 overlaps ONLY d3's attributes — must score zero/omitted
	got, _ := s7.Score(context.Background(), recs.UserID("u1"), []recs.AnimeID{"c3"})
	if _, ok := got["c3"]; ok {
		t.Fatalf("c3 only matches a liked drop (score>=7); must be omitted, got %v", got["c3"])
	}
}

func TestS7_IDAndPrecompute(t *testing.T) {
	s7 := NewS7DroppedPenalty(nil)
	if s7.ID() != recs.SignalID("s7") {
		t.Fatalf("ID = %q, want s7", s7.ID())
	}
	if err := s7.Precompute(context.Background(), "u1"); err != nil {
		t.Fatalf("Precompute must be a no-op, got %v", err)
	}
}
```
Write `seedS7Fixtures`/`seedOneDrop` against the same table-seeding style the S2 test uses (raw `db.Exec` inserts into `anime_list`, `anime_genres`, `anime_tags`).

- [ ] **Step 2: Run to verify failure** — `cd services/recs && go test ./internal/service/recs/signals/ -run TestS7 -count=1` → compile error.

- [ ] **Step 3: Implement `s7_dropped_penalty.go`:**

```go
package signals

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S7DroppedPenalty is the negative "more like what you dropped" signal
// (spec 2026-06-11 Phase 3). It mirrors S2's stateless request-time pattern,
// inverted: seeds are the user's DROPPED anime, similarity is max Jaccard
// over the namespaced union of genre IDs + tag IDs, and the ENSEMBLE applies
// it with a negative weight (-0.15) so high similarity demotes a candidate.
//
// Per the SignalModule contract the signal itself still returns POSITIVE
// raw scores in [0,1] — the minus sign lives in the weight, never here.
//
// Guards (dropping is a noisy signal — demote, never bury):
//   - Drops the user scored >= 7 are excluded ("liked but life happened").
//   - Fewer than 2 eligible seeds => silent (empty map, cold-start).
type S7DroppedPenalty struct {
	db *gorm.DB
}

const (
	// s7LikedDropThreshold: dropped rows with score >= 7 are NOT dislike
	// evidence and are excluded from the seed set.
	s7LikedDropThreshold = 7
	// s7MinSeeds: below this many eligible dropped seeds the signal stays
	// silent — one drop is mood, two is a pattern.
	s7MinSeeds = 2
)

func NewS7DroppedPenalty(db *gorm.DB) *S7DroppedPenalty {
	return &S7DroppedPenalty{db: db}
}

// ID returns the stable signal identifier "s7".
func (s *S7DroppedPenalty) ID() recs.SignalID { return recs.SignalID("s7") }

// Precompute is a no-op — S7 is request-time only, like S2.
func (s *S7DroppedPenalty) Precompute(_ context.Context, _ recs.UserID) error { return nil }

// Score returns max-Jaccard similarity between each candidate's genre+tag
// set and the user's dropped seeds. Candidates with no overlap are omitted.
func (s *S7DroppedPenalty) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}

	var seeds []string
	if err := s.db.WithContext(ctx).
		Table("anime_list").
		Select("anime_id").
		Where("user_id = ? AND status = ? AND score < ?", userID, "dropped", s7LikedDropThreshold).
		Pluck("anime_id", &seeds).Error; err != nil {
		return nil, fmt.Errorf("s7: load dropped seeds: %w", err)
	}
	if len(seeds) < s7MinSeeds {
		return out, nil
	}

	seedAttrs, err := s.loadAttrSets(ctx, seeds)
	if err != nil {
		return nil, fmt.Errorf("s7: load seed attrs: %w", err)
	}
	candAttrs, err := s.loadAttrSets(ctx, animeIDsToStrings(candidates))
	if err != nil {
		return nil, fmt.Errorf("s7: load candidate attrs: %w", err)
	}

	for _, candidateID := range candidates {
		cset := candAttrs[string(candidateID)]
		if len(cset) == 0 {
			continue
		}
		var best float64
		for _, sset := range seedAttrs {
			if j := jaccard(sset, cset); j > best {
				best = j
			}
		}
		if best > 0 {
			out[candidateID] = recs.RawScore(best)
		}
	}
	return out, nil
}

// loadAttrSets builds per-anime namespaced attribute sets from anime_genres
// ("genre:{id}") and anime_tags ("tag:{id}") in two batched queries.
func (s *S7DroppedPenalty) loadAttrSets(ctx context.Context, animeIDs []string) (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{}, len(animeIDs))
	if len(animeIDs) == 0 {
		return out, nil
	}
	add := func(animeID, key string) {
		set, ok := out[animeID]
		if !ok {
			set = make(map[string]struct{})
			out[animeID] = set
		}
		set[key] = struct{}{}
	}

	var genreRows []s2GenreRow
	if err := s.db.WithContext(ctx).
		Table("anime_genres").Select("anime_id, genre_id").
		Where("anime_id IN ?", animeIDs).Scan(&genreRows).Error; err != nil {
		return nil, err
	}
	for _, r := range genreRows {
		add(r.AnimeID, "genre:"+r.GenreID)
	}

	var tagRows []struct {
		AnimeID string
		TagID   string
	}
	if err := s.db.WithContext(ctx).
		Table("anime_tags").Select("anime_id, tag_id").
		Where("anime_id IN ?", animeIDs).Scan(&tagRows).Error; err != nil {
		return nil, err
	}
	for _, r := range tagRows {
		add(r.AnimeID, "tag:"+r.TagID)
	}
	return out, nil
}

func animeIDsToStrings(ids []recs.AnimeID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
```
(`jaccard` and `s2GenreRow` already exist in package `signals` — reuse, don't redefine. Verify `anime_tags` column is `tag_id` — `\d anime_tags` via psql; adjust if the FK column is named differently.)

- [ ] **Step 4: Run tests** — `go test ./internal/service/recs/signals/ -run TestS7 -count=1 -race` → PASS.

- [ ] **Step 5: Commit**

```bash
git add services/recs/internal/service/recs/signals/s7_dropped_penalty.go services/recs/internal/service/recs/signals/s7_dropped_penalty_test.go
git commit -m "feat(recs): S7 dropped-penalty signal (max genre+tag Jaccard vs dropped seeds)"
git push
```

### Task 15: Wire S7 into the ensemble + cache bump + deploy

**Files:**
- Modify: `services/recs/internal/handler/recs.go` (struct, constructor, `computeFreshForUser`)
- Modify: `services/recs/internal/handler/admin_recs.go` (weights map line ~198, ensemble line ~281)
- Modify: `services/recs/internal/service/recs/user_orchestrator.go` (`UserTopNKeySuffix`)

- [ ] **Step 1: Handler** — in `recs.go`: add field `s7 *signals.S7DroppedPenalty` to `RecsHandler`; in `NewRecsHandler` add `s7: signals.NewS7DroppedPenalty(db),`. In `computeFreshForUser`, extend BOTH the ensemble (line ~405) and `upNextWeights` (line ~449) with:

```go
		{Module: h.s7, Weight: -0.15}, // S7 dropped-penalty (spec 2026-06-11 §Phase 3): demotes, never buries
```
Update the function doc comment: `... + 0.20·S5 − 0.15·S7 ensemble`. Note: positive weights still sum to 1.0; S7 subtracts. `deriveTopContributor` is safe — a negative weighted contribution can never exceed a non-negative one, so S7 never becomes `top_contributor` (the initial `topVal = -1.0` only matters in the all-zero case, where the first POSITIVE-weight signal still wins because iteration order starts at s1).

- [ ] **Step 2: Admin** — in `admin_recs.go`: add `recs.SignalID("s7"): -0.15,` to `adminEnsembleWeights` (line ~198); add `{Module: h.s7, Weight: adminEnsembleWeights[recs.SignalID("s7")]},` to its ensemble construction (line ~281); add the `s7` field + construction to `AdminRecsHandler`/`NewAdminRecsHandler` mirroring how `s5` is held there.

- [ ] **Step 3: Cache bump** — in `user_orchestrator.go` line 33: `UserTopNKeySuffix = ":topN:v2"` → `":topN:v3"`, and extend the comment: `// :v3 — S7 dropped-penalty entered the ensemble (2026-06-11); v2 rankings are pre-S7.` This single constant feeds handler + orchestrator + admin paths (that's why it exists) — verify with `grep -rn "topN:v2" services/recs/` → zero hits after the change.

- [ ] **Step 4: Tests + handler-level assertion.** Add to the existing handler test file (reuse its fixture style) a test asserting a candidate similar to dropped seeds ranks BELOW an otherwise-equal candidate. Then:

```bash
cd services/recs && go test ./... -count=1 -race 2>&1 | tail -5
```
Expected: PASS. Existing recs_test.go fixtures may need the new constructor field — fix mechanically.

- [ ] **Step 5: Deploy + verify**

```bash
make redeploy-recs && make health
curl -s http://localhost:8000/api/users/recs -H "Authorization: Bearer $UI_KEY" | jq '.data.recs[0:3] | map({rank, final})'
docker compose -f docker/docker-compose.yml exec -T redis redis-cli KEYS 'recs:user:*:topN:*' | head -3
```
Expected: row serves, new keys carry `:v3`. If ui_audit_bot has ≥2 dropped anime in its seed list, compare against the Phase-1 capture to see S7 demotions; otherwise drop two seeded anime via the UI/API first.

- [ ] **Step 6: Run `/animeenigma-after-update`** for Phase 3.

---

# PHASE 4 — S12 diversification re-rank (Tasks 16–18)

### Task 16: Diversifier (TDD)

**Files:**
- Create: `services/recs/internal/service/recs/diversify.go`
- Test: `services/recs/internal/service/recs/diversify_test.go`

- [ ] **Step 1: Write the failing test:**

```go
package recs

import (
	"context"
	"testing"
)

// fakeAttrLoader implements attrLoader with a fixed map.
type fakeAttrLoader struct{ sets map[string]map[string]struct{} }

func (f *fakeAttrLoader) LoadAttrSets(_ context.Context, ids []string) (map[string]map[string]struct{}, error) {
	return f.sets, nil
}

func set(keys ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

func recsList(pairs ...any) []Recommendation {
	out := make([]Recommendation, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Recommendation{AnimeID: AnimeID(pairs[i].(string)), Final: pairs[i+1].(float64)})
	}
	return out
}

func TestDiversify_LambdaZeroIsIdentity(t *testing.T) {
	d := NewDiversifier(&fakeAttrLoader{sets: map[string]map[string]struct{}{
		"a": set("genre:1"), "b": set("genre:1"), "c": set("genre:2"),
	}})
	in := recsList("a", 0.9, "b", 0.8, "c", 0.7)
	got, err := d.Rerank(context.Background(), in, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range in {
		if got[i].AnimeID != in[i].AnimeID {
			t.Fatalf("lambda=0 must preserve order; pos %d = %s, want %s", i, got[i].AnimeID, in[i].AnimeID)
		}
	}
}

func TestDiversify_PrefersDiverseOverNearDuplicate(t *testing.T) {
	// b is a near-clone of a (same genre+studio); c is different but slightly
	// lower-scored. With lambda=0.3 c must outrank b in position 2.
	d := NewDiversifier(&fakeAttrLoader{sets: map[string]map[string]struct{}{
		"a": set("genre:1", "studio:x"), "b": set("genre:1", "studio:x"), "c": set("genre:2", "studio:y"),
	}})
	in := recsList("a", 0.90, "b", 0.85, "c", 0.80)
	got, _ := d.Rerank(context.Background(), in, "", 0.3)
	if got[0].AnimeID != "a" || got[1].AnimeID != "c" || got[2].AnimeID != "b" {
		t.Fatalf("order = %v, want [a c b]", []AnimeID{got[0].AnimeID, got[1].AnimeID, got[2].AnimeID})
	}
}

func TestDiversify_GenreSetHardCap(t *testing.T) {
	// five items with the IDENTICAL genre set; cap=3 → items 4 and 5 must be
	// pushed behind the different item f even at lambda that wouldn't reorder.
	sets := map[string]map[string]struct{}{
		"a": set("genre:1"), "b": set("genre:1"), "c": set("genre:1"),
		"d": set("genre:1"), "e": set("genre:1"), "f": set("genre:2"),
	}
	d := NewDiversifier(&fakeAttrLoader{sets: sets})
	in := recsList("a", 0.9, "b", 0.89, "c", 0.88, "d", 0.87, "e", 0.86, "f", 0.5)
	got, _ := d.Rerank(context.Background(), in, "", 0)
	if got[3].AnimeID != "f" {
		t.Fatalf("position 4 = %s, want f (cap of 3 identical genre-sets)", got[3].AnimeID)
	}
	if len(got) != len(in) {
		t.Fatalf("rerank must keep all items, got %d of %d", len(got), len(in))
	}
}

func TestDiversify_SeedCountsAsPicked(t *testing.T) {
	// seed "p" is a clone of "a": with the seed provided, "a" gets a
	// similarity penalty immediately and diverse "c" wins position 1.
	d := NewDiversifier(&fakeAttrLoader{sets: map[string]map[string]struct{}{
		"p": set("genre:1", "studio:x"), "a": set("genre:1", "studio:x"), "c": set("genre:2", "studio:y"),
	}})
	in := recsList("a", 0.9, "c", 0.85)
	got, _ := d.Rerank(context.Background(), in, "p", 0.3)
	if got[0].AnimeID != "c" {
		t.Fatalf("seed-similar item must be demoted; pos 0 = %s, want c", got[0].AnimeID)
	}
}
```

- [ ] **Step 2: Run to verify failure** — compile error on `NewDiversifier`.

- [ ] **Step 3: Implement `diversify.go`:**

```go
package recs

import (
	"context"
	"sort"
	"strings"
)

// attrLoader supplies per-anime attribute sets (namespaced "genre:{id}" /
// "studio:{id}") for similarity. Production: GormAttrLoader (below); tests
// inject a fake.
type attrLoader interface {
	LoadAttrSets(ctx context.Context, animeIDs []string) (map[string]map[string]struct{}, error)
}

// Diversifier is the S12 post-rank greedy MMR re-rank (spec 2026-06-11
// Phase 4). It never adds or removes items — it only reorders, trading a
// little Final score for variety so the row isn't 20 near-identical cards.
type Diversifier struct {
	loader attrLoader
}

// s12GenreSetCap: at most this many picked items may share an IDENTICAL
// genre-ID set — the closest stand-in for franchise dedup until a real
// franchise column exists (sequels share exact genre sets).
const s12GenreSetCap = 3

func NewDiversifier(loader attrLoader) *Diversifier {
	return &Diversifier{loader: loader}
}

// Rerank greedily re-orders ranked by  score = Final − λ·maxSim(candidate,
// picked). seedAnimeID, when non-empty (the S6 pin), counts as already
// picked for similarity but is NOT part of the output. λ=0 degenerates to
// the input order. The full input always comes back, only reordered.
func (d *Diversifier) Rerank(ctx context.Context, ranked []Recommendation, seedAnimeID string, lambda float64) ([]Recommendation, error) {
	if len(ranked) <= 1 {
		return ranked, nil
	}

	ids := make([]string, 0, len(ranked)+1)
	for _, r := range ranked {
		ids = append(ids, string(r.AnimeID))
	}
	if seedAnimeID != "" {
		ids = append(ids, seedAnimeID)
	}
	attrs, err := d.loader.LoadAttrSets(ctx, ids)
	if err != nil {
		return nil, err
	}

	pickedSets := make([]map[string]struct{}, 0, len(ranked)+1)
	if seedAnimeID != "" {
		if s, ok := attrs[seedAnimeID]; ok {
			pickedSets = append(pickedSets, s)
		}
	}
	genreSigCount := make(map[string]int, len(ranked))

	remaining := make([]Recommendation, len(ranked))
	copy(remaining, ranked)
	out := make([]Recommendation, 0, len(ranked))

	for len(remaining) > 0 {
		bestIdx := -1
		bestScore := 0.0
		capped := true // does every remaining item violate the genre cap?
		for i, cand := range remaining {
			if genreSigCount[genreSignature(attrs[string(cand.AnimeID)])] >= s12GenreSetCap {
				continue
			}
			capped = false
			score := cand.Final - lambda*maxSim(attrs[string(cand.AnimeID)], pickedSets)
			if bestIdx == -1 || score > bestScore {
				bestIdx = i
				bestScore = score
			}
		}
		if capped {
			// Everything left violates the cap — relax it (keep all items,
			// reordering only) and pick by plain MMR score.
			for i, cand := range remaining {
				score := cand.Final - lambda*maxSim(attrs[string(cand.AnimeID)], pickedSets)
				if bestIdx == -1 || score > bestScore {
					bestIdx = i
					bestScore = score
				}
			}
		}

		pick := remaining[bestIdx]
		out = append(out, pick)
		pickedSets = append(pickedSets, attrs[string(pick.AnimeID)])
		genreSigCount[genreSignature(attrs[string(pick.AnimeID)])]++
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}
	return out, nil
}

// maxSim is the max Jaccard similarity between set a and any picked set.
func maxSim(a map[string]struct{}, picked []map[string]struct{}) float64 {
	var best float64
	for _, p := range picked {
		if j := jaccardSets(a, p); j > best {
			best = j
		}
	}
	return best
}

func jaccardSets(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersect := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersect++
		}
	}
	union := len(a) + len(b) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

// genreSignature returns a canonical string for the genre subset of an
// attribute set ("genre:1|genre:5"), used for the identical-genre-set cap.
func genreSignature(attrs map[string]struct{}) string {
	genres := make([]string, 0, len(attrs))
	for k := range attrs {
		if strings.HasPrefix(k, "genre:") {
			genres = append(genres, k)
		}
	}
	sort.Strings(genres)
	return strings.Join(genres, "|")
}
```
And the production loader in the same file (genres + studios — studios for similarity, genres for both similarity and the cap):

```go
// GormAttrLoader loads namespaced genre+studio sets from the shared DB.
type GormAttrLoader struct{ db *gorm.DB }

func NewGormAttrLoader(db *gorm.DB) *GormAttrLoader { return &GormAttrLoader{db: db} }

func (l *GormAttrLoader) LoadAttrSets(ctx context.Context, animeIDs []string) (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{}, len(animeIDs))
	if len(animeIDs) == 0 {
		return out, nil
	}
	add := func(animeID, key string) {
		set, ok := out[animeID]
		if !ok {
			set = make(map[string]struct{})
			out[animeID] = set
		}
		set[key] = struct{}{}
	}
	var rows []struct {
		AnimeID string
		AttrID  string
	}
	if err := l.db.WithContext(ctx).Table("anime_genres").
		Select("anime_id, genre_id AS attr_id").Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		add(r.AnimeID, "genre:"+r.AttrID)
	}
	rows = nil
	if err := l.db.WithContext(ctx).Table("anime_studios").
		Select("anime_id, studio_id AS attr_id").Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		add(r.AnimeID, "studio:"+r.AttrID)
	}
	return out, nil
}
```
(add the `gorm.io/gorm` import; verify `anime_studios.studio_id` column name via psql `\d anime_studios`.)

- [ ] **Step 4: Run tests** — `go test ./internal/service/recs/ -run TestDiversify -count=1 -race` → PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add services/recs/internal/service/recs/diversify.go services/recs/internal/service/recs/diversify_test.go
git commit -m "feat(recs): S12 greedy-MMR diversifier with identical-genre-set cap"
git push
```

### Task 17: Wire S12 into both rows + cache bumps

**Files:**
- Modify: `services/recs/internal/handler/recs.go`
- Modify: `services/recs/internal/handler/admin_recs.go`
- Modify: `services/recs/internal/service/recs/user_orchestrator.go` (suffix v3→v4)

- [ ] **Step 1: Handler wiring.** Add to `RecsHandler`: field `diversifier *recs.Diversifier`; in `NewRecsHandler`: `diversifier: recs.NewDiversifier(recs.NewGormAttrLoader(db)),`. Add the lambda constant next to the other consts:

```go
// s12Lambda is the MMR diversity strength (spec 2026-06-11 Phase 4):
// score = final − λ·max_similarity(candidate, picked). 0.3 is conservative;
// tune against the Phase-2 CTR panels.
const s12Lambda = 0.3
```

- [ ] **Step 2: Restructure `computeFreshForUser`** — pin resolution moves BEFORE the MMR so the pin counts as picked. The section between the `top := ranked[:end]` slice and the hydrate call becomes:

```go
	// Phase 13 S6 pin — resolved BEFORE the S12 re-rank so the pinned anime
	// counts as "already picked" for similarity (the row must not open with
	// three clones of the pin). Spec 2026-06-11 Phase 4.
	var pin *signals.PinCandidate
	if h.s6 != nil {
		topIDs := make([]string, 0, len(top))
		for _, r := range top {
			topIDs = append(topIDs, string(r.AnimeID))
		}
		var s6Err error
		pin, s6Err = h.s6.Resolve(ctx, userID, topIDs)
		if s6Err != nil {
			h.log.Warnw("s6 resolve failed (non-fatal)", "user_id", userID, "error", s6Err)
			pin = nil
		}
	}

	// S12 diversification re-rank (greedy MMR over the server slice).
	// Non-fatal: on error serve the undiversified order.
	pinSeed := ""
	if pin != nil {
		pinSeed = pin.AnimeID
	}
	if diversified, dErr := h.diversifier.Rerank(ctx, top, pinSeed, s12Lambda); dErr != nil {
		h.log.Warnw("s12 rerank failed (non-fatal); serving undiversified", "user_id", userID, "error", dErr)
	} else {
		top = diversified
	}
```
Then the existing hydrate + items loop runs unchanged on the reordered `top`, and the existing pin block (line ~479 onward) changes only its first line: it already HAS the resolved `pin` — delete the inner `h.s6.Resolve` call + `topIDs` rebuild and start from `if pin != nil { pinHydrated, hydrateErr := ... }` (keep the dedup/prepend/re-rank logic verbatim). Check `signals.PinCandidate` is the exported type `Resolve` returns (grep `func (s *S6ComboPin) Resolve` for the exact return type and field names — `AnimeID`/`SeedName`/`SeedAnimeID`/`Source` per current usage).

- [ ] **Step 3: Anonymous row** — in `computeFresh`, after its `top := ranked[:end]` slice, add the same Rerank with no seed:

```go
	// S12 diversification — the trending row is the most genre-monotone.
	if diversified, dErr := h.diversifier.Rerank(ctx, top, "", s12Lambda); dErr != nil {
		h.log.Warnw("s12 rerank failed (non-fatal); serving undiversified", "error", dErr)
	} else {
		top = diversified
	}
```

- [ ] **Step 4: Cache bumps.** `user_orchestrator.go`: `UserTopNKeySuffix` `":topN:v3"` → `":topN:v4"` (comment: `// :v4 — S12 diversification re-rank (2026-06-11)`). `recs.go`: `PublicTrendingKey = "recs:public:trending:topN"` → `"recs:public:trending:topN:v2"` (extend its comment: the un-versioned key predates S12).

- [ ] **Step 5: Admin debug `pre_s12_rank`.** In `admin_recs.go`: add `diversifier` to `AdminRecsHandler` (constructed the same way). Find where the admin response rows are assembled from `RankWithBreakdown` output (the struct with `Weights` at line ~118 and assembly near lines ~332–393). Add an int field to the per-row response struct:

```go
	PreS12Rank int `json:"pre_s12_rank"` // 1-based rank before the S12 MMR re-rank (Phase 4)
```
Then, after the admin path's sort + slice of breakdown rows: record each AnimeID's position into a `map[recs.AnimeID]int` (1-based), build a `[]recs.Recommendation` projection (`AnimeID` + `Final`), run `h.diversifier.Rerank(ctx, proj, pinSeed, s12Lambda)` with the same pin-seed the admin path resolves (if it resolves one; else `""`), reorder the breakdown rows to match, and set `PreS12Rank` from the recorded map. Keep the same non-fatal error contract (serve undiversified on error). The admin response must mirror the public ordering — assert that in the admin test below.

- [ ] **Step 6: Tests.** Update existing handler tests broken by the restructure (constructor field, pin-flow reorder). Add one handler-level test: with a fixture where two near-identical candidates lead, assert the served order interleaves (position 2 is the diverse item) and `rank` fields are sequential post-rerank. Run:

```bash
cd services/recs && go test ./... -count=1 -race 2>&1 | tail -5
```
Expected: PASS. Also `grep -rn "topN:v3" services/recs/` → zero hits.

- [ ] **Step 7: Commit**

```bash
git add services/recs
git commit -m "feat(recs): S12 diversification wired into both rows; cache keys bumped (user v4, public v2)"
git push
```

### Task 18: Deploy + verify + milestone close

- [ ] **Step 1: Deploy**

```bash
make redeploy-recs && make health
```

- [ ] **Step 2: Verify reordering + key bumps**

```bash
curl -s http://localhost:8000/api/users/recs | jq '.data.recs | map(.anime.name)[0:10]'
curl -s http://localhost:8000/api/users/recs -H "Authorization: Bearer $UI_KEY" | jq '.data.recs | map({rank, name: .anime.name, final})[0:10]'
docker compose -f docker/docker-compose.yml exec -T redis redis-cli KEYS 'recs:*topN*'
```
Expected: rows serve; keys are `recs:public:trending:topN:v2` and `recs:user:*:topN:v4`; the trending row is visibly less genre-monotone than the Phase-1 baseline capture (compare `/tmp/recs-baseline-anon.json` names). The S6 pin (if one fires for ui_audit_bot) still sits at rank 1 with `pinned: true`.

- [ ] **Step 3: In-browser smoke (DS-NF-06)** — home page rec row renders normally at desktop + mobile (ranking changed, surface didn't — still mandatory after a rendered-data change), console clean.

- [ ] **Step 4: Run `/animeenigma-after-update`** for Phase 4 (this is the user-visible changelog entry: smarter row — penalizes dropped-similar, diversifies the lineup).

- [ ] **Step 5: Milestone wrap** — update the spec's status line to `Implemented 2026-06-XX`, commit, push.

---

## Self-review checklist (already applied)

- Spec coverage: Phase ① extraction (Tasks 1–11: scaffold/move/seam/gateway/ops/docs/gate), Phase ② ISS-026 (12–13), Phase ③ S7 (14–15), Phase ④ S12 (16–18), out-of-scope items untouched. ✓
- The trigger seam consolidates BOTH player couplings (debounce + S6 seed) into one hint — discovered during recon, refines the spec's "debounce moves into recs" wording; behavior preserved. ✓
- Type consistency: `recs.UserID`/`recs.AnimeID` conversions explicit; `UserTopNKey` reused in hint handler; `jaccard` reused by S7; `Recommendation` reused by Diversifier. ✓
- Known executor-discretion points are bounded with explicit anchors and verification greps (middleware copy, notifications config field shapes, admin row assembly, mark-watched API path, anime_tags/anime_studios column names).
