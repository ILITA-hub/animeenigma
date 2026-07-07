# Source Panel Phase B — Playability Index + Per-Title Promotion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rank the Source panel's `degraded` bucket by a real, decayed **playability index** (blended from watch success + probe-up signals + provider health) instead of by name/order, and **promote** a per-title-proven degraded provider to a normal-mode selectable source for that one title.

**Architecture:** Analytics owns the ClickHouse data and exposes a new Docker-network-only read endpoint `GET /internal/playability?anime_id=<id>` returning per-provider decayed weights. Catalog is the assembler: a best-effort read client fetches those weights during capability-report assembly (cached alongside the report's 10-min TTL), blends them with the provider's live health into a `playability_index` attached to each `ProviderCap`, and — when this-title watch evidence crosses `PromoteFloor` — flips a degraded provider to active/selectable for that title. Frontend adds `playability_index` to its types and re-tiebreaks the `degraded` bucket by it. Everything degrades gracefully: an analytics outage yields a health-only index and zero promotions, and never blocks the feed.

**Tech Stack:** Go 1.x (analytics `services/analytics`, catalog `services/catalog`), ClickHouse (`clickhouse-go/v2`), Prometheus (`libs/metrics`, `promauto`), Vue 3 + TypeScript + Vitest (`frontend/web`).

## Global Constraints

- **Provider identity is the capability `provider` id** (`ae`, `kodik`, `allanime-okru`, `gogoanime`, `miruro`, `nineanime`, `animekai`, `animelib`, `hanime`, `18anime`, `animejoy-sibnet`, `animejoy-allvideo`). The catalog `ProviderCap.Provider` field is authoritative. All blending matches analytics scores to caps by this id.
- **Naming divergence that MUST be handled:** the FE player-events telemetry sends `provider = combo.provider` (= capability id), so `events.target` uses capability ids — but the analytics **ingestion whitelist** (`knownProviders`) is stale and silently drops `allanime-okru` / `animejoy-sibnet` / `animejoy-allvideo` today. And `probe_runs.provider` uses the **probe roster** name `kodik-noads` where the capability id is `kodik`. Task 1 fixes the whitelist; Task 5's client applies the `kodik-noads → kodik` alias. No other divergence exists.
- **Decay:** exponential, `weight = exp(-dateDiff('day', <ts>, now()) / 14.0)` (τ ≈ 14 days). **No hard threshold** anywhere except the single `PromoteFloor` boundary that turns the score into a binary state flip.
- **Best-effort / never-block:** the catalog→analytics read is best-effort with a short timeout; ANY failure (timeout, non-200, decode error, disabled flag) returns an empty scores map → health-only index, zero promotions, feed unaffected. Mirror the existing `libs/tracing` producer posture (swallow errors, never propagate).
- **`/internal/playability` is Docker-network-only** — registered ONLY on the analytics service router, NEVER added to the gateway router (same posture as `/internal/effects`).
- **Kill switch:** catalog env `PLAYABILITY_INDEX_ENABLED` (default `true`, read via `os.Getenv` like `ANALYTICS_INTERNAL_URL`). When false: skip the fetch entirely → identical to pre-Phase-B behavior.
- **Priority ordering (from the owner):** recent **this-specific-anime** watch success is the top-priority signal; then recent probe-up + provider health; then global watch. Weights encode this (`WThisAnime` dominant).
- **Effort metrics** (`.planning/CONVENTIONS.md`): no time units. This phase: `UXΔ = +2 (Better)` (degraded sources now ranked by real playability; a proven source auto-surfaces per title) · `CDI = 0.04 * 34` (three services, one new endpoint, one new read client, pure-function blend) · `MVQ = Griffin 85%/80%`.

---

## File Structure

**Analytics (`services/analytics/`):**
- `internal/handler/playertelemetry_whitelist.go` — MODIFY: expand `knownProviders` to the current roster (union of capability + probe names). Reused as the scores-query roster filter.
- `internal/repo/clickhouse_store.go` — MODIFY: add free query fns `QueryPlayabilityWatch` + `QueryPlayabilityProbeUp` (mirror `QueryProviderReliability` idiom).
- `internal/service/playability.go` — CREATE: pure `BuildPlayabilityScores(watchRows, probeRows, roster)` merge/filter/shape → response DTO. Fully unit-testable.
- `internal/service/playability_test.go` — CREATE: empty-anime, unknown-provider-filtered, merge cases.
- `internal/handler/playability.go` — CREATE: `PlayabilityHandler` (reads `?anime_id=`, calls store + service, writes JSON).
- `internal/transport/router.go` — MODIFY: register `GET /internal/playability` with nil-guard; add handler to `NewRouter` signature.
- `cmd/analytics-api/main.go` — MODIFY: build `PlayabilityHandler` behind `chConn != nil`, pass to `NewRouter`.

**Catalog (`services/catalog/`):**
- `internal/domain/capability.go` — MODIFY: add `PlayabilityIndex float64` json `playability_index,omitempty` to `ProviderCap`.
- `internal/service/capability/analytics_client.go` — CREATE: best-effort `playabilityClient` (mirror `libs/tracing` producer posture), `kodik-noads → kodik` alias-merge.
- `internal/service/capability/playability.go` — CREATE: tunable weight constants, `blendData` (ctx-carried scores + normalization maxima), `indexFor`, `thisAnimeWatch`, ctx helpers.
- `internal/service/capability/playability_test.go` — CREATE: norm/blend/health-term/promotion unit tests.
- `internal/service/capability/providerview.go` — MODIFY: `deriveProviderView` gains `thisAnimeWatch`, `promoteFloor` params + promotion short-circuit.
- `internal/service/capability/families_ru.go` — MODIFY: `applyFeedFields` gains `ctx`; computes+attaches `PlayabilityIndex`, threads promotion signal into `deriveProviderView`.
- `internal/service/capability/service.go` — MODIFY: `Service` gains a `playability PlayabilitySource` dep; `buildFamilies` fetches scores once, seeds `blendData` into ctx; update `applyFeedFields` call sites (8) to pass `ctx`.
- `internal/service/capability/*_test.go` — MODIFY: update `applyFeedFields`/`deriveProviderView` call sites in existing tests.
- `cmd/catalog-api/main.go` — MODIFY: build the `playabilityClient` from `ANALYTICS_INTERNAL_URL` + `PLAYABILITY_INDEX_ENABLED`, inject into the capability `Service`.
- `libs/metrics/*.go` — MODIFY (or new file `libs/metrics/playability.go`): define `CapabilityPlayabilityPromotionsTotal` + `CapabilityPlayabilityFetchFailuresTotal` counters.

**Frontend (`frontend/web/`):**
- `src/types/capabilities.ts` — MODIFY: add `playability_index?: number` to `ProviderCap`.
- `src/types/aePlayer.ts` — MODIFY: add `playability_index?: number` to `ProviderRow`.
- `src/composables/aePlayer/useProviderFeed.ts` — MODIFY: map `playability_index` in `toRow`.
- `src/components/player/aePlayer/SourcePanel.vue` — MODIFY: re-tiebreak the `degraded` bucket by `playability_index`.
- `src/components/player/aePlayer/SourcePanel.spec.ts` — MODIFY: add degraded-bucket index-sort test; extend row factory.

---

## Task 1: Analytics — reconcile the ingestion/roster whitelist

**Files:**
- Modify: `services/analytics/internal/handler/playertelemetry_whitelist.go`
- Test: `services/analytics/internal/handler/playertelemetry_whitelist_test.go` (create if absent)

**Interfaces:**
- Consumes: nothing.
- Produces: `knownProviders map[string]struct{}` (package `handler`) and `whitelistProvider(string) string` — unchanged signatures; roster expanded. Task 2's roster filter reuses `knownProviders`.

**Context:** The FE emits `provider = combo.provider` (capability ids) on player-events. The current roster silently drops `allanime-okru`, `animejoy-sibnet`, `animejoy-allvideo` at ingest, so watch telemetry for those providers never lands in `events.target`. This is a real pre-existing gap; Phase B needs those rows. Also add `kodik-noads` (the probe roster name) so the same map can serve as the scores-query roster filter across BOTH data sources. This fix is **forward-only** — historical dropped events cannot be recovered; the decaying index simply populates over the following days.

- [ ] **Step 1: Write the failing test**

Create/append `services/analytics/internal/handler/playertelemetry_whitelist_test.go`:

```go
package handler

import "testing"

func TestWhitelistProvider_CurrentRoster(t *testing.T) {
	// Capability ids the FE actually sends on player-events, plus the probe roster name.
	for _, p := range []string{
		"allanime-okru", "animejoy-sibnet", "animejoy-allvideo", "kodik-noads",
		"gogoanime", "miruro", "nineanime", "animekai", "kodik", "animelib", "hanime", "ae", "18anime",
	} {
		if got := whitelistProvider(p); got != p {
			t.Errorf("whitelistProvider(%q) = %q, want %q (must be in roster)", p, got, p)
		}
	}
	if got := whitelistProvider("  AllAnime-OKRU "); got != "allanime-okru" {
		t.Errorf("whitelistProvider trims+lowercases: got %q", got)
	}
	if got := whitelistProvider("evil-injected"); got != "" {
		t.Errorf("unknown provider must be dropped, got %q", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/analytics && go test ./internal/handler/ -run TestWhitelistProvider_CurrentRoster -v`
Expected: FAIL — `allanime-okru`, `animejoy-*`, `kodik-noads` not in roster → returns `""`.

- [ ] **Step 3: Expand the roster**

In `playertelemetry_whitelist.go`, replace the `knownProviders` map literal with (keep existing tombstones so legacy events still record):

```go
// knownProviders is the canonical stream-source roster. It gates player-telemetry
// ingestion (whitelistProvider) AND doubles as the playability scores-query roster
// filter (see service/playability.go). It is the UNION of:
//   - capability provider ids the FE sends as combo.provider on player-events
//     (these land in events.target), and
//   - probe_runs.provider names (PROBE_PROVIDERS), which use "kodik-noads" where
//     the capability id is "kodik".
// Keep in sync when a provider is added/removed.
var knownProviders = map[string]struct{}{
	// EN chain
	"gogoanime": {}, "allanime-okru": {}, "miruro": {}, "nineanime": {}, "animekai": {},
	// RU / adult / first-party / per-title
	"kodik": {}, "kodik-noads": {}, "animelib": {}, "hanime": {}, "ae": {}, "18anime": {},
	"animejoy-sibnet": {}, "animejoy-allvideo": {},
	// disabled tombstones (kept so any residual/legacy events still record)
	"allanime": {}, "animepahe": {}, "animefever": {},
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/analytics && go test ./internal/handler/ -run TestWhitelistProvider_CurrentRoster -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/analytics/internal/handler/playertelemetry_whitelist.go services/analytics/internal/handler/playertelemetry_whitelist_test.go
git commit -m "fix(analytics): reconcile provider whitelist to current roster (allanime-okru, animejoy legs, kodik-noads)"
```

---

## Task 2: Analytics — ClickHouse playability queries

**Files:**
- Modify: `services/analytics/internal/repo/clickhouse_store.go`
- Test: `services/analytics/internal/repo/playability_query_test.go` (create — string-shape assertions only; no live CH)

**Interfaces:**
- Consumes: `driver.Conn` (existing CH connection).
- Produces:
  - `type PlayabilityWatchRow struct { Provider string; ThisAnimeWatch, GlobalWatch float64 }`
  - `type PlayabilityProbeRow struct { Provider string; RecentUp float64 }`
  - `func QueryPlayabilityWatch(ctx context.Context, conn driver.Conn, animeID string) ([]PlayabilityWatchRow, error)`
  - `func QueryPlayabilityProbeUp(ctx context.Context, conn driver.Conn) ([]PlayabilityProbeRow, error)`

**Context:** Mirror the existing free-function query idiom (`QueryProviderReliability`, `QueryReadThresholdP95`): `?`-parameterized `conn.Query` → `defer rows.Close()` → `rows.Next()/Scan` → `rows.Err()`. `events.properties` is a `String`, so use `JSONExtractBool(properties,'reached_playback')`. The provider dimension for watch events is the `target` column (that's where whitelisted player-event providers land). Both queries bound the scan to a 60-day window (`exp(-60/14) ≈ 0.014`, negligible tail). `anime_id` is `Nullable(String)`; `anime_id = ?` yields 0 for the global term on null/mismatch rows, so ONE query produces both watch terms via `sumIf`.

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/repo/playability_query_test.go`:

```go
package repo

import (
	"strings"
	"testing"
)

// The query text is a package-level const so it is testable without a live CH.
func TestPlayabilityWatchQuery_Shape(t *testing.T) {
	q := playabilityWatchQuery
	for _, want := range []string{
		"effect_kind = 'player_resolve'",
		"JSONExtractBool(properties,'reached_playback')",
		"exp(-dateDiff('day', timestamp, now()) / 14.0)",
		"target AS provider",
		"anime_id = ?",       // this_anime_watch term binds the anime id
		"GROUP BY target",
		"INTERVAL 60 DAY",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("watch query missing %q\n---\n%s", want, q)
		}
	}
}

func TestPlayabilityProbeQuery_Shape(t *testing.T) {
	q := playabilityProbeQuery
	for _, want := range []string{
		"FROM probe_runs",
		"playable = 1",
		"exp(-dateDiff('day', run_ts, now()) / 14.0)",
		"GROUP BY provider",
		"INTERVAL 60 DAY",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("probe query missing %q\n---\n%s", want, q)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/analytics && go test ./internal/repo/ -run TestPlayability -v`
Expected: FAIL — `playabilityWatchQuery` / `playabilityProbeQuery` undefined.

- [ ] **Step 3: Add the query consts + functions**

Append to `services/analytics/internal/repo/clickhouse_store.go` (after `QueryProviderReliability`):

```go
// PlayabilityWatchRow is one provider's decayed watch-success weights.
type PlayabilityWatchRow struct {
	Provider       string
	ThisAnimeWatch float64
	GlobalWatch    float64
}

// PlayabilityProbeRow is one provider's decayed recent-up weight.
type PlayabilityProbeRow struct {
	Provider string
	RecentUp float64
}

// playabilityWatchQuery weights each successful player_resolve by exp-decay
// (tau=14d) and splits global vs this-anime via sumIf on the Nullable anime_id.
// Provider dimension is `target` (where whitelisted player-event providers land).
const playabilityWatchQuery = `SELECT target AS provider,
       sum(exp(-dateDiff('day', timestamp, now()) / 14.0)) AS global_watch,
       sumIf(exp(-dateDiff('day', timestamp, now()) / 14.0), anime_id = ?) AS this_anime_watch
FROM events
WHERE effect_kind = 'player_resolve'
  AND JSONExtractBool(properties,'reached_playback')
  AND target != ''
  AND timestamp >= now() - INTERVAL 60 DAY
GROUP BY target`

// playabilityProbeQuery weights each playable probe run by exp-decay (tau=14d).
const playabilityProbeQuery = `SELECT provider,
       sum(exp(-dateDiff('day', run_ts, now()) / 14.0)) AS recent_up
FROM probe_runs
WHERE playable = 1
  AND run_ts >= now() - INTERVAL 60 DAY
GROUP BY provider`

// QueryPlayabilityWatch returns per-provider global + this-anime decayed watch weights.
func QueryPlayabilityWatch(ctx context.Context, conn driver.Conn, animeID string) ([]PlayabilityWatchRow, error) {
	rows, err := conn.Query(ctx, playabilityWatchQuery, animeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayabilityWatchRow
	for rows.Next() {
		var r PlayabilityWatchRow
		if err := rows.Scan(&r.Provider, &r.GlobalWatch, &r.ThisAnimeWatch); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryPlayabilityProbeUp returns per-provider decayed recent-up weights.
func QueryPlayabilityProbeUp(ctx context.Context, conn driver.Conn) ([]PlayabilityProbeRow, error) {
	rows, err := conn.Query(ctx, playabilityProbeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayabilityProbeRow
	for rows.Next() {
		var r PlayabilityProbeRow
		if err := rows.Scan(&r.Provider, &r.RecentUp); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes + build**

Run: `cd services/analytics && go test ./internal/repo/ -run TestPlayability -v && go build ./...`
Expected: PASS; build clean.

- [ ] **Step 5: Commit**

```bash
git add services/analytics/internal/repo/clickhouse_store.go services/analytics/internal/repo/playability_query_test.go
git commit -m "feat(analytics): ClickHouse playability queries (decayed watch + probe-up per provider)"
```

---

## Task 3: Analytics — scores shaping service + endpoint + wiring

**Files:**
- Create: `services/analytics/internal/service/playability.go`
- Create: `services/analytics/internal/service/playability_test.go`
- Create: `services/analytics/internal/handler/playability.go`
- Modify: `services/analytics/internal/transport/router.go`
- Modify: `services/analytics/cmd/analytics-api/main.go`

**Interfaces:**
- Consumes: `repo.PlayabilityWatchRow`, `repo.PlayabilityProbeRow`, `handler.knownProviders` (roster).
- Produces:
  - `service.PlayabilityScores` = `map[string]service.ProviderScore` where `ProviderScore{ ThisAnimeWatch, GlobalWatch, RecentUp float64 }` with json tags `this_anime_watch` / `global_watch` / `recent_up`.
  - `service.BuildPlayabilityScores(watch []repo.PlayabilityWatchRow, probe []repo.PlayabilityProbeRow, roster func(string) bool) PlayabilityScores` — pure merge/filter/shape.
  - `handler.NewPlayabilityHandler(conn driver.Conn) *PlayabilityHandler` serving `GET /internal/playability?anime_id=<id>` → `{"providers": {...}}`.

**Context:** Keep CH access in the handler (call the two repo query fns), keep the merge/filter/shape pure in `service` so it is unit-testable without CH. The roster filter reuses the Task-1 `knownProviders` — but `service` must not import `handler` (avoid a cycle); pass roster membership as a `func(string) bool` closure built in the handler. Mirror the read-side handler posture of `player_ranking.go` (60s ctx timeout, 500 on error). Register on the analytics router ONLY (Docker-network-only); do NOT touch the gateway.

- [ ] **Step 1: Write the failing test (pure shaping)**

Create `services/analytics/internal/service/playability_test.go`:

```go
package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

func allow(known ...string) func(string) bool {
	m := map[string]struct{}{}
	for _, k := range known {
		m[k] = struct{}{}
	}
	return func(p string) bool { _, ok := m[p]; return ok }
}

func TestBuildPlayabilityScores_MergesAndFilters(t *testing.T) {
	watch := []repo.PlayabilityWatchRow{
		{Provider: "allanime-okru", GlobalWatch: 41.3, ThisAnimeWatch: 2.7},
		{Provider: "evil", GlobalWatch: 9, ThisAnimeWatch: 9}, // not in roster → dropped
	}
	probe := []repo.PlayabilityProbeRow{
		{Provider: "allanime-okru", RecentUp: 6.1},
		{Provider: "kodik-noads", RecentUp: 4.0}, // in roster, no watch rows → probe-only entry
	}
	got := BuildPlayabilityScores(watch, probe, allow("allanime-okru", "kodik-noads"))

	if _, ok := got["evil"]; ok {
		t.Fatal("unroster provider must be filtered out")
	}
	aa := got["allanime-okru"]
	if aa.GlobalWatch != 41.3 || aa.ThisAnimeWatch != 2.7 || aa.RecentUp != 6.1 {
		t.Errorf("allanime-okru merge wrong: %+v", aa)
	}
	kn := got["kodik-noads"]
	if kn.RecentUp != 4.0 || kn.GlobalWatch != 0 || kn.ThisAnimeWatch != 0 {
		t.Errorf("probe-only provider wrong: %+v", kn)
	}
}

func TestBuildPlayabilityScores_Empty(t *testing.T) {
	got := BuildPlayabilityScores(nil, nil, allow("ae"))
	if len(got) != 0 {
		t.Errorf("empty inputs → empty map, got %d entries", len(got))
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/analytics && go test ./internal/service/ -run TestBuildPlayabilityScores -v`
Expected: FAIL — `BuildPlayabilityScores` undefined.

- [ ] **Step 3: Write the service (pure shaping)**

Create `services/analytics/internal/service/playability.go`:

```go
package service

import "github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"

// ProviderScore is one provider's decayed playability weights (JSON wire shape).
type ProviderScore struct {
	ThisAnimeWatch float64 `json:"this_anime_watch"`
	GlobalWatch    float64 `json:"global_watch"`
	RecentUp       float64 `json:"recent_up"`
}

// PlayabilityScores maps provider id → decayed weights.
type PlayabilityScores map[string]ProviderScore

// BuildPlayabilityScores merges the watch and probe query rows into one
// per-provider map, filtered to the known roster. Pure — no CH access.
func BuildPlayabilityScores(watch []repo.PlayabilityWatchRow, probe []repo.PlayabilityProbeRow, inRoster func(string) bool) PlayabilityScores {
	out := PlayabilityScores{}
	for _, w := range watch {
		if w.Provider == "" || !inRoster(w.Provider) {
			continue
		}
		s := out[w.Provider]
		s.ThisAnimeWatch = w.ThisAnimeWatch
		s.GlobalWatch = w.GlobalWatch
		out[w.Provider] = s
	}
	for _, p := range probe {
		if p.Provider == "" || !inRoster(p.Provider) {
			continue
		}
		s := out[p.Provider]
		s.RecentUp = p.RecentUp
		out[p.Provider] = s
	}
	return out
}
```

- [ ] **Step 4: Run service test to verify it passes**

Run: `cd services/analytics && go test ./internal/service/ -run TestBuildPlayabilityScores -v`
Expected: PASS.

- [ ] **Step 5: Write the handler**

Create `services/analytics/internal/handler/playability.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
)

// PlayabilityHandler serves GET /internal/playability?anime_id=<id> with
// per-provider decayed watch/probe weights. Docker-network-only.
type PlayabilityHandler struct {
	conn chdriver.Conn
}

// NewPlayabilityHandler builds the handler over the shared CH connection.
func NewPlayabilityHandler(conn chdriver.Conn) *PlayabilityHandler {
	return &PlayabilityHandler{conn: conn}
}

type playabilityResponse struct {
	Providers service.PlayabilityScores `json:"providers"`
}

func (h *PlayabilityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	animeID := r.URL.Query().Get("anime_id")

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	watch, err := repo.QueryPlayabilityWatch(ctx, h.conn, animeID)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	probe, err := repo.QueryPlayabilityProbeUp(ctx, h.conn)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	scores := service.BuildPlayabilityScores(watch, probe, func(p string) bool {
		_, ok := knownProviders[p]
		return ok
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(playabilityResponse{Providers: scores})
}
```

- [ ] **Step 6: Register the route (nil-guarded) + extend NewRouter**

In `services/analytics/internal/transport/router.go`: add a `*handler.PlayabilityHandler` parameter to `NewRouter` (place it after `probe` in the signature to match main.go ordering), and beside the other `/internal/*` registrations add:

```go
	if playability != nil {
		r.Get("/internal/playability", playability.ServeHTTP)
	}
```

(Match the existing nil-guard style used for `readThresholds`/`playerRanking`/etc.)

- [ ] **Step 7: Wire it in main.go (behind chConn != nil)**

In `services/analytics/cmd/analytics-api/main.go`, alongside the other `chConn != nil` read-side handlers, add:

```go
	var playabilityHandler *handler.PlayabilityHandler
	if chConn != nil {
		playabilityHandler = handler.NewPlayabilityHandler(chConn)
	}
```

and pass `playabilityHandler` into the `transport.NewRouter(...)` call in the position matching the Step-6 signature.

- [ ] **Step 8: Build the whole service**

Run: `cd services/analytics && go build ./... && go vet ./internal/handler/ ./internal/service/ ./internal/transport/`
Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add services/analytics/internal/service/playability.go services/analytics/internal/service/playability_test.go services/analytics/internal/handler/playability.go services/analytics/internal/transport/router.go services/analytics/cmd/analytics-api/main.go
git commit -m "feat(analytics): /internal/playability endpoint (Docker-network-only per-provider scores)"
```

---

## Task 4: Catalog — metrics + `playability_index` wire field

**Files:**
- Create: `libs/metrics/playability.go`
- Modify: `services/catalog/internal/domain/capability.go`

**Interfaces:**
- Produces: `metrics.CapabilityPlayabilityPromotionsTotal prometheus.Counter`, `metrics.CapabilityPlayabilityFetchFailuresTotal prometheus.Counter`; `domain.ProviderCap.PlayabilityIndex float64` (json `playability_index,omitempty`).

**Context:** All catalog metrics live in `libs/metrics` via `promauto` (auto-registered on the default registry). `promauto.NewCounter` with no labels matches the `WatchProgressSavesTotal` precedent. Adding a field to `ProviderCap` is a pure struct change — GORM is not involved (this is a wire DTO, not a table).

- [ ] **Step 1: Add the metrics**

Create `libs/metrics/playability.go`:

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CapabilityPlayabilityPromotionsTotal counts per-title promotions of a
	// degraded provider to active/selectable (Phase B B3/B4).
	CapabilityPlayabilityPromotionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "capability_playability_promotions_total",
		Help: "Total per-title promotions of a degraded provider to selectable via playability index",
	})

	// CapabilityPlayabilityFetchFailuresTotal counts best-effort analytics
	// playability-scores fetches that failed (fed a health-only index).
	CapabilityPlayabilityFetchFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "capability_playability_fetch_failures_total",
		Help: "Total failed analytics playability-scores fetches (index fell back to health-only)",
	})
)
```

- [ ] **Step 2: Add the wire field**

In `services/catalog/internal/domain/capability.go`, add to `ProviderCap` (right after the `Lang` field, before `Variants`):

```go
	// PlayabilityIndex is the blended, decayed rank score (Phase B). Higher =
	// more playable. The FE sorts the `degraded` bucket by it. omitempty drops
	// it when analytics is unavailable and the blend was skipped.
	PlayabilityIndex float64 `json:"playability_index,omitempty"`
```

- [ ] **Step 3: Build**

Run: `cd services/catalog && go build ./... && cd ../../libs/metrics && go build ./...`
Expected: clean. (No test — pure declarations; exercised by Task 5/6.)

- [ ] **Step 4: Commit**

```bash
git add libs/metrics/playability.go services/catalog/internal/domain/capability.go
git commit -m "feat(catalog): playability_index wire field + promotion/fetch-failure metrics"
```

---

## Task 5: Catalog — best-effort analytics read client + blend math

**Files:**
- Create: `services/catalog/internal/service/capability/analytics_client.go`
- Create: `services/catalog/internal/service/capability/playability.go`
- Create: `services/catalog/internal/service/capability/playability_test.go`

**Interfaces:**
- Consumes: `ANALYTICS_INTERNAL_URL`, `domain.ScraperProvider` (`.Health`), `domain` health consts.
- Produces:
  - `type providerScore struct { ThisAnimeWatch, GlobalWatch, RecentUp float64 }`
  - `type PlayabilitySource interface { Scores(ctx context.Context, animeID string) map[string]providerScore }` — the dep `Service` will hold (Task 7). Best-effort: returns nil/empty on any failure, never errors.
  - `NewPlayabilityClient(analyticsURL string, enabled bool) *playabilityClient` implementing `PlayabilitySource`.
  - `type blendData struct { ... }`, `newBlendData(map[string]providerScore) *blendData`, `(*blendData).indexFor(provider string, health domain.HealthStatus) float64`, `(*blendData).thisAnimeWatch(provider string) float64`, ctx helpers `withBlend`/`blendFrom`, and the tunable weight consts + `promoteFloor()`.

**Context:** Mirror the `libs/tracing` producer posture (5s client timeout, swallow all errors, best-effort) but as a synchronous read on the request path — use a **3s** timeout so a slow analytics never stretches report assembly. The client applies the ONE name alias: probe's `recent_up` arrives under `kodik-noads`, but the cap is `kodik` — merge it. Normalization is max-over-providers within the fetched set (so the score set is self-consistent regardless of which providers made the feed). Confirm the exact health enum identifiers in `services/catalog/internal/domain/scraper_provider.go` (`domain.HealthUp`/`HealthRecovering`/`HealthDown` and the `Health` field type) and use them verbatim in `healthTerm`.

- [ ] **Step 1: Write the failing test (pure blend + client alias)**

Create `services/catalog/internal/service/capability/playability_test.go`:

```go
package capability

import (
	"math"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBlend_NormalizesAndWeights(t *testing.T) {
	scores := map[string]providerScore{
		"gogoanime":     {ThisAnimeWatch: 2, GlobalWatch: 40, RecentUp: 6},
		"allanime-okru": {ThisAnimeWatch: 1, GlobalWatch: 20, RecentUp: 3},
	}
	b := newBlendData(scores)
	// gogoanime is the max in every term → normalized terms all 1.0, up health.
	idxGogo := b.indexFor("gogoanime", domain.HealthUp)
	wantGogo := wThisAnime*1 + wGlobal*1 + wRecentUp*1 + wHealth*hUp
	if !approx(idxGogo, wantGogo) {
		t.Errorf("gogoanime index = %v, want %v", idxGogo, wantGogo)
	}
	// A down provider ranks below an up provider with identical raw scores.
	up := b.indexFor("allanime-okru", domain.HealthUp)
	down := b.indexFor("allanime-okru", domain.HealthDown)
	if !(up > down) {
		t.Errorf("up (%v) must exceed down (%v)", up, down)
	}
}

func TestBlend_UnknownProviderIsHealthOnly(t *testing.T) {
	b := newBlendData(map[string]providerScore{"x": {GlobalWatch: 5}})
	// A provider absent from the scores map contributes only its health term.
	if got := b.indexFor("not-present", domain.HealthUp); !approx(got, wHealth*hUp) {
		t.Errorf("absent provider index = %v, want health-only %v", got, wHealth*hUp)
	}
}

func TestBlend_NilIsHealthOnly(t *testing.T) {
	var b *blendData // analytics unavailable
	if got := b.indexFor("gogoanime", domain.HealthUp); !approx(got, wHealth*hUp) {
		t.Errorf("nil blend index = %v, want health-only %v", got, wHealth*hUp)
	}
	if got := b.thisAnimeWatch("gogoanime"); got != 0 {
		t.Errorf("nil blend thisAnimeWatch = %v, want 0", got)
	}
}

func TestParseScores_AliasesKodikNoads(t *testing.T) {
	// probe recent_up arrives under "kodik-noads"; cap id is "kodik" → merge.
	raw := map[string]providerScore{
		"kodik":       {GlobalWatch: 10, ThisAnimeWatch: 1},
		"kodik-noads": {RecentUp: 7},
	}
	merged := applyProviderAliases(raw)
	if _, ok := merged["kodik-noads"]; ok {
		t.Error("kodik-noads must be merged away")
	}
	k := merged["kodik"]
	if k.RecentUp != 7 || k.GlobalWatch != 10 || k.ThisAnimeWatch != 1 {
		t.Errorf("kodik merge wrong: %+v", k)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run 'TestBlend|TestParseScores' -v`
Expected: FAIL — `newBlendData`, `providerScore`, `applyProviderAliases`, weight consts undefined.

- [ ] **Step 3: Write the blend math + weights**

Create `services/catalog/internal/service/capability/playability.go`:

```go
package capability

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// --- tunable weights (single source; adjust here) ---------------------------
// Applied to max-normalized terms in [0,1]; the owner ranks recent this-title
// watch success highest, then recent probe-up + health, then global watch.
const (
	wThisAnime = 3.0
	wGlobal    = 1.0
	wRecentUp  = 1.5
	wHealth    = 1.0

	hUp   = 1.0  // health term multipliers (pre-wHealth)
	hRec  = 0.3
	hDown = -1.0

	// promoteFloorValue: decayed this-anime watch weight (~one successful watch
	// within the last week) that flips a has-content degraded provider to
	// active/selectable for that title. The single hard boundary in the model.
	promoteFloorValue = 0.5
)

func promoteFloor() float64 { return promoteFloorValue }

// providerScore is one provider's decayed weights (catalog-side mirror of the
// analytics wire shape).
type providerScore struct {
	ThisAnimeWatch float64 `json:"this_anime_watch"`
	GlobalWatch    float64 `json:"global_watch"`
	RecentUp       float64 `json:"recent_up"`
}

// providerAliases maps a source-specific provider name to its canonical
// capability id. probe_runs uses "kodik-noads" where the cap id is "kodik".
var providerAliases = map[string]string{"kodik-noads": "kodik"}

// applyProviderAliases merges alias keys into their canonical cap id, summing
// each term (an alias only carries the term its source produces).
func applyProviderAliases(raw map[string]providerScore) map[string]providerScore {
	if len(raw) == 0 {
		return raw
	}
	out := make(map[string]providerScore, len(raw))
	for k, v := range raw {
		canon := k
		if a, ok := providerAliases[k]; ok {
			canon = a
		}
		cur := out[canon]
		cur.ThisAnimeWatch += v.ThisAnimeWatch
		cur.GlobalWatch += v.GlobalWatch
		cur.RecentUp += v.RecentUp
		out[canon] = cur
	}
	return out
}

// blendData carries the fetched, aliased scores plus the normalization maxima
// for one report build. A nil *blendData means "analytics unavailable" and
// yields a health-only index (graceful degradation).
type blendData struct {
	scores                       map[string]providerScore
	maxThis, maxGlobal, maxRecent float64
}

func newBlendData(scores map[string]providerScore) *blendData {
	b := &blendData{scores: scores}
	for _, s := range scores {
		if s.ThisAnimeWatch > b.maxThis {
			b.maxThis = s.ThisAnimeWatch
		}
		if s.GlobalWatch > b.maxGlobal {
			b.maxGlobal = s.GlobalWatch
		}
		if s.RecentUp > b.maxRecent {
			b.maxRecent = s.RecentUp
		}
	}
	return b
}

func norm(x, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return x / max
}

func healthTerm(h domain.HealthStatus) float64 {
	switch h {
	case domain.HealthUp:
		return hUp
	case domain.HealthRecovering:
		return hRec
	case domain.HealthDown:
		return hDown
	default:
		return 0
	}
}

// indexFor blends one provider's normalized watch/probe terms with its health.
// Nil-safe: a nil blend (or an absent provider) yields the health-only term.
func (b *blendData) indexFor(provider string, h domain.HealthStatus) float64 {
	ht := wHealth * healthTerm(h)
	if b == nil {
		return ht
	}
	s := b.scores[provider]
	return wThisAnime*norm(s.ThisAnimeWatch, b.maxThis) +
		wGlobal*norm(s.GlobalWatch, b.maxGlobal) +
		wRecentUp*norm(s.RecentUp, b.maxRecent) +
		ht
}

// thisAnimeWatch returns the raw decayed this-title watch weight (drives
// promotion). Nil-safe → 0.
func (b *blendData) thisAnimeWatch(provider string) float64 {
	if b == nil {
		return 0
	}
	return b.scores[provider].ThisAnimeWatch
}

// --- ctx plumbing: blendData is per-request, carried through applyFeedFields --
type blendCtxKey struct{}

func withBlend(ctx context.Context, b *blendData) context.Context {
	return context.WithValue(ctx, blendCtxKey{}, b)
}

// blendFrom returns the request's blendData, or nil if none was seeded.
func blendFrom(ctx context.Context) *blendData {
	b, _ := ctx.Value(blendCtxKey{}).(*blendData)
	return b
}
```

- [ ] **Step 4: Run blend tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/capability/ -run 'TestBlend|TestParseScores' -v`
Expected: PASS. (If a health const name differs, fix `healthTerm` to the real identifiers from `scraper_provider.go` and re-run.)

- [ ] **Step 5: Write the best-effort read client**

Create `services/catalog/internal/service/capability/analytics_client.go`:

```go
package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// playabilityFetchTimeout bounds the synchronous read on the report path.
const playabilityFetchTimeout = 3 * time.Second

// PlayabilitySource supplies per-provider decayed playability scores for an
// anime. Best-effort: implementations MUST return an empty map (never error)
// on any failure so report assembly is never blocked.
type PlayabilitySource interface {
	Scores(ctx context.Context, animeID string) map[string]providerScore
}

// playabilityClient reads /internal/playability from analytics. Mirrors the
// libs/tracing producer posture (short timeout, swallow all errors).
type playabilityClient struct {
	url     string // "<ANALYTICS_INTERNAL_URL>/internal/playability"
	client  *http.Client
	enabled bool
}

// NewPlayabilityClient builds the read client. enabled=false (kill switch) makes
// Scores a no-op returning nil.
func NewPlayabilityClient(analyticsURL string, enabled bool) *playabilityClient {
	return &playabilityClient{
		url:     analyticsURL + "/internal/playability",
		client:  &http.Client{Timeout: playabilityFetchTimeout},
		enabled: enabled,
	}
}

type playabilityWire struct {
	Providers map[string]providerScore `json:"providers"`
}

// Scores fetches and alias-merges per-provider scores. Any failure → nil map
// (health-only index, no promotions) + a fetch-failure metric.
func (c *playabilityClient) Scores(ctx context.Context, animeID string) map[string]providerScore {
	if c == nil || !c.enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, playabilityFetchTimeout)
	defer cancel()

	u := c.url + "?anime_id=" + url.QueryEscape(animeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	resp, err := c.client.Do(req)
	if err != nil {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	var wire playabilityWire
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		metrics.CapabilityPlayabilityFetchFailuresTotal.Inc()
		return nil
	}
	return applyProviderAliases(wire.Providers)
}
```

- [ ] **Step 6: Build + run the capability tests**

Run: `cd services/catalog && go build ./... && go test ./internal/service/capability/ -run 'TestBlend|TestParseScores' -v`
Expected: build clean, tests PASS.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/service/capability/analytics_client.go services/catalog/internal/service/capability/playability.go services/catalog/internal/service/capability/playability_test.go
git commit -m "feat(catalog): best-effort playability read client + blend math (norm+weights+health)"
```

---

## Task 6: Catalog — promotion in `deriveProviderView` + attach index in assembly

**Files:**
- Modify: `services/catalog/internal/service/capability/providerview.go`
- Modify: `services/catalog/internal/service/capability/families_ru.go` (`applyFeedFields`)
- Modify: `services/catalog/internal/service/capability/service.go` (`Service` dep + `buildFamilies` fetch/seed + call sites)
- Modify: `services/catalog/cmd/catalog-api/main.go` (inject client)
- Test: `services/catalog/internal/service/capability/providerview_test.go` (create/extend)
- Modify (compile fixes): existing `*_test.go` that call `deriveProviderView`/`applyFeedFields`.

**Interfaces:**
- Consumes: `blendFrom(ctx)`, `promoteFloor()`, `PlayabilitySource`.
- Produces: `deriveProviderView(row domain.ScraperProvider, hasContent bool, thisAnimeWatch, promoteFloor float64) (state string, selectable, hackerOnly bool)`; `applyFeedFields(ctx context.Context, cap *domain.ProviderCap, row domain.ScraperProvider, hasContent bool) bool`; `Service` field `playability PlayabilitySource`.

**Context:** Promotion is the FIRST check in `deriveProviderView`, guarded by `hasContent` (a `no_content` provider can't have real watches, so no conflict with Phase A). `applyFeedFields` now pulls the per-request `blendData` from ctx, computes+attaches `PlayabilityIndex` (health-only if blend nil), and threads the this-anime watch signal into promotion — incrementing the promotions counter when a manual-policy row is flipped. `buildFamilies` fetches scores ONCE at the top (best-effort) and seeds them into ctx via `withBlend`; because scores are fetched only on a report cache miss, they are effectively cached alongside the report's 10-min TTL (a promotion that changes between builds is masked for ≤10 min — acceptable, since watch weights decay over days).

- [ ] **Step 1: Write the failing promotion test**

Create/extend `services/catalog/internal/service/capability/providerview_test.go`:

```go
package capability

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestDeriveProviderView_PromotionFlipsManualWhenWatched(t *testing.T) {
	row := domain.ScraperProvider{Policy: domain.PolicyManual, Health: domain.HealthUp}
	// Below floor → stays degraded/hacker-only (unchanged Phase-A behavior).
	if st, sel, hk := deriveProviderView(row, true, 0.2, promoteFloor()); st != "degraded" || !sel || !hk {
		t.Errorf("below-floor manual = (%q,%v,%v), want degraded/true/true", st, sel, hk)
	}
	// At/above floor + has content → promoted to active/selectable/non-hacker.
	if st, sel, hk := deriveProviderView(row, true, 0.9, promoteFloor()); st != "active" || !sel || hk {
		t.Errorf("promoted manual = (%q,%v,%v), want active/true/false", st, sel, hk)
	}
	// Above floor but NO content → cannot promote (Phase A no_content wins).
	if st, _, _ := deriveProviderView(row, false, 0.9, promoteFloor()); st != "degraded" {
		// manual+no-content: manual gate still first for a NON-promoted row, but
		// promotion is guarded by hasContent so it does not fire → degraded.
		t.Errorf("no-content manual above floor = %q, want degraded", st)
	}
}

func TestDeriveProviderView_UnchangedWithoutSignal(t *testing.T) {
	// thisAnimeWatch=0 preserves the exact pre-Phase-B truth table.
	auto := domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthUp}
	if st, sel, hk := deriveProviderView(auto, true, 0, promoteFloor()); st != "active" || !sel || hk {
		t.Errorf("auto+content = (%q,%v,%v), want active/true/false", st, sel, hk)
	}
	if st, _, _ := deriveProviderView(auto, false, 0, promoteFloor()); st != "no_content" {
		t.Errorf("auto+no-content = %q, want no_content", st)
	}
	rec := domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthRecovering}
	if st, _, _ := deriveProviderView(rec, true, 0, promoteFloor()); st != "recovering" {
		t.Errorf("auto+recovering = %q, want recovering", st)
	}
}
```

(Confirm `domain.PolicyAuto` is the non-manual policy identifier from `scraper_provider.go`; adjust if named differently.)

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/catalog && go test ./internal/service/capability/ -run TestDeriveProviderView -v`
Expected: FAIL — `deriveProviderView` arity mismatch (still 2 params).

- [ ] **Step 3: Extend `deriveProviderView`**

Replace the function in `providerview.go` with:

```go
func deriveProviderView(row domain.ScraperProvider, hasContent bool, thisAnimeWatch, promoteFloor float64) (state string, selectable, hackerOnly bool) {
	// Per-title promotion (B3/B4): a has-content provider with enough recent
	// this-title watch success flips to active/selectable regardless of a
	// manual policy. Guarded by hasContent so it never overrides Phase-A
	// no_content (a no_content provider can't have real watches anyway).
	if hasContent && thisAnimeWatch >= promoteFloor {
		return "active", true, false
	}
	if row.Policy == domain.PolicyManual {
		return "degraded", true, true
	}
	if !hasContent {
		return "no_content", false, false
	}
	if row.Health == domain.HealthRecovering {
		return "recovering", true, false
	}
	return "active", true, false
}
```

Update the doc comment above it to note the new promotion branch.

- [ ] **Step 4: Run promotion tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/capability/ -run TestDeriveProviderView -v`
Expected: PASS.

- [ ] **Step 5: Thread ctx + index into `applyFeedFields`**

In `families_ru.go`, replace `applyFeedFields` with (adds `ctx`, index attach, promotion signal + metric):

```go
func applyFeedFields(ctx context.Context, cap *domain.ProviderCap, row domain.ScraperProvider, hasContent bool) bool {
	if !row.IsRegistered() { // disabled → omit
		return false
	}
	blend := blendFrom(ctx)
	watch := blend.thisAnimeWatch(cap.Provider)
	prevManual := row.Policy == domain.PolicyManual

	state, selectable, hackerOnly := deriveProviderView(row, hasContent, watch, promoteFloor())
	cap.State, cap.Selectable, cap.HackerOnly = state, selectable, hackerOnly
	cap.Order = row.PreferenceWeight
	cap.Group = wireGroup(row.Group)
	cap.Audios = audiosFromTraits(row)
	cap.Reason = row.Reason
	cap.PlayabilityIndex = blend.indexFor(cap.Provider, row.Health)

	// A manual-policy row that came out active was promoted → count it.
	if prevManual && state == "active" {
		metrics.CapabilityPlayabilityPromotionsTotal.Inc()
	}
	return true
}
```

Add imports to `families_ru.go` as needed: `"context"` and the `metrics` package (`"github.com/ILITA-hub/animeenigma/libs/metrics"`).

- [ ] **Step 6: Update all `applyFeedFields` call sites to pass ctx**

The 8 call sites all have `ctx` in scope. Update each:
- `service.go:189` (EN) → `applyFeedFields(ctx, &cap, row, true)`
- `families_ru.go:77` (noContentFamily) → pass its `ctx`
- `families_ru.go:147, :186, :243, :298` → pass `ctx`
- `families_firstparty.go:99` (ae), `:118` → pass `ctx`

Grep to confirm none missed: `grep -rn "applyFeedFields(" services/catalog/internal/service/capability/ | grep -v _test | grep -v "func applyFeedFields"`.

- [ ] **Step 7: Add the `Service` dep + fetch/seed scores in `buildFamilies`**

In `service.go`:
1. Add a field to the `Service` struct: `playability PlayabilitySource` (place beside the other source deps `catalog`/`library`/`health`). Update the `Service` constructor (`NewService`/`New`) to accept and store it. If the constructor uses a functional-options or positional pattern, follow that pattern; a nil `playability` MUST be tolerated (→ health-only, no promotion).
2. At the TOP of `buildFamilies`, before the fan-out, seed the blend into ctx:

```go
	var raw map[string]providerScore
	if s.playability != nil {
		raw = s.playability.Scores(ctx, animeID)
	}
	ctx = withBlend(ctx, newBlendData(raw))
```

(`newBlendData(nil)` returns a non-nil `*blendData` with empty scores + zero maxima → `indexFor` yields health-only, `thisAnimeWatch` yields 0. That is the correct graceful path, and every family goroutine that closes over `ctx` sees the seeded blend.)

- [ ] **Step 8: Inject the client in catalog main.go**

In `services/catalog/cmd/catalog-api/main.go`, near the existing `ANALYTICS_INTERNAL_URL` block (lines ~378), build the read client and pass it into the capability `Service` constructor:

```go
	playabilityEnabled := os.Getenv("PLAYABILITY_INDEX_ENABLED") != "false" // default true
	playabilityClient := capability.NewPlayabilityClient(analyticsURL, playabilityEnabled)
```

Then add `playabilityClient` to the capability `Service` construction call (matching the Step-7 constructor change). `analyticsURL` already exists from the effects-producer block — reuse it (do not re-read the env).

- [ ] **Step 9: Fix existing test call sites + build**

Compile the package; fix any existing `*_test.go` calling the old 2-arg `applyFeedFields` or old 2-arg `deriveProviderView` (pass `ctx` / `0, promoteFloor()` respectively — a plain `withBlend(context.Background(), newBlendData(nil))` ctx for tests that don't exercise promotion).

Run: `cd services/catalog && go build ./... && go test ./internal/service/capability/ -count=1 -race`
Expected: build clean; ALL capability tests pass (existing + new).

- [ ] **Step 10: Commit**

```bash
git add services/catalog/internal/service/capability/ services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): blend playability_index onto caps + per-title promotion of degraded providers"
```

---

## Task 7: Frontend — types + degraded-bucket sort by index

**Files:**
- Modify: `frontend/web/src/types/capabilities.ts`
- Modify: `frontend/web/src/types/aePlayer.ts`
- Modify: `frontend/web/src/composables/aePlayer/useProviderFeed.ts`
- Modify: `frontend/web/src/components/player/aePlayer/SourcePanel.vue`
- Test: `frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts`

**Interfaces:**
- Consumes: wire field `playability_index?: number` on `ProviderCap`.
- Produces: `ProviderRow.playability_index?: number`; `sortedRows` degraded-bucket tiebreak by index.

**Context:** `ProviderCap` (`capabilities.ts`) mirrors the Go wire shape — add the field there; it flows into `capMap`/`report` automatically. The cap→row map in `useProviderFeed.ts:toRow` is EXPLICIT, so the field must be added there to reach `ProviderRow`/`sortedRows`. Only the WITHIN-`degraded` tiebreak changes; `active`/`recovering`/`no_content` buckets keep `order`. Promoted providers arrive as `active` and sort in the active bucket automatically (no FE special-casing).

- [ ] **Step 1: Write the failing sort test**

In `SourcePanel.spec.ts`, first extend the row factory to accept `playability_index` (the `r`/`a` helpers build a `ProviderRow`), then add:

```ts
  it('sorts the degraded bucket by playability_index desc, order as final tiebreak (hacker mode)', () => {
    // All three degraded → within-bucket order is by playability_index desc,
    // NOT by `order` (gogoanime has the highest order but the lowest index).
    const rows = [
      a('gogoanime', 'degraded', { order: 90, playability_index: 0.5 }),
      a('miruro', 'degraded', { order: 10, playability_index: 4.2 }),
      a('nineanime', 'degraded', { order: 50, playability_index: 2.0 }),
    ]
    const w = mount(SourcePanel, {
      props: { ...cb, rows, provider: '', hackerMode: true, playbackError: false } as any,
      ...mountOpts,
    })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['miruro', 'nineanime', 'gogoanime'])
  })

  it('keeps active bucket ordered by `order` (index does NOT reorder active)', () => {
    const rows = [
      a('allanime', 'active', { order: 10, playability_index: 0.1 }),
      a('miruro', 'active', { order: 90, playability_index: 5.0 }),
    ]
    const w = mount(SourcePanel, {
      props: { ...cb, rows, provider: '', hackerMode: true, playbackError: false } as any,
      ...mountOpts,
    })
    const ids = w.findAll('[data-test="provider-chip"]').map(c => c.attributes('data-id'))
    expect(ids).toEqual(['miruro', 'allanime']) // by order desc, index ignored
  })
```

(If the existing `a(id, state)` helper signature can't take a third overrides arg, widen it: `const a = (id, state, over = {}) => r(id, { state, ...over })`.)

- [ ] **Step 2: Run it to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts`
Expected: FAIL — degraded rows still sort by `order` (gogoanime first), and `playability_index` isn't on the row type.

- [ ] **Step 3: Add the wire field to `ProviderCap`**

In `capabilities.ts`, add after `lang?` (before `variants`):

```ts
  /** Blended, decayed playability rank (Phase B). Higher = more playable. The
   *  panel sorts the `degraded` bucket by it. Absent when analytics is
   *  unavailable and the backend skipped the blend. */
  playability_index?: number
```

- [ ] **Step 4: Add the field to `ProviderRow` + map it in `toRow`**

In `aePlayer.ts`, add to `ProviderRow` (after `reason?`):

```ts
  /** Mirrors `ProviderCap.playability_index`; drives the degraded-bucket sort. */
  playability_index?: number
```

In `useProviderFeed.ts` `toRow`, add the field to the returned object:

```ts
    lang: cap.lang,
    reason: cap.reason,
    playability_index: cap.playability_index,
```

- [ ] **Step 5: Re-tiebreak the degraded bucket in `SourcePanel.vue`**

Replace the `sortedRows` comparator (lines ~253-259) with:

```ts
const sortedRows = computed(() =>
  [...props.rows].sort((a, b) => {
    const stateDiff = STATE_RANK[a.state] - STATE_RANK[b.state]
    if (stateDiff) return stateDiff
    // Within the degraded bucket, rank by real playability; order is the final
    // tiebreak. Other buckets keep backend `order` (desc). (Promoted providers
    // arrive as `active` and sort with the active bucket.)
    if (a.state === 'degraded') {
      return (b.playability_index ?? 0) - (a.playability_index ?? 0) || b.order - a.order
    }
    return b.order - a.order
  }),
)
```

- [ ] **Step 6: Run the sort tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/SourcePanel.spec.ts`
Expected: PASS (all existing + 2 new).

- [ ] **Step 7: Type-check + feed spec**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderFeed.spec.ts && bunx tsc --noEmit`
Expected: PASS; no type errors.

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/types/capabilities.ts frontend/web/src/types/aePlayer.ts frontend/web/src/composables/aePlayer/useProviderFeed.ts frontend/web/src/components/player/aePlayer/SourcePanel.vue frontend/web/src/components/player/aePlayer/SourcePanel.spec.ts
git commit -m "feat(player): sort degraded source bucket by playability_index"
```

---

## Integration & Deploy (post-implementation, via /animeenigma-after-update)

1. **Deploy order matters for verification** (feature is best-effort so order is not correctness-critical, but for end-to-end proof): **analytics → catalog → web**. The endpoint must exist before catalog's fetch returns non-empty; catalog must emit `playability_index` before the FE can sort by it.
2. **Verify analytics endpoint** (inside the Docker network): `curl -s 'http://analytics:8092/internal/playability?anime_id=<uuid>' | jq` → `{"providers":{...}}` with non-zero terms for providers that have recent watches/probes. Confirm it is NOT reachable via the gateway (`/api/...` has no such route).
3. **Verify catalog feed:** `curl -s http://localhost:8081/api/anime/<uuid>/capabilities | jq '.data.families[].providers[] | {provider, state, playability_index}'` → degraded providers carry a `playability_index`; a title with recent this-anime watches on a manual provider shows that provider `state:"active"`.
4. **Verify FE:** on an anime with ≥2 degraded providers, hacker mode lists them highest-index-first (not name/order order).
5. **Verify graceful degradation:** with analytics stopped, the feed still returns (health-only index, no promotions); `capability_playability_fetch_failures_total` increments.
6. **Metrics:** `curl -s http://localhost:8081/metrics | grep capability_playability` shows both counters; `capability_playability_promotions_total` rises when a promotion fires.
7. **Changelog:** Trump-mode entry (Russian) — degraded sources now ranked by REAL playability + a proven source auto-surfaces per title.
8. **Note:** whitelist reconciliation (Task 1) is forward-only; the index populates over the following days as new telemetry accrues.
