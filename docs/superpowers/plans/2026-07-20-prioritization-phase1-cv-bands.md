# Unified Prioritization — Phase 1: Interest Endpoint + content-verify Bands — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace content-verify's flat visitor-dominated priority score with a banded model (pinned / hot-ongoing / watched+top / idle-backfill), fed by a new catalog `/internal/interest/bands` endpoint, with weighted band selection and a round-robin idle sweep of the catalog tail.

**Architecture:** Catalog gains one Docker-network-only endpoint exposing per-anime bands + raw signals (ongoing, top window, planned counts, next_episode_at, idle window at a cursor offset). content-verify replaces `Candidate.Score()`/`Rank()` with `BandOf`/`IntraLess`, picks a band per claim by weighted lottery with deterministic fall-through, and advances a Redis idle cursor to sweep top-200/300/… + user watchlists when the hot set is settled.

**Tech Stack:** Go (catalog :8081, content-verify :8101), PostgreSQL (`animeenigma` DB), Redis. Tests: catalog = in-memory SQLite with explicit DDL (mirror `services/catalog/internal/repo/browse_filter_test.go`); cv = `github.com/alicebob/miniredis/v2` + `httptest` (mirror `services/content-verify/internal/queue/engine_test.go`).

## Global Constraints

- **content-verify k8s replicas MUST stay 1** — probe leases are in-process only. Nothing here changes that.
- **`/internal/*` is Docker-network-only** — never proxied by the gateway; no auth middleware, same model as the existing `/internal/verify/membership`.
- **Fail-open everywhere:** any Redis or catalog-endpoint error degrades to "lowest actionable signal", never blocks a tick. `signals.go` already returns 0/nil on Redis error — preserve.
- **All new weights/windows are env-configurable** — no compile-time priority magic left in `queue.go`.
- **Never run `gofmt -w` or `make fmt`** (smart-quote landmine). Fix any `gofmt -l` finding by hand.
- **Co-authors** on every commit: `Co-Authored-By: Claude Code <noreply@anthropic.com>`, `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>`, `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`.
- Keep the existing `/internal/verify/membership` route working unchanged (one-release alias; do not delete it in this phase).

---

## File Structure

**Catalog (new/modified):**
- `services/catalog/internal/repo/anime.go` — add `InterestRow`, `InterestBands` types + `ListInterestBands(...)` method (near existing `ListVerifyMembership` at :625-650).
- `services/catalog/internal/repo/interest_test.go` — **new** repo test (SQLite + DDL for `animes` + `anime_list`).
- `services/catalog/internal/handler/internal_interest.go` — **new** handler `GET /internal/interest/bands`.
- `services/catalog/internal/handler/internal_interest_test.go` — **new** handler test (stub source + httptest).
- `services/catalog/internal/transport/router.go` — mount the new route next to the existing `/internal/verify/membership`.
- `services/catalog/cmd/catalog-api/main.go` — construct + wire `NewInternalInterestHandler`.

**content-verify (modified):**
- `services/content-verify/internal/config/config.go` — new env knobs; delete dead `TopLimit`.
- `services/content-verify/internal/catalogclient/client.go` — add `Interest`/`InterestRow` types + `InterestBands(ctx, idleOffset, idleWindow)` method.
- `services/content-verify/internal/catalogclient/client_test.go` — add test for the new method.
- `services/content-verify/internal/queue/queue.go` — `Band` type, `BandOf`, `freshBoost`, `IntraLess`, `weightedPick`, `bandOrder`, `CooldownTTL(band, idle)`, new `Candidate` fields; remove `Score()`/`Rank()`.
- `services/content-verify/internal/queue/queue_test.go` — replace `Score`/`Rank`/`CooldownTTL` tests with band tests.
- `services/content-verify/internal/queue/bands_test.go` — **new** table tests for `BandOf`/`IntraLess`/`weightedPick`/`bandOrder`.
- `services/content-verify/internal/signals/signals.go` — idle cursor get/set.
- `services/content-verify/internal/signals/signals_test.go` — cursor test.
- `services/content-verify/internal/queue/engine.go` — `BuildCandidates` new signature, `interest()` replacing `membership()`, `bandedCandidates()`, band-aware cooldown in `Claim`, updated `Snapshot`/`QueueEntry`.
- `services/content-verify/internal/queue/engine_test.go` — update fakes/asserts for banded claim.
- `services/content-verify/cmd/content-verify-api/main.go` — pass new config to `NewEngine`.
- `docs/environment-variables.md` — document the new `CV_*` knobs.

---

## Task 1: Catalog `ListInterestBands` repo method

**Files:**
- Modify: `services/catalog/internal/repo/anime.go` (add after `ListVerifyMembership`, ~line 650)
- Test: `services/catalog/internal/repo/interest_test.go` (create)

**Interfaces:**
- Produces: `repo.InterestRow{ID string; Name string; EpisodesAired int; Score float64; NextEpisodeAt *time.Time; TopRank int; Planners int}`; `repo.InterestBands{Ongoing, Top, Planned, IdleWindow []InterestRow; IdleTotal int}`; `func (r *AnimeRepository) ListInterestBands(ctx context.Context, ongoingLimit, topLimit, idleWindow, idleOffset int) (InterestBands, error)`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/repo/interest_test.go`. Mirror `browse_filter_test.go`'s SQLite setup; add an `anime_list` table for the planned count.

```go
package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInterestTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY, name TEXT, status TEXT, score REAL,
		episodes_aired INTEGER DEFAULT 0, next_episode_at DATETIME,
		hidden INTEGER DEFAULT 0, sort_priority INTEGER DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_list (
		user_id TEXT, anime_id TEXT, status TEXT
	)`).Error)
	return db
}

func seedInterestAnime(t *testing.T, db *gorm.DB, id, name, status string, score float64, aired, sortPri int, hidden bool) {
	t.Helper()
	h := 0
	if hidden {
		h = 1
	}
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id,name,status,score,episodes_aired,sort_priority,hidden) VALUES (?,?,?,?,?,?,?)`,
		id, name, status, score, aired, sortPri, h).Error)
}

func TestListInterestBands(t *testing.T) {
	db := setupInterestTestDB(t)
	// ongoing (2), released top-eligible (3), one hidden (excluded everywhere).
	seedInterestAnime(t, db, "ong1", "Ongoing High", "ongoing", 8.5, 12, 0, false)
	seedInterestAnime(t, db, "ong2", "Ongoing Low", "ongoing", 6.0, 3, 0, false)
	seedInterestAnime(t, db, "rel1", "Released A", "released", 9.1, 24, 5, false)
	seedInterestAnime(t, db, "rel2", "Released B", "released", 7.0, 12, 0, false)
	seedInterestAnime(t, db, "rel3", "Released C", "released", 6.5, 24, 0, false)
	seedInterestAnime(t, db, "hid1", "Hidden", "released", 9.9, 1, 9, true)
	// planners: rel2 has 2 plan_to_watch, rel3 has 1.
	require.NoError(t, db.Exec(`INSERT INTO anime_list (user_id,anime_id,status) VALUES
		('u1','rel2','plan_to_watch'),('u2','rel2','plan_to_watch'),('u3','rel3','plan_to_watch'),
		('u4','rel2','watching')`).Error)

	r := &AnimeRepository{db: db}
	b, err := r.ListInterestBands(context.Background(), 500, 2, 2, 0)
	require.NoError(t, err)

	// Ongoing: only status=ongoing, score DESC, hidden excluded.
	require.Len(t, b.Ongoing, 2)
	require.Equal(t, "ong1", b.Ongoing[0].ID)

	// Top: browse order sort_priority DESC, score DESC, hidden excluded, LIMIT 2.
	// rel1 (sort_priority 5) first, then highest score among the rest = ong1 (8.5).
	require.Len(t, b.Top, 2)
	require.Equal(t, "rel1", b.Top[0].ID)
	require.Equal(t, 1, b.Top[0].TopRank)
	require.Equal(t, 2, b.Top[1].TopRank)

	// Planned: non-ongoing with plan_to_watch, planners DESC, LIMIT 2.
	require.Len(t, b.Planned, 2)
	require.Equal(t, "rel2", b.Planned[0].ID)
	require.Equal(t, 2, b.Planned[0].Planners)

	// IdleWindow: non-ongoing browse order, OFFSET 0 LIMIT 2 → rel1, rel3 (rel1 sort_priority 5 first, then score DESC rel3 6.5 > rel2 7.0? rel2=7.0 > rel3=6.5, so rel1, rel2).
	require.Len(t, b.IdleWindow, 2)
	require.Equal(t, "rel1", b.IdleWindow[0].ID)
	require.Equal(t, "rel2", b.IdleWindow[1].ID)

	// IdleTotal: all visible non-ongoing = rel1, rel2, rel3 = 3.
	require.Equal(t, 3, b.IdleTotal)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/content-verify-probing && go test ./services/catalog/internal/repo/ -run TestListInterestBands`
Expected: FAIL — `InterestRow`/`InterestBands`/`ListInterestBands` undefined.

- [ ] **Step 3: Write the types + method**

Add to `services/catalog/internal/repo/anime.go` after `ListVerifyMembership` (~:650):

```go
import "time" // ensure imported (already used elsewhere in the file)

// InterestRow is the richer projection the unified interest endpoint returns:
// identity + the raw signals each content-verify band ranks on.
type InterestRow struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	EpisodesAired int        `json:"episodes_aired"`
	Score         float64    `json:"score"`
	NextEpisodeAt *time.Time `json:"next_episode_at,omitempty"`
	TopRank       int        `json:"top_rank,omitempty"`  // 1-based browse rank in the top window
	Planners      int        `json:"planners,omitempty"`  // count of plan_to_watch rows
}

// InterestBands is the full interest snapshot for the content-verify queue.
type InterestBands struct {
	Ongoing    []InterestRow `json:"ongoing"`
	Top        []InterestRow `json:"top"`
	Planned    []InterestRow `json:"planned"`
	IdleWindow []InterestRow `json:"idle_window"`
	IdleTotal  int           `json:"idle_total"`
}

// ListInterestBands returns the banded interest snapshot. `idleOffset` pages the
// non-ongoing browse tail so content-verify can round-robin through top-200/300/…;
// the caller seeds idleOffset at topLimit so the idle window never overlaps `Top`.
func (r *AnimeRepository) ListInterestBands(ctx context.Context, ongoingLimit, topLimit, idleWindow, idleOffset int) (InterestBands, error) {
	var b InterestBands
	db := r.db.WithContext(ctx)

	// Band 1 — visible ongoings, score DESC.
	if err := db.Model(&domain.Anime{}).
		Select("id, name, episodes_aired, score, next_episode_at").
		Where("status = ? AND (hidden = ? OR hidden IS NULL)", "ongoing", false).
		Order("score DESC").Limit(ongoingLimit).
		Scan(&b.Ongoing).Error; err != nil {
		return b, err
	}

	// Band 2 slice — browse-order top window.
	if err := db.Model(&domain.Anime{}).
		Select("id, name, episodes_aired, score").
		Where("hidden = ? OR hidden IS NULL", false).
		Order("sort_priority DESC, score DESC").Limit(topLimit).
		Scan(&b.Top).Error; err != nil {
		return b, err
	}
	for i := range b.Top {
		b.Top[i].TopRank = i + 1
	}

	// Band 3 sub-source (a) — planned (non-ongoing), planners DESC.
	if err := db.Table("animes a").
		Select("a.id AS id, a.name AS name, a.episodes_aired AS episodes_aired, a.score AS score, COUNT(al.anime_id) AS planners").
		Joins("JOIN anime_list al ON al.anime_id = a.id AND al.status = ?", "plan_to_watch").
		Where("a.status <> ? AND (a.hidden = ? OR a.hidden IS NULL)", "ongoing", false).
		Group("a.id, a.name, a.episodes_aired, a.score").
		Order("planners DESC, a.score DESC").Limit(idleWindow).
		Scan(&b.Planned).Error; err != nil {
		return b, err
	}

	// Band 3 sub-source (b) — non-ongoing browse tail at the cursor offset.
	if err := db.Model(&domain.Anime{}).
		Select("id, name, episodes_aired, score").
		Where("status <> ? AND (hidden = ? OR hidden IS NULL)", "ongoing", false).
		Order("sort_priority DESC, score DESC").Offset(idleOffset).Limit(idleWindow).
		Scan(&b.IdleWindow).Error; err != nil {
		return b, err
	}
	for i := range b.IdleWindow {
		b.IdleWindow[i].TopRank = idleOffset + i + 1
	}

	// idle_total — visible non-ongoing count, so the caller wraps the cursor.
	var total int64
	if err := db.Model(&domain.Anime{}).
		Where("status <> ? AND (hidden = ? OR hidden IS NULL)", "ongoing", false).
		Count(&total).Error; err != nil {
		return b, err
	}
	b.IdleTotal = int(total)
	return b, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/catalog/internal/repo/ -run TestListInterestBands`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/repo/anime.go services/catalog/internal/repo/interest_test.go
git commit -m "feat(catalog): ListInterestBands repo method for unified interest endpoint"
```

---

## Task 2: Catalog `/internal/interest/bands` handler + route

**Files:**
- Create: `services/catalog/internal/handler/internal_interest.go`
- Test: `services/catalog/internal/handler/internal_interest_test.go`
- Modify: `services/catalog/internal/transport/router.go` (mount next to `/internal/verify/membership`)
- Modify: `services/catalog/cmd/catalog-api/main.go` (construct handler)

**Interfaces:**
- Consumes: `repo.ListInterestBands` (Task 1).
- Produces: `GET /internal/interest/bands?ongoing_limit=&top_limit=&idle_window=&idle_offset=` → `{"success":true,"data":{ongoing,top,planned,idle_window,idle_total}}`.

- [ ] **Step 1: Write the failing handler test**

Create `services/catalog/internal/handler/internal_interest_test.go` (mirror `internal_verify_test.go`):

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type stubInterest struct {
	bands                                             repo.InterestBands
	err                                               error
	gotOngoing, gotTop, gotIdleWindow, gotIdleOffset  int
}

func (s *stubInterest) ListInterestBands(_ context.Context, ongoingLimit, topLimit, idleWindow, idleOffset int) (repo.InterestBands, error) {
	s.gotOngoing, s.gotTop, s.gotIdleWindow, s.gotIdleOffset = ongoingLimit, topLimit, idleWindow, idleOffset
	return s.bands, s.err
}

func TestInterestBands(t *testing.T) {
	s := &stubInterest{bands: repo.InterestBands{
		Ongoing:   []repo.InterestRow{{ID: "o1", EpisodesAired: 12}},
		Top:       []repo.InterestRow{{ID: "t1", TopRank: 1}},
		Planned:   []repo.InterestRow{{ID: "p1", Planners: 3}},
		IdleWindow: []repo.InterestRow{{ID: "i1", TopRank: 101}},
		IdleTotal: 4824,
	}}
	h := NewInternalInterestHandler(s, nil)
	rec := httptest.NewRecorder()
	h.Bands(rec, httptest.NewRequest("GET", "/internal/interest/bands?idle_offset=100", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if s.gotIdleOffset != 100 {
		t.Fatalf("idle_offset not threaded: %d", s.gotIdleOffset)
	}
	var env struct {
		Data repo.InterestBands `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.IdleTotal != 4824 || len(env.Data.Planned) != 1 || env.Data.IdleWindow[0].TopRank != 101 {
		t.Fatalf("body: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/catalog/internal/handler/ -run TestInterestBands`
Expected: FAIL — `NewInternalInterestHandler` undefined.

- [ ] **Step 3: Write the handler**

Create `services/catalog/internal/handler/internal_interest.go`:

```go
package handler

// Unified interest bands — content-verify queue (:8101) support.
//
// GET /internal/interest/bands?ongoing_limit=500&top_limit=100&idle_window=100&idle_offset=N
//
// Superset of /internal/verify/membership: ongoing + browse-order top plus the
// idle backfill sub-sources (planned, non-ongoing tail window at idle_offset)
// and idle_total so the caller can wrap its round-robin cursor. Docker-network-
// only: /internal/* is never proxied by the gateway.

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type interestSource interface {
	ListInterestBands(ctx context.Context, ongoingLimit, topLimit, idleWindow, idleOffset int) (repo.InterestBands, error)
}

// InternalInterestHandler serves the banded interest snapshot.
type InternalInterestHandler struct {
	src interestSource
	log *logger.Logger
}

// NewInternalInterestHandler constructs the handler.
func NewInternalInterestHandler(src interestSource, log *logger.Logger) *InternalInterestHandler {
	return &InternalInterestHandler{src: src, log: log}
}

// Bands handles GET /internal/interest/bands.
func (h *InternalInterestHandler) Bands(w http.ResponseWriter, r *http.Request) {
	ongoingLimit := queryInt(r, "ongoing_limit", 500, 1, 2000)
	topLimit := queryInt(r, "top_limit", 100, 1, 500)
	idleWindow := queryInt(r, "idle_window", 100, 1, 500)
	idleOffset := queryInt(r, "idle_offset", 0, 0, 1_000_000)
	b, err := h.src.ListInterestBands(r.Context(), ongoingLimit, topLimit, idleWindow, idleOffset)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("interest bands query failed", "error", err)
		}
		http.Error(w, "interest query failed", http.StatusInternalServerError)
		return
	}
	httputil.OK(w, b)
}
```

Note: `queryInt` already exists in `internal_verify.go` (same package) — reuse it; do NOT redefine.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/catalog/internal/handler/ -run TestInterestBands`
Expected: PASS.

- [ ] **Step 5: Mount the route + wire main**

In `services/catalog/internal/transport/router.go`, find the line mounting `/internal/verify/membership` (search `verify/membership`) and add directly after it:

```go
r.Get("/internal/interest/bands", interestHandler.Bands)
```

The router constructor must accept `interestHandler *handler.InternalInterestHandler`. Add it to the router's params/struct following the exact pattern used for `verifyHandler`/`InternalVerifyHandler` (grep `InternalVerifyHandler` in `router.go` and `main.go` and mirror every occurrence).

In `services/catalog/cmd/catalog-api/main.go`, where `NewInternalVerifyHandler(animeRepo, log)` is constructed (grep `NewInternalVerifyHandler`), add beside it:

```go
interestHandler := handler.NewInternalInterestHandler(animeRepo, log)
```

and thread `interestHandler` into the router construction call the same way `verifyHandler` is threaded.

- [ ] **Step 6: Build to verify wiring**

Run: `go build ./services/catalog/...`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/handler/internal_interest.go services/catalog/internal/handler/internal_interest_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): /internal/interest/bands endpoint (banded interest snapshot)"
```

---

## Task 3: content-verify config knobs

**Files:**
- Modify: `services/content-verify/internal/config/config.go`
- Test: reuse existing config behavior; add asserts in a new `config_bands_test.go` if none exists, else extend.

**Interfaces:**
- Produces on `config.Config`: `BandWeights [3]int` (Band1/2/3), `FreshWindow time.Duration`, `IdleCooldown time.Duration`, `IdleWindow int`. **Remove** the dead `TopLimit int` field and its `CV_TOP_LIMIT` load line.

- [ ] **Step 1: Write the failing test**

Create `services/content-verify/internal/config/config_bands_test.go`:

```go
package config

import (
	"testing"
	"time"
)

func TestParseBandWeights(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
	}{
		{"", [3]int{60, 30, 10}},
		{"50,40,10", [3]int{50, 40, 10}},
		{"bad", [3]int{60, 30, 10}},
		{"1,2", [3]int{60, 30, 10}},     // wrong arity → default
		{"0,0,0", [3]int{60, 30, 10}},   // all-zero is meaningless → default
	}
	for _, c := range cases {
		if got := parseBandWeights(c.in); got != c.want {
			t.Errorf("parseBandWeights(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIdleCooldownDefault(t *testing.T) {
	if getEnvDuration("CV_IDLE_COOLDOWN_UNSET_XYZ", 168*time.Hour) != 168*time.Hour {
		t.Fatal("duration default helper regressed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/content-verify/internal/config/ -run TestParseBandWeights`
Expected: FAIL — `parseBandWeights` undefined.

- [ ] **Step 3: Implement**

In `config.go`, add fields to `Config` (after `Pins`):

```go
	// Banded prioritization (spec §3). BandWeights is the per-claim lottery
	// [Band1, Band2, Band3]; FreshWindow is the ± window on next_episode_at that
	// floats a just-aired ongoing to the front of Band 1; IdleCooldown is the
	// long cooldown a settled idle-backfill title gets; IdleWindow pages the tail.
	BandWeights  [3]int
	FreshWindow  time.Duration
	IdleCooldown time.Duration
	IdleWindow   int
```

In `Load()`, delete the `TopLimit: getEnvInt("CV_TOP_LIMIT", 100),` line and the `TopLimit int` struct field (dead — never wired). Add to the struct literal:

```go
		BandWeights:  parseBandWeights(getEnv("CV_BAND_WEIGHTS", "")),
		FreshWindow:  getEnvDuration("CV_FRESH_WINDOW", 48*time.Hour),
		IdleCooldown: getEnvDuration("CV_IDLE_COOLDOWN", 168*time.Hour),
		IdleWindow:   getEnvInt("CV_IDLE_WINDOW", 100),
```

Add the parser:

```go
// parseBandWeights parses CV_BAND_WEIGHTS ("60,30,10") into the Band1/2/3 claim
// lottery weights. Malformed input, wrong arity, or an all-zero (meaningless)
// total falls back to the 60/30/10 default — an operator env, not user input.
func parseBandWeights(s string) [3]int {
	def := [3]int{60, 30, 10}
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return def
	}
	var w [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return def
		}
		w[i] = n
	}
	if w[0]+w[1]+w[2] == 0 {
		return def
	}
	return w
}
```

`strings` and `strconv` are already imported.

- [ ] **Step 4: Run test + build**

Run: `go test ./services/content-verify/internal/config/ && go build ./services/content-verify/...`
Expected: config test PASS; build FAILS at `main.go` (still passes removed `cfg.TopLimit`) — that's fixed in Task 8. If `main.go` doesn't reference `TopLimit`, build passes. Proceed regardless; Task 8 reconciles `main.go`.

- [ ] **Step 5: Commit**

```bash
git add services/content-verify/internal/config/config.go services/content-verify/internal/config/config_bands_test.go
git commit -m "feat(content-verify): band config knobs (weights/fresh/idle); drop dead CV_TOP_LIMIT"
```

---

## Task 4: content-verify catalog client `InterestBands`

**Files:**
- Modify: `services/content-verify/internal/catalogclient/client.go`
- Test: `services/content-verify/internal/catalogclient/client_test.go`

**Interfaces:**
- Produces: `catalogclient.InterestRow{ID,Name string; EpisodesAired int; Score float64; NextEpisodeAt *time.Time; TopRank,Planners int}`; `catalogclient.Interest{Ongoing,Top,Planned,IdleWindow []InterestRow; IdleTotal int}`; `func (c *Client) InterestBands(ctx context.Context, idleOffset, idleWindow int) (*Interest, error)`.

- [ ] **Step 1: Write the failing test**

Add to `services/content-verify/internal/catalogclient/client_test.go`:

```go
func TestInterestBands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/interest/bands" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("idle_offset") != "100" || r.URL.Query().Get("idle_window") != "100" {
			t.Errorf("params %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"success":true,"data":{
			"ongoing":[{"id":"o1","episodes_aired":12,"score":8.5}],
			"top":[{"id":"t1","top_rank":1}],
			"planned":[{"id":"p1","planners":3}],
			"idle_window":[{"id":"i1","top_rank":101}],
			"idle_total":4824}}`))
	}))
	defer srv.Close()
	c := New(srv.URL, srv.URL, srv.Client())
	got, err := c.InterestBands(context.Background(), 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.IdleTotal != 4824 || got.Ongoing[0].Score != 8.5 || got.IdleWindow[0].TopRank != 101 {
		t.Fatalf("got %+v", got)
	}
}
```

(Ensure `context`, `net/http`, `net/http/httptest` are imported in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/content-verify/internal/catalogclient/ -run TestInterestBands`
Expected: FAIL — `InterestBands` undefined.

- [ ] **Step 3: Implement**

Add to `client.go` (after the `Membership` types/method):

```go
type InterestRow struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	EpisodesAired int        `json:"episodes_aired"`
	Score         float64    `json:"score"`
	NextEpisodeAt *time.Time `json:"next_episode_at"`
	TopRank       int        `json:"top_rank"`
	Planners      int        `json:"planners"`
}

type Interest struct {
	Ongoing    []InterestRow `json:"ongoing"`
	Top        []InterestRow `json:"top"`
	Planned    []InterestRow `json:"planned"`
	IdleWindow []InterestRow `json:"idle_window"`
	IdleTotal  int           `json:"idle_total"`
}

// InterestBands fetches the banded interest snapshot at the given idle cursor
// offset. Docker-network-only catalog route (no gateway exposure).
func (c *Client) InterestBands(ctx context.Context, idleOffset, idleWindow int) (*Interest, error) {
	var it Interest
	u := fmt.Sprintf("%s/internal/interest/bands?idle_offset=%d&idle_window=%d", c.catalog, idleOffset, idleWindow)
	if err := c.getJSON(ctx, u, metaTimeout, &it); err != nil {
		return nil, err
	}
	return &it, nil
}
```

`time` is already imported.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/content-verify/internal/catalogclient/ -run TestInterestBands`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/content-verify/internal/catalogclient/client.go services/content-verify/internal/catalogclient/client_test.go
git commit -m "feat(content-verify): catalog client InterestBands fetch"
```

---

## Task 5: content-verify band model (queue.go)

**Files:**
- Modify: `services/content-verify/internal/queue/queue.go` (replace `Score()`/`Rank()`, extend `Candidate`, new `CooldownTTL`)
- Test: `services/content-verify/internal/queue/bands_test.go` (create)
- Modify: `services/content-verify/internal/queue/queue_test.go` (remove obsolete `Score`/`Rank`/old-`CooldownTTL` tests)

**Interfaces:**
- Produces: `Band` (`BandPinned/BandOngoing/BandWatchedTop/BandIdle`); `Candidate` extra fields `MalScore float64; TopRank int; NextEpisodeAt *time.Time; Planners int; Idle bool`; `BandOf(Candidate) Band`; `IntraLess(a,b Candidate, now time.Time, freshWindow time.Duration) bool`; `weightedPick([3]int, float64) Band`; `bandOrder([3]int, float64) []Band`; `CooldownTTL(Band, idle time.Duration) time.Duration`.
- Note: `Score()` and `Rank()` are **removed** — Task 7 updates `engine.go` callers.

- [ ] **Step 1: Write the failing table test**

Create `services/content-verify/internal/queue/bands_test.go`:

```go
package queue

import (
	"testing"
	"time"
)

func TestBandOf(t *testing.T) {
	now := time.Now()
	_ = now
	cases := []struct {
		c    Candidate
		want Band
	}{
		{Candidate{Pinned: true, Ongoing: true}, BandPinned},
		{Candidate{Ongoing: true}, BandOngoing},
		{Candidate{Top: true}, BandWatchedTop},
		{Candidate{Visitors: 3}, BandWatchedTop},
		{Candidate{}, BandIdle},
		{Candidate{Idle: true, Planners: 5}, BandIdle},
	}
	for i, c := range cases {
		if got := BandOf(c.c); got != c.want {
			t.Errorf("case %d: BandOf=%d want %d", i, got, c.want)
		}
	}
}

func TestIntraLessOngoingFreshFirst(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	soon := now.Add(6 * time.Hour)
	far := now.Add(20 * 24 * time.Hour)
	fresh := Candidate{Ongoing: true, NextEpisodeAt: &soon, Visitors: 1, MalScore: 5}
	stale := Candidate{Ongoing: true, NextEpisodeAt: &far, Visitors: 9, MalScore: 9}
	// Fresh beats a higher-visitor stale one within the 48h window.
	if !IntraLess(fresh, stale, now, 48*time.Hour) {
		t.Fatal("fresh ongoing should sort before stale")
	}
}

func TestIntraLessWatchedByVisitorsThenRank(t *testing.T) {
	now := time.Now()
	a := Candidate{Visitors: 5, TopRank: 50}
	b := Candidate{Visitors: 2, TopRank: 1}
	if !IntraLess(a, b, now, time.Hour) {
		t.Fatal("more visitors should win in Band 2")
	}
	c := Candidate{Visitors: 0, TopRank: 10}
	d := Candidate{Visitors: 0, TopRank: 0, MalScore: 9}
	if !IntraLess(c, d, now, time.Hour) {
		t.Fatal("ranked should sort before unranked in Band 2")
	}
}

func TestWeightedPick(t *testing.T) {
	w := [3]int{60, 30, 10}
	if weightedPick(w, 0.0) != BandOngoing {
		t.Error("0.0 → Band1")
	}
	if weightedPick(w, 0.7) != BandWatchedTop {
		t.Error("0.7 → Band2")
	}
	if weightedPick(w, 0.95) != BandIdle {
		t.Error("0.95 → Band3")
	}
}

func TestBandOrderPinnedFirstThenPrimaryThenRest(t *testing.T) {
	order := bandOrder([3]int{60, 30, 10}, 0.95) // primary = Band3
	if order[0] != BandPinned || order[1] != BandIdle {
		t.Fatalf("order=%v", order)
	}
	// remaining bands present in fixed priority
	if len(order) != 4 {
		t.Fatalf("order len=%d", len(order))
	}
}

func TestCooldownTTLByBand(t *testing.T) {
	idle := 168 * time.Hour
	if CooldownTTL(BandOngoing, idle) != 6*time.Hour {
		t.Error("ongoing 6h")
	}
	if CooldownTTL(BandWatchedTop, idle) != 24*time.Hour {
		t.Error("watched 24h")
	}
	if CooldownTTL(BandIdle, idle) != idle {
		t.Error("idle = CV_IDLE_COOLDOWN")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/content-verify/internal/queue/ -run 'TestBandOf|TestIntraLess|TestWeightedPick|TestBandOrder|TestCooldownTTLByBand'`
Expected: FAIL — undefined `Band`, `BandOf`, etc.

- [ ] **Step 3: Implement in queue.go**

Delete the weight consts block (`weightVisitor/weightOngoing/weightTop/weightPinned`, `queue.go:12-19`) and the `Score()` (`:35-47`) and `Rank()` (`:91-103`) funcs and the old `CooldownTTL(ongoing bool)` (`:152-157`). Keep `backoffBase`/`backoffCap`, `Backoff`, `UnitDue`, `PendingUnits`.

Add:

```go
// Band is a content-verify priority class (spec §1). Lower value = higher
// priority; pinned titles always lead.
type Band int

const (
	BandPinned     Band = iota // operator CV_PIN_ANIME
	BandOngoing                // hot ongoing
	BandWatchedTop             // watched (visitors>0) or browse-order top-100
	BandIdle                   // idle backfill (planned + tail windows)
)

// BandOf classifies a candidate.
func BandOf(c Candidate) Band {
	switch {
	case c.Pinned:
		return BandPinned
	case c.Ongoing:
		return BandOngoing
	case c.Visitors > 0 || c.Top:
		return BandWatchedTop
	default:
		return BandIdle
	}
}

// freshBoost reports whether an ongoing has an episode within ±window of now
// (a just-aired or imminent episode), floating it to the front of Band 1.
func freshBoost(c Candidate, now time.Time, window time.Duration) bool {
	if c.NextEpisodeAt == nil {
		return false
	}
	d := c.NextEpisodeAt.Sub(now)
	if d < 0 {
		d = -d
	}
	return d <= window
}

// IntraLess reports whether a should sort BEFORE b within their shared band.
func IntraLess(a, b Candidate, now time.Time, freshWindow time.Duration) bool {
	switch BandOf(a) {
	case BandOngoing:
		if af, bf := freshBoost(a, now, freshWindow), freshBoost(b, now, freshWindow); af != bf {
			return af
		}
		if a.Visitors != b.Visitors {
			return a.Visitors > b.Visitors
		}
		return a.MalScore > b.MalScore
	case BandWatchedTop:
		if a.Visitors != b.Visitors {
			return a.Visitors > b.Visitors
		}
		aRanked, bRanked := a.TopRank > 0, b.TopRank > 0
		if aRanked != bRanked {
			return aRanked // ranked before unranked
		}
		if aRanked && a.TopRank != b.TopRank {
			return a.TopRank < b.TopRank
		}
		return a.MalScore > b.MalScore
	default: // BandIdle
		if a.Planners != b.Planners {
			return a.Planners > b.Planners
		}
		return a.MalScore > b.MalScore
	}
}

// weightedPick chooses a primary band [Band1..Band3] from the lottery weights.
func weightedPick(w [3]int, r float64) Band {
	total := w[0] + w[1] + w[2]
	if total <= 0 {
		return BandOngoing
	}
	x := r * float64(total)
	switch {
	case x < float64(w[0]):
		return BandOngoing
	case x < float64(w[0]+w[1]):
		return BandWatchedTop
	default:
		return BandIdle
	}
}

// bandOrder returns the per-claim band try-order: pins first, then the
// lottery-chosen primary, then the remaining organic bands in fixed priority.
func bandOrder(w [3]int, r float64) []Band {
	primary := weightedPick(w, r)
	order := []Band{BandPinned, primary}
	for _, b := range []Band{BandOngoing, BandWatchedTop, BandIdle} {
		if b != primary {
			order = append(order, b)
		}
	}
	return order
}

// CooldownTTL is the settled-title cooldown by band: ongoing must resurface
// same-day for new episodes; watched/top daily; idle-backfill on the long
// CV_IDLE_COOLDOWN so the tail doesn't re-spin before the cursor sweeps past.
func CooldownTTL(band Band, idle time.Duration) time.Duration {
	switch band {
	case BandOngoing, BandPinned:
		return 6 * time.Hour
	case BandWatchedTop:
		return 24 * time.Hour
	default:
		return idle
	}
}
```

Extend the `Candidate` struct with the new fields:

```go
type Candidate struct {
	AnimeID       string
	Name          string
	Ongoing       bool
	Top           bool
	Pinned        bool
	Visitors      int
	EpisodesAired int
	MalScore      float64    // animes.score — intra-band tiebreak
	TopRank       int        // 1-based browse rank; 0 = outside top/idle window
	NextEpisodeAt *time.Time // Band-1 freshBoost
	Planners      int        // Band-3 planned ordering
	Idle          bool       // sourced from planned/idle-window
}
```

- [ ] **Step 4: Remove obsolete tests**

In `services/content-verify/internal/queue/queue_test.go`, delete any `TestScore`, `TestRank`, and the old `TestCooldownTTL(ongoing bool)` cases (grep `Score()`, `Rank(`, `CooldownTTL(true`). Leave `Backoff`/`UnitDue`/`PendingUnits` tests intact.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/content-verify/internal/queue/ -run 'TestBandOf|TestIntraLess|TestWeightedPick|TestBandOrder|TestCooldownTTLByBand'`
Expected: PASS. (The package won't fully build until Task 7 fixes `engine.go`; run with `-run` to compile only when the file compiles. If the package fails to compile due to engine.go referencing removed `Score()`/`Rank()`, proceed to Task 7 and run these together — note this in the task report.)

- [ ] **Step 6: Commit**

```bash
git add services/content-verify/internal/queue/queue.go services/content-verify/internal/queue/bands_test.go services/content-verify/internal/queue/queue_test.go
git commit -m "feat(content-verify): band model (BandOf/IntraLess/weightedPick/bandOrder/CooldownTTL)"
```

---

## Task 6: content-verify idle cursor (signals)

**Files:**
- Modify: `services/content-verify/internal/signals/signals.go`
- Test: `services/content-verify/internal/signals/signals_test.go`

**Interfaces:**
- Produces: `func (s *Signals) IdleCursor(ctx context.Context) int` (0 on miss/err); `func (s *Signals) AdvanceIdleCursor(ctx context.Context, by, total int) int` — reads current, computes `next = (cur+by) % max(total,1)` (0 when total≤0), persists, returns the NEW value.

- [ ] **Step 1: Write the failing test**

Add to `services/content-verify/internal/signals/signals_test.go` (mirror its miniredis setup — grep the file for the existing `miniredis`/`New(` helper and reuse it):

```go
func TestIdleCursorAdvanceAndWrap(t *testing.T) {
	s, mr := newTestSignals(t) // existing helper in this file; if named differently, reuse that
	_ = mr
	ctx := context.Background()

	if got := s.IdleCursor(ctx); got != 0 {
		t.Fatalf("cold cursor = %d, want 0", got)
	}
	if got := s.AdvanceIdleCursor(ctx, 100, 250); got != 100 {
		t.Fatalf("advance = %d, want 100", got)
	}
	if got := s.AdvanceIdleCursor(ctx, 100, 250); got != 200 {
		t.Fatalf("advance = %d, want 200", got)
	}
	// 200+100 = 300 wraps past total 250 → 300 % 250 = 50.
	if got := s.AdvanceIdleCursor(ctx, 100, 250); got != 50 {
		t.Fatalf("wrap = %d, want 50", got)
	}
	if got := s.IdleCursor(ctx); got != 50 {
		t.Fatalf("read back = %d, want 50", got)
	}
	// total 0 (empty tail) → cursor pinned at 0, no divide-by-zero.
	if got := s.AdvanceIdleCursor(ctx, 100, 0); got != 0 {
		t.Fatalf("total 0 = %d, want 0", got)
	}
}
```

If `signals_test.go` has no shared constructor helper, add one:

```go
func newTestSignals(t *testing.T) (*Signals, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(rdb), mr
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/content-verify/internal/signals/ -run TestIdleCursorAdvanceAndWrap`
Expected: FAIL — `IdleCursor`/`AdvanceIdleCursor` undefined.

- [ ] **Step 3: Implement**

Add to `signals.go`:

```go
func idleCursorKey() string { return "cv:idle:cursor" }

// IdleCursor returns the current idle-window byte offset (0 on miss or error).
func (s *Signals) IdleCursor(ctx context.Context) int {
	v, err := s.rdb.Get(ctx, idleCursorKey()).Int()
	if err != nil {
		return 0
	}
	if v < 0 {
		return 0
	}
	return v
}

// AdvanceIdleCursor moves the idle window cursor forward by `by`, wrapping at
// `total` (the visible non-ongoing count), and persists+returns the new value.
// total<=0 pins the cursor at 0 (nothing to sweep). No TTL — the cursor is a
// long-lived sweep position, not a signal.
func (s *Signals) AdvanceIdleCursor(ctx context.Context, by, total int) int {
	if total <= 0 {
		_ = s.rdb.Set(ctx, idleCursorKey(), 0, 0).Err()
		return 0
	}
	next := (s.IdleCursor(ctx) + by) % total
	if next < 0 {
		next = 0
	}
	_ = s.rdb.Set(ctx, idleCursorKey(), next, 0).Err()
	return next
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/content-verify/internal/signals/ -run TestIdleCursorAdvanceAndWrap`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/content-verify/internal/signals/signals.go services/content-verify/internal/signals/signals_test.go
git commit -m "feat(content-verify): Redis idle-sweep cursor (get/advance+wrap)"
```

---

## Task 7: content-verify banded claim (engine.go)

**Files:**
- Modify: `services/content-verify/internal/queue/engine.go` — `BuildCandidates` (in queue.go actually; move if needed), `interest()`, `bandedCandidates()`, `Claim()` band-aware cooldown, `Snapshot`/`QueueEntry`, Engine gains `weights`, `freshWindow`, `idleCooldown`, `idleWindow`, `rng`.
- Modify: `services/content-verify/internal/queue/queue.go` — replace `BuildCandidates` signature.
- Test: `services/content-verify/internal/queue/engine_test.go` (update fakes + add banded-claim test)

**Interfaces:**
- Consumes: `catalogclient.Interest` (Task 4), `Band`/`BandOf`/`IntraLess`/`bandOrder`/`CooldownTTL` (Task 5), `Signals.IdleCursor`/`AdvanceIdleCursor` (Task 6), `config` knobs (Task 3).
- Produces: `NewEngine(cat, sig, store, reprobeTTL, skipEnabled, pins, weights [3]int, freshWindow, idleCooldown time.Duration, idleWindow int, log)`; updated `BuildCandidates(it *catalogclient.Interest, visited []string, pins map[string]string, visitors func(string) int) []Candidate`.

- [ ] **Step 1: Write the failing banded-claim test**

Read the existing `engine_test.go` fakes first (`fakeCatalog`/store/miniredis). Add a test that seeds the interest fetch so a fresh ongoing outranks a high-visitor released title, and that an all-idle claim advances the cursor. Because the existing test harness already stands up an Engine, adapt its constructor call. Add:

```go
func TestBandedClaimPrefersOngoing(t *testing.T) {
	// Build an Engine whose interest fetch returns one ongoing with pending
	// work and one released+watched title, both with pending units. With
	// weights favouring Band 1, the first claim must be the ongoing's unit.
	// (Mirror the existing engine_test harness: fake catalog client returning
	// a fixed *catalogclient.Interest, in-memory store with no prior verdicts,
	// miniredis signals. Set rng to always 0.0 so primary band = Band1.)
	// ... harness wiring per existing engine_test.go ...
}
```

Because the exact harness shape is in `engine_test.go`, the implementer writes this test against that harness. Minimum assertions:
1. With `rng=()=>0.0` and default weights, first `Claim` returns the ongoing candidate's unit (assert `unit.AnimeID == "ong1"`).
2. After all non-idle candidates are settled+cooled, a claim with `rng=()=>0.99` fetches the idle window and calls `AdvanceIdleCursor` (assert the Redis `cv:idle:cursor` moved).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/content-verify/internal/queue/ -run TestBandedClaimPrefersOngoing`
Expected: FAIL — `NewEngine` arity / `BuildCandidates` signature mismatch.

- [ ] **Step 3: Rewrite `BuildCandidates` (queue.go)**

Replace the existing `BuildCandidates` (`queue.go:53-89`) with one that consumes `*catalogclient.Interest`:

```go
// BuildCandidates merges the interest snapshot (ongoing ∪ top ∪ planned ∪
// idle-window ∪ visited ∪ pinned) and attaches the unique-visitor count. Rows
// carry their band-relevant signals (top_rank, next_episode_at, planners,
// score). Pinned titles are injected even when in no bucket.
func BuildCandidates(it *catalogclient.Interest, visited []string, pins map[string]string, visitors func(string) int) []Candidate {
	byID := map[string]*Candidate{}
	get := func(id string) *Candidate {
		c, ok := byID[id]
		if !ok {
			c = &Candidate{AnimeID: id}
			byID[id] = c
		}
		return c
	}
	setName := func(c *Candidate, n string) {
		if c.Name == "" {
			c.Name = n
		}
	}
	if it != nil {
		for _, r := range it.Ongoing {
			c := get(r.ID)
			setName(c, r.Name)
			c.Ongoing = true
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			c.MalScore = r.Score
			c.NextEpisodeAt = r.NextEpisodeAt
		}
		for _, r := range it.Top {
			c := get(r.ID)
			setName(c, r.Name)
			c.Top = true
			c.TopRank = r.TopRank
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			if r.Score > 0 {
				c.MalScore = r.Score
			}
		}
		for _, r := range it.Planned {
			c := get(r.ID)
			setName(c, r.Name)
			c.Idle = true
			c.Planners = r.Planners
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			if r.Score > 0 {
				c.MalScore = r.Score
			}
		}
		for _, r := range it.IdleWindow {
			c := get(r.ID)
			setName(c, r.Name)
			c.Idle = true
			if c.TopRank == 0 {
				c.TopRank = r.TopRank
			}
			c.EpisodesAired = maxInt(c.EpisodesAired, r.EpisodesAired)
			if r.Score > 0 {
				c.MalScore = r.Score
			}
		}
	}
	for _, id := range visited {
		get(id)
	}
	for id := range pins {
		get(id).Pinned = true
	}
	out := make([]Candidate, 0, len(byID))
	for _, c := range byID {
		c.Visitors = visitors(c.AnimeID)
		out = append(out, *c)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

Ensure `catalogclient` is imported in queue.go (it already is).

- [ ] **Step 4: Rewrite the engine claim path (engine.go)**

Add Engine fields and constructor params:

```go
type Engine struct {
	// ... existing fields ...
	weights      [3]int
	freshWindow  time.Duration
	idleCooldown time.Duration
	idleWindow   int
	rng          func() float64 // injectable for tests; default rand.Float64
	// replace `memb *catalogclient.Membership` / `membAt` with:
	interestCache *catalogclient.Interest
	interestAt    time.Time
}
```

Update `NewEngine`:

```go
func NewEngine(cat *catalogclient.Client, sig *signals.Signals, store *repo.Store, reprobeTTL time.Duration, skipEnabled bool, pins map[string]string, weights [3]int, freshWindow, idleCooldown time.Duration, idleWindow int, log *logger.Logger) *Engine {
	if pins == nil {
		pins = map[string]string{}
	}
	return &Engine{
		cat: cat, sig: sig, store: store, reprobeTTL: reprobeTTL, skipEnabled: skipEnabled, pins: pins,
		weights: weights, freshWindow: freshWindow, idleCooldown: idleCooldown, idleWindow: idleWindow,
		rng: rand.Float64, log: log, now: time.Now,
		enumCache:     map[string]enumEntry{},
		aniskipCache:  map[string]aniskipEntry{},
		malIDs:        map[string]string{},
		inflightUnits: map[string]struct{}{},
		inflightProv:  map[string]struct{}{},
	}
}
```

Import `math/rand`.

Replace `membership()` with `interest()`, fetching at the current idle cursor and advancing it on each fresh fetch:

```go
// interest fetches the banded interest snapshot behind membershipTTL. On a
// fresh fetch it advances the idle sweep cursor by idleWindow (wrapping at
// idle_total) so successive refreshes walk the catalog tail. The HTTP call
// runs WITHOUT holding mu (same reasoning as the old membership()).
func (e *Engine) interest(ctx context.Context) *catalogclient.Interest {
	e.mu.Lock()
	if e.interestCache != nil && e.now().Sub(e.interestAt) < membershipTTL {
		it := e.interestCache
		e.mu.Unlock()
		return it
	}
	e.mu.Unlock()

	offset := e.sig.IdleCursor(ctx)
	it, err := e.cat.InterestBands(ctx, offset, e.idleWindow)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("interest fetch failed; reusing stale", "error", err)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.interestCache // possibly nil — BuildCandidates tolerates it
	}
	e.sig.AdvanceIdleCursor(ctx, e.idleWindow, it.IdleTotal)
	e.mu.Lock()
	e.interestCache, e.interestAt = it, e.now()
	e.mu.Unlock()
	return it
}
```

Replace `ranked()` with `bandedCandidates()`:

```go
// bandedCandidates returns candidates concatenated in this claim's band
// try-order (pins, lottery-primary band, then the rest), each band's slice
// intra-sorted. The lottery gives the weighting; fall-through means an empty
// higher band never wastes the tick.
func (e *Engine) bandedCandidates(ctx context.Context) []Candidate {
	it := e.interest(ctx)
	visited := e.sig.VisitedAnime(ctx)
	all := BuildCandidates(it, visited, e.pins, func(id string) int { return e.sig.UniqueVisitors(ctx, id) })
	cvmetrics.QueueDepth.Set(float64(len(all)))

	groups := map[Band][]Candidate{}
	for _, c := range all {
		b := BandOf(c)
		groups[b] = append(groups[b], c)
	}
	now := e.now()
	for b := range groups {
		g := groups[b]
		sort.SliceStable(g, func(i, j int) bool { return IntraLess(g[i], g[j], now, e.freshWindow) })
		groups[b] = g
	}
	order := bandOrder(e.weights, e.rng())
	out := make([]Candidate, 0, len(all))
	for _, b := range order {
		out = append(out, groups[b]...)
	}
	return out
}
```

In `Claim`, change the candidate source and the cooldown TTL:
- Replace `for _, cand := range e.ranked(ctx) {` with `for _, cand := range e.bandedCandidates(ctx) {`.
- Replace the settle line `e.sig.SetCooldown(ctx, cand.AnimeID, CooldownTTL(cand.Ongoing))` with `e.sig.SetCooldown(ctx, cand.AnimeID, CooldownTTL(BandOf(cand), e.idleCooldown))`.

Everything else in `Claim` (cooldown gate, enumerate, PendingUnits, skip lane, leases) stays byte-for-byte.

Update `QueueEntry`/`Snapshot` — `Score()` is gone; expose band instead:

```go
type QueueEntry struct {
	AnimeID  string `json:"anime_id"`
	Name     string `json:"name"`
	Band     int    `json:"band"`
	Ongoing  bool   `json:"ongoing"`
	Top      bool   `json:"top"`
	Visitors int    `json:"visitors"`
	Cooling  bool   `json:"cooling"`
}

func (e *Engine) Snapshot(ctx context.Context, limit int) []QueueEntry {
	out := []QueueEntry{}
	for i, c := range e.bandedCandidates(ctx) {
		if i >= limit {
			break
		}
		out = append(out, QueueEntry{AnimeID: c.AnimeID, Name: c.Name, Band: int(BandOf(c)),
			Ongoing: c.Ongoing, Top: c.Top, Visitors: c.Visitors,
			Cooling: e.sig.InCooldown(ctx, c.AnimeID)})
	}
	return out
}
```

- [ ] **Step 5: Update the existing engine_test harness**

Update every `NewEngine(...)` call in `engine_test.go` to the new arity (add `weights`, `freshWindow`, `idleCooldown`, `idleWindow`). Set `rng` on the returned engine where determinism is needed (`eng.rng = func() float64 { return 0 }`). Point the fake catalog client at an `*catalogclient.Interest` instead of `*catalogclient.Membership` (the fake must now answer `InterestBands`). If the fake is an interface, add the method; if it's the real `*catalogclient.Client` pointed at an httptest server, update the server to serve `/internal/interest/bands`.

- [ ] **Step 6: Run the full queue package tests**

Run: `go test ./services/content-verify/internal/queue/...`
Expected: PASS (band tests from Task 5 + banded-claim + existing enumerate/skip/aniskip tests).

- [ ] **Step 7: Commit**

```bash
git add services/content-verify/internal/queue/engine.go services/content-verify/internal/queue/queue.go services/content-verify/internal/queue/engine_test.go
git commit -m "feat(content-verify): banded weighted claim + idle round-robin sweep"
```

---

## Task 8: Wire config → engine, env docs, integration verify

**Files:**
- Modify: `services/content-verify/cmd/content-verify-api/main.go`
- Modify: `docs/environment-variables.md`

**Interfaces:**
- Consumes: `config` knobs (Task 3), `NewEngine` new arity (Task 7).

- [ ] **Step 1: Update main.go**

In `services/content-verify/cmd/content-verify-api/main.go`, find the `queue.NewEngine(...)` call and update it to pass the new config:

```go
engine := queue.NewEngine(cat, sig, store, cfg.ReprobeTTL, cfg.SkipEnabled, cfg.Pins,
	cfg.BandWeights, cfg.FreshWindow, cfg.IdleCooldown, cfg.IdleWindow, log)
```

Remove any reference to `cfg.TopLimit` (deleted in Task 3).

- [ ] **Step 2: Build the whole service**

Run: `go build ./services/content-verify/...`
Expected: no errors.

- [ ] **Step 3: Run the whole service test suite**

Run: `go test ./services/content-verify/...`
Expected: PASS.

- [ ] **Step 4: Document env knobs**

In `docs/environment-variables.md`, in the content-verify section, add:

```
- `CV_BAND_WEIGHTS` (default `60,30,10`) — per-claim band lottery weights [ongoing, watched+top, idle-backfill].
- `CV_FRESH_WINDOW` (default `48h`) — ± window on `next_episode_at` that floats a just-aired ongoing to the front of Band 1.
- `CV_IDLE_COOLDOWN` (default `168h`) — settled-title cooldown for the idle-backfill band (long, so the tail doesn't re-spin).
- `CV_IDLE_WINDOW` (default `100`) — idle-sweep page size over the non-ongoing catalog tail.
- (removed) `CV_TOP_LIMIT` — was never wired; the top-100 cutoff lives in the catalog endpoint's `top_limit` default.
```

- [ ] **Step 5: Commit**

```bash
git add services/content-verify/cmd/content-verify-api/main.go docs/environment-variables.md
git commit -m "feat(content-verify): wire band config into engine; document CV_BAND_* env"
```

---

## Post-plan verification (run after all tasks)

```bash
cd /data/animeenigma/.claude/worktrees/content-verify-probing
go build ./services/catalog/... ./services/content-verify/...
go test ./services/catalog/internal/repo/ ./services/catalog/internal/handler/ ./services/content-verify/...
gofmt -l services/catalog/internal/repo/anime.go services/catalog/internal/handler/internal_interest.go services/content-verify/internal/queue/queue.go services/content-verify/internal/queue/engine.go services/content-verify/internal/signals/signals.go services/content-verify/internal/config/config.go services/content-verify/internal/catalogclient/client.go
```

Expected: builds clean, all tests pass, `gofmt -l` prints nothing for the touched files (fix by hand if it flags — never `gofmt -w`). Then `/animeenigma-after-update` (redeploys `catalog` + `content-verify`, changelog, push). Live smoke: after deploy, `curl -s http://localhost:8081/internal/interest/bands?idle_offset=100 | python3 -m json.tool | head -40` and confirm ongoing/top/planned/idle_window/idle_total are populated; `curl -s http://localhost:8101/queue?limit=20` (or the existing snapshot route — grep the cv router) to confirm the queue now shows a `band` field with ongoings leading.

---

## Self-Review notes (author)

- **Spec coverage:** §1 bands → Task 5 `BandOf`; §2 endpoint → Tasks 1-2; §3.1 band assignment/IntraScore → Task 5 `IntraLess` + freshBoost; §3.2 weighted claim + fall-through → Task 5 `weightedPick`/`bandOrder` + Task 7 `bandedCandidates`; §3.3 idle cursor → Task 6 + Task 7 `interest()`; §3.4 env knobs → Task 3 + Task 8; kill dead `CV_TOP_LIMIT` → Task 3.
- **Type consistency:** `Candidate` fields (`MalScore`, `TopRank`, `NextEpisodeAt`, `Planners`, `Idle`) defined in Task 5, populated in Task 7 `BuildCandidates`, consumed by `IntraLess`/`BandOf`. `catalogclient.Interest`/`InterestRow` (Task 4) mirror `repo.InterestRow`/`InterestBands` (Task 1) field-for-field. `NewEngine` arity change (Task 7) reconciled in `main.go` (Task 8).
- **Cross-task compile ordering:** Task 5 removes `Score()`/`Rank()` which `engine.go` still calls until Task 7 — the queue package won't fully build between Tasks 5 and 7. The reviewer/implementer runs Task 5's band tests with `-run` (they compile in isolation via the new file) and expects a full green only after Task 7. Flagged in Task 5 Step 5.
