# Smart Source Selection — Stage 2a Implementation Plan (telemetry capture & observability)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture, per video provider + anime, how playback actually goes — resolve outcome (ok/fail), whether it reached the first frame, resolve latency, error kind, and stall/rebuffer events — as queryable rows in ClickHouse, and surface it on the Grafana "Playback / Health" dashboard. This is the data foundation the Stage 2b daily ranking will aggregate.

**Architecture:** A frontend beacon (`playerTelemetry.ts`, sibling of `feErrorLog.ts`) batches player events and POSTs them to a new public `/api/analytics/player-events` endpoint. A new analytics handler (mirroring `collect.go`) maps each to the existing `domain.Event` and enqueues it on the SAME batcher → `analytics.events` ClickHouse table, using two new `effect_kind` values. The unified player emits the events at the right moments. Grafana gets ClickHouse-backed panels for real-user provider reliability.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vitest; Go (analytics service), Go testing; Grafana ClickHouse datasource (`aenigma-clickhouse`).

**Scope boundary:** Stage 2a of `docs/superpowers/specs/2026-06-15-smart-source-selection-design.md`. Builds on Stage 1a/1b (shipped). **No player behavior change** — this only *records and visualizes*. Stage 2b (daily ranking pipeline) and 2c (ranking-aware default + same-day override + playback-time fallback) are separate plans that consume this data.

---

## Event model (no schema migration)

Player events reuse the existing `analytics.events` columns (confirmed schema at `services/analytics/internal/repo/clickhouse_schema.go`):

| Concept | Column | Encoding |
|---|---|---|
| event type | `effect_kind` | `'player_resolve'` or `'player_stall'` (new LowCardinality values) |
| provider | `target` | provider id (e.g. `kodik`, `allanime`) |
| provider marker | `target_kind` | `'provider'` |
| anime | `anime_id` | the uuid |
| resolve latency / stall duration | `duration_ms` | ms |
| outcome / reached-frame / error / stall metrics / audio / lang | `properties` | JSON: `{"outcome":"ok"|"fail","reached_playback":bool,"error_kind":"...","stall_ms":N,"audio":"sub","lang":"en","episode":N}` |
| attribution | `source` | `'fe'`; `origin` `'api'`; `event_type` `'player'` |

No new columns, no migration, no change to `InsertBatch`'s column order — a new `effect_kind` row flows through unchanged.

---

## File Structure

- **Create** `frontend/web/src/utils/playerTelemetry.ts` — beacon (buffer, dedup, rate-cap, `sendBeacon`→fetch).
- **Create** `frontend/web/src/utils/playerTelemetry.spec.ts` — rate-cap/dedup/flush tests.
- **Create** `services/analytics/internal/handler/playertelemetry.go` — HTTP handler (mirror `collect.go`).
- **Create** `services/analytics/internal/handler/playertelemetry_test.go` — wire→Event mapping + enqueue test.
- **Modify** `services/analytics/internal/transport/router.go` — register `/api/analytics/player-events`.
- **Modify** `services/analytics/cmd/analytics-api/main.go` — construct + wire the handler (same sink).
- **Modify** `services/gateway/internal/transport/router.go` — proxy `/analytics/player-events` (public).
- **Modify** `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — emit `resolve`(ok+reached) / `resolve`(fail) / `stall` at the right hooks.
- **Modify** `docker/grafana/dashboards/playback-health.json` — add ClickHouse panels.

---

### Task 1: Analytics — `/api/analytics/player-events` handler

**Files:**
- Create: `services/analytics/internal/handler/playertelemetry.go`
- Test: `services/analytics/internal/handler/playertelemetry_test.go`
- Modify: `services/analytics/internal/transport/router.go`, `services/analytics/cmd/analytics-api/main.go`

**Context:** Mirror `collect.go`. It accepts a JSON batch, maps each wire event to `domain.Event`, and calls `h.sink.Enqueue(ev)` (the shared `Sink` interface — `Enqueue(domain.Event) bool`). READ `collect.go`, `effects.go`, `domain/event.go`, and the `Sink` wiring in `cmd/analytics-api/main.go` + `adapters.go` first to match the exact construction pattern.

- [ ] **Step 1: Write the failing test**

Create `playertelemetry_test.go`. Use a fake sink capturing enqueued events (mirror how `collect_test.go`/`effects_test.go` fake the sink — read one first). Assert the mapping:

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type capSink struct{ events []domain.Event }

func (c *capSink) Enqueue(e domain.Event) bool { c.events = append(c.events, e); return true }

func TestPlayerTelemetry_MapsResolveAndStall(t *testing.T) {
	sink := &capSink{}
	h := NewPlayerTelemetryHandler(sink, "salt")
	body := `{"events":[
		{"kind":"resolve","provider":"kodik","anime_id":"a1","outcome":"ok","reached_playback":true,"latency_ms":1200,"audio":"dub","lang":"ru","episode":3},
		{"kind":"resolve","provider":"allanime","anime_id":"a2","outcome":"fail","reached_playback":false,"error_kind":"no_servers","latency_ms":5000},
		{"kind":"stall","provider":"kodik","anime_id":"a1","stall_ms":350,"episode":3}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204", rec.Code)
	}
	if len(sink.events) != 3 {
		t.Fatalf("enqueued %d want 3", len(sink.events))
	}
	r0 := sink.events[0]
	if r0.EffectKind != "player_resolve" || r0.Target != "kodik" || r0.TargetKind != "provider" || r0.AnimeID != "a1" || r0.DurationMS != 1200 {
		t.Errorf("resolve row mismatch: %+v", r0)
	}
	if r0.Source != "fe" || r0.EventType != "player" {
		t.Errorf("attribution mismatch: %+v", r0)
	}
	if !strings.Contains(r0.Properties, `"outcome":"ok"`) || !strings.Contains(r0.Properties, `"reached_playback":true`) {
		t.Errorf("properties missing outcome/reached: %s", r0.Properties)
	}
	s := sink.events[2]
	if s.EffectKind != "player_stall" || s.DurationMS != 350 {
		t.Errorf("stall row mismatch: %+v", s)
	}
	_ = json.RawMessage{}
}
```
(Adjust `NewPlayerTelemetryHandler`'s signature and the `Sink` type name to whatever `collect.go` actually uses — read it. If the real sink interface differs, match it.)

- [ ] **Step 2: Run, verify FAIL**

`cd services/analytics && go test ./internal/handler/ -run TestPlayerTelemetry` → FAIL (undefined `NewPlayerTelemetryHandler`).

- [ ] **Step 3: Implement the handler**

Create `playertelemetry.go` modeled on `collect.go`. Sketch (adapt types/sink to the real ones):

```go
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type wirePlayerEvent struct {
	Kind            string `json:"kind"` // "resolve" | "stall"
	Provider        string `json:"provider"`
	AnimeID         string `json:"anime_id"`
	Episode         int    `json:"episode"`
	Outcome         string `json:"outcome"` // resolve: "ok"|"fail"
	ReachedPlayback bool   `json:"reached_playback"`
	ErrorKind       string `json:"error_kind"`
	LatencyMS       int    `json:"latency_ms"`
	StallMS         int    `json:"stall_ms"`
	Audio           string `json:"audio"`
	Lang            string `json:"lang"`
}

type playerEventEnvelope struct {
	Events []wirePlayerEvent `json:"events"`
}

type PlayerTelemetryHandler struct {
	sink Sink // same Sink interface collect.go uses
	salt string
}

func NewPlayerTelemetryHandler(sink Sink, salt string) *PlayerTelemetryHandler {
	return &PlayerTelemetryHandler{sink: sink, salt: salt}
}

const maxPlayerEventsPerPost = 100

func (h *PlayerTelemetryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var env playerEventEnvelope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&env); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	for i, we := range env.Events {
		if i >= maxPlayerEventsPerPost {
			break
		}
		if we.Provider == "" || (we.Kind != "resolve" && we.Kind != "stall") {
			continue
		}
		effectKind := "player_resolve"
		dur := we.LatencyMS
		if we.Kind == "stall" {
			effectKind = "player_stall"
			dur = we.StallMS
		}
		props, _ := json.Marshal(map[string]any{
			"outcome":          we.Outcome,
			"reached_playback": we.ReachedPlayback,
			"error_kind":       we.ErrorKind,
			"audio":            we.Audio,
			"lang":             we.Lang,
			"episode":          we.Episode,
		})
		ev := domain.Event{
			EventID:    uuid.NewString(),
			ReceivedAt: now,
			Timestamp:  now,
			EventType:  domain.EventType("player"),
			Origin:     "api",
			Source:     "fe",
			Accuracy:   "exact",
			EffectKind: effectKind,
			TargetKind: "provider",
			Target:     capString(we.Provider), // reuse collect.go's capString
			AnimeID:    we.AnimeID,
			DurationMS: dur,
			Requests:   1,
			Properties: string(props),
		}
		h.sink.Enqueue(ev)
	}
	w.WriteHeader(http.StatusNoContent)
}
```
(`capString` exists in the handler package — confirm. `Sink` is the interface `collect.go`'s handler holds — confirm its exact name/shape. `domain.EventType("player")` — confirm `EventType` is a string type and `"player"` passes `Event.Validate()`; if `Validate()` whitelists event types, add `"player"` there additively, and add a test for it.)

- [ ] **Step 4: Register the route**

In `services/analytics/internal/transport/router.go`, beside `/api/analytics/client-errors`:
```go
r.Post("/api/analytics/player-events", playerTelemetry.ServeHTTP)
```
Construct `playerTelemetry` in `cmd/analytics-api/main.go` with the SAME sink passed to `collect` (the `countingSink{batcher}`), mirroring how `collect`/`clientError` handlers are built and passed into the router constructor. Read main.go to match the wiring (the router likely takes handlers as params).

- [ ] **Step 5: Run tests**

`cd services/analytics && go test ./internal/handler/ -run TestPlayerTelemetry` → PASS.
`cd services/analytics && go test ./...` → all pass (add `"player"` to any event-type whitelist if a validate test fails).

- [ ] **Step 6: Commit (path-scoped, no amend, no push)**

```bash
cd /data/animeenigma
git add services/analytics/internal/handler/playertelemetry.go \
        services/analytics/internal/handler/playertelemetry_test.go \
        services/analytics/internal/transport/router.go \
        services/analytics/cmd/analytics-api/main.go
git commit -m "feat(analytics): /api/analytics/player-events ingestion (player_resolve/player_stall)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: Gateway — proxy the public route

**Files:** Modify `services/gateway/internal/transport/router.go`

- [ ] **Step 1: Add the proxy route**

Beside the existing `r.Post("/analytics/client-errors", proxyHandler.ProxyToAnalytics)`:
```go
// Player telemetry beacon (resolve/stall outcomes). PUBLIC — anonymous,
// same trust model as /collect; per-IP rate limiting already applies.
r.Post("/analytics/player-events", proxyHandler.ProxyToAnalytics)
```

- [ ] **Step 2: Build + test**

`cd services/gateway && go build ./... && go test ./...` → pass. If there's a router test asserting the analytics route set, extend it.

- [ ] **Step 3: Commit (path-scoped)** — `router.go`. Message: `feat(gateway): proxy /api/analytics/player-events`. No amend, no push.

---

### Task 3: Frontend — `playerTelemetry.ts` beacon

**Files:**
- Create: `frontend/web/src/utils/playerTelemetry.ts`
- Test: `frontend/web/src/utils/playerTelemetry.spec.ts`

**Context:** Mirror `frontend/web/src/utils/feErrorLog.ts` (READ it): a module-level buffer, `MAX_BATCH`/`FLUSH_MS` flush, a rate-cap + session-cap, `sendBeacon`→`fetch keepalive`, `pagehide` flush, and a loop-guard so the beacon never reports its own traffic. Endpoint: `${import.meta.env.VITE_API_URL || '/api'}/analytics/player-events`.

- [ ] **Step 1: Write the failing test**

Create `playerTelemetry.spec.ts`. Test the pure-ish buffer/rate-cap behavior with a mocked `navigator.sendBeacon`. Cover: (a) `recordPlayerEvent` buffers and a manual `flushPlayerTelemetry()` calls sendBeacon once with the batched payload; (b) rate-cap drops beyond N/min; (c) malformed/no-provider events are ignored. Model assertions on how `feErrorLog.spec.ts` mocks `sendBeacon` (read it). Example skeleton:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { recordPlayerEvent, flushPlayerTelemetry, __resetPlayerTelemetryForTest } from './playerTelemetry'

describe('playerTelemetry', () => {
  let beacon: ReturnType<typeof vi.fn>
  beforeEach(() => {
    beacon = vi.fn(() => true)
    vi.stubGlobal('navigator', { sendBeacon: beacon, userAgent: 'test' })
    __resetPlayerTelemetryForTest()
  })

  it('buffers events and flushes one beacon with the batch', () => {
    recordPlayerEvent({ kind: 'resolve', provider: 'kodik', anime_id: 'a1', outcome: 'ok', reached_playback: true, latency_ms: 900 })
    recordPlayerEvent({ kind: 'stall', provider: 'kodik', anime_id: 'a1', stall_ms: 300 })
    flushPlayerTelemetry()
    expect(beacon).toHaveBeenCalledTimes(1)
    const body = JSON.parse((beacon.mock.calls[0][1] as Blob).toString?.() ?? '{}')
    // (adapt Blob reading to match feErrorLog.spec.ts technique)
  })

  it('ignores events with no provider', () => {
    recordPlayerEvent({ kind: 'resolve', provider: '', anime_id: 'a1' } as never)
    flushPlayerTelemetry()
    expect(beacon).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run, verify FAIL** — module not found.

- [ ] **Step 3: Implement** `playerTelemetry.ts` mirroring `feErrorLog.ts`:

```ts
const ENDPOINT = `${import.meta.env.VITE_API_URL || '/api'}/analytics/player-events`
const MAX_BATCH = 20
const FLUSH_MS = 5000
const RATE_PER_MIN = 60        // player events are sparse; cap generously
const SESSION_CAP = 500

export interface PlayerEvent {
  kind: 'resolve' | 'stall'
  provider: string
  anime_id: string
  episode?: number
  outcome?: 'ok' | 'fail'
  reached_playback?: boolean
  error_kind?: string
  latency_ms?: number
  stall_ms?: number
  audio?: string
  lang?: string
}

let buf: PlayerEvent[] = []
// ... minuteCount/sessionCount/timers exactly like feErrorLog ...

export function recordPlayerEvent(e: PlayerEvent): void {
  if (!e || !e.provider || (e.kind !== 'resolve' && e.kind !== 'stall')) return
  // rate-cap (mirror feErrorLog.tryPush), then buf.push(e); armLifecycle(); maybeFlushBySize()
}

export function flushPlayerTelemetry(_reason = 'manual'): void {
  if (buf.length === 0) return
  const events = buf; buf = []
  const payload = JSON.stringify({ events })
  // sendBeacon(ENDPOINT, Blob) → fetch keepalive fallback (copy feErrorLog.flushFeErrors)
}

// test-only reset
export function __resetPlayerTelemetryForTest(): void { /* clear buf + counters + timers */ }
```
Keep it DRY by copying feErrorLog's proven rate-cap/flush/lifecycle code (don't invent a new mechanism). Loop-guard not strictly needed (this endpoint isn't an error sink) but exclude `/analytics/` URLs defensively if you add any URL field.

- [ ] **Step 4: Run, verify PASS** + `bunx tsc --noEmit` clean for the two files.

- [ ] **Step 5: Commit (path-scoped)** — the two files. Message: `feat(player): playerTelemetry beacon (resolve/stall events)`. No amend, no push.

---

### Task 4: Frontend — emit telemetry from the unified player

**Files:** Modify `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

⚠️ SHARED hot file: commit ONLY `UnifiedPlayer.vue`; never `--amend`; verify `git show --stat HEAD`; do NOT push. Locate anchors by searching quoted code.

**Context:** Emit at three moments (all best-effort; telemetry must never break playback):
1. **resolve ok + reached_playback** — once, when playback first passes frame 0.
2. **resolve fail** — in `loadEpisodesAndStream`'s catch (NotAvailableError → `error_kind:'not_available'`; other → `'stream_error'`) and on hls.js fatal (the engine fatal watcher → `'media_fatal'`).
3. **stall** — on the video `waiting` event during playback.

- [ ] **Step 1: Import + a resolve-timing ref**

```ts
import { recordPlayerEvent } from '@/utils/playerTelemetry'
```
Add `let resolveStartedAt = 0` and set `resolveStartedAt = performance.now()` at the top of `loadEpisodesAndStream` (and in `resolveStreamForCurrentEpisode` if it resolves independently). Add `let reachedReported = false` reset whenever a new stream load starts and on episode/provider change.

- [ ] **Step 2: Emit resolve-ok on first frame**

Find where the player marks playback started (search `hasStarted`). When `hasStarted` flips true (or on the first `timeupdate`/`playing` with `currentTime > 0`), once per stream:
```ts
if (!reachedReported) {
  reachedReported = true
  recordPlayerEvent({
    kind: 'resolve', provider: state.combo.value.provider, anime_id: props.animeId,
    episode: selectedEpisode.value?.number, outcome: 'ok', reached_playback: true,
    latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
    audio: state.combo.value.audio, lang: state.combo.value.lang,
  })
}
```

- [ ] **Step 3: Emit resolve-fail in the catch**

In `loadEpisodesAndStream`'s catch, where `sourceError` is set:
```ts
recordPlayerEvent({
  kind: 'resolve', provider: state.combo.value.provider, anime_id: props.animeId,
  episode: selectedEpisode.value?.number, outcome: 'fail', reached_playback: false,
  error_kind: isNotAvailable ? 'not_available' : 'stream_error',
  latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
  audio: state.combo.value.audio, lang: state.combo.value.lang,
})
```
And in the engine fatal handler (search `engine.fatal` / the watcher that sets the unrecoverable state), emit once with `error_kind:'media_fatal'`.

- [ ] **Step 4: Emit stall on `waiting`**

Find the `<video>` element binding / where media events are attached (search `@waiting` or `addEventListener('waiting'`). If a `waiting` handler exists, add the emit; else add a handler. Track stall start with a `let stallStartedAt = 0` set on `waiting` and, on the next `playing`, emit:
```ts
recordPlayerEvent({
  kind: 'stall', provider: state.combo.value.provider, anime_id: props.animeId,
  episode: selectedEpisode.value?.number,
  stall_ms: stallStartedAt ? Math.round(performance.now() - stallStartedAt) : 0,
})
```
Only emit when `reachedReported` (i.e. real mid-playback stalls, not the initial buffer).

- [ ] **Step 5: Flush on unmount/pagehide**

`playerTelemetry` arms its own interval + `pagehide` flush (like feErrorLog), so no extra wiring is required — but confirm `armLifecycle` runs on first `recordPlayerEvent`. (If feErrorLog's lifecycle is opt-in, mirror it.)

- [ ] **Step 6: Verify**

`cd frontend/web && bunx tsc --noEmit 2>&1 | grep -i UnifiedPlayer` (none); `bunx vitest run src/components/player/unified/` (all pass); `bunx eslint src/components/player/unified/UnifiedPlayer.vue` (clean).

- [ ] **Step 7: Commit (path-scoped, no amend)** — `UnifiedPlayer.vue`. Message: `feat(player): emit resolve/stall telemetry from unified player`. No push.

---

### Task 5: Grafana — ClickHouse panels on the Playback dashboard

**Files:** Modify `docker/grafana/dashboards/playback-health.json`

**Context:** Add a new row/section of ClickHouse panels (datasource `{ "type": "grafana-clickhouse-datasource", "uid": "aenigma-clickhouse" }`, `"rawSql"` with `$__timeFilter(timestamp)`). Model the panel JSON on an existing ClickHouse panel in `infra/grafana/dashboards/product-analytics.json`.

- [ ] **Step 1: Add panels** (give each a unique `id` and non-overlapping `gridPos`):
  1. **Reached-playback rate by provider** (timeseries):
     `SELECT target AS provider, countIf(JSONExtractBool(properties,'reached_playback'))/count() AS reached_rate FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_resolve' GROUP BY provider`
  2. **Resolve success % by provider**:
     `SELECT target AS provider, countIf(JSONExtractString(properties,'outcome')='ok')/count() AS ok_rate FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_resolve' GROUP BY provider`
  3. **p95 resolve latency by provider**:
     `SELECT target AS provider, quantile(0.95)(duration_ms) AS p95_ms FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_resolve' GROUP BY provider`
  4. **Stall rate / p95 stall ms by provider**:
     `SELECT target AS provider, count() AS stalls, quantile(0.95)(duration_ms) AS p95_stall_ms FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_stall' GROUP BY provider`
  5. **Top failing (provider, error_kind)** (table):
     `SELECT target AS provider, JSONExtractString(properties,'error_kind') AS error_kind, count() AS fails FROM events WHERE $__timeFilter(timestamp) AND effect_kind='player_resolve' AND JSONExtractString(properties,'outcome')='fail' GROUP BY provider, error_kind ORDER BY fails DESC LIMIT 20`

- [ ] **Step 2: Validate JSON** — `node -e "JSON.parse(require('fs').readFileSync('docker/grafana/dashboards/playback-health.json','utf8'))"`. Confirm dashboard `id`/`uid` unchanged and panel ids are unique.

- [ ] **Step 3: Commit (path-scoped)** — the dashboard json. Message: `feat(observability): real-user player telemetry panels on Playback dashboard`. No push.

---

## Final verification

- [ ] `cd services/analytics && go test ./...` ; `cd services/gateway && go test ./...`
- [ ] `cd frontend/web && bunx vitest run src/utils/playerTelemetry.spec.ts src/components/player/unified/ && bunx tsc --noEmit`
- [ ] Grafana JSON parses.
- [ ] `/animeenigma-after-update` — redeploy `analytics`, `gateway`, `player`(no—player not changed backend; web yes), `web` (CLEAN from origin/main worktree per Stage 1 lesson), and `grafana` (`make restart-grafana` to reload dashboards). Order: analytics + gateway first (endpoint exists), then web (starts emitting). Changelog: this is mostly invisible to users — a SHORT Trump-mode entry is fine ("📊 Теперь МЫ измеряем, какой источник реально работает у каждого аниме — фундамент для умного выбора. ЖОСКО.") or skip the user changelog and note it as internal.
- [ ] Runtime smoke: watch one title, then `docker compose exec` ClickHouse (or Grafana Explore) `SELECT effect_kind, target, count() FROM events WHERE effect_kind LIKE 'player_%' GROUP BY effect_kind, target` shows rows.

## Spec coverage (this plan)

| Spec section | Covered by |
|---|---|
| §2 Telemetry (player_resolve incl. reached-playback + player_stall) to ClickHouse | Tasks 1,3,4 |
| §2 one beacon covers all providers uniformly | Task 4 (emits with `state.combo.value.provider`) |
| §3 Widen the Playback/Health dashboard with real-user panels | Task 5 |

## Assumptions to confirm

- Reusing `events` columns + `effect_kind` (no migration) is acceptable (recommended; avoids a ClickHouse DDL change).
- `event_type='player'` is allowed by `Event.Validate()` (add additively if a whitelist rejects it).
- Player telemetry shares the same drop-on-full batcher as clickstream (acceptable — not billing data).
- No sampling for now (player events are sparse: ~1 resolve + a few stalls per watch).

---

## Stage 2b & 2c — outline (separate plans, drafted on request)

**Stage 2b — daily ranking pipeline (clone of `read_threshold`):**
- `services/scheduler/internal/jobs/provider_ranking.go` — daily cron (~`0 4 * * *`, sibling of `top_anime.go`) → POSTs `analytics /internal/player-ranking/recompute`.
- `services/analytics` — new `/internal/player-ranking/recompute` + a ClickHouse aggregate query (trailing 7d, recency-weighted, `HAVING count()>=N` min-sample) computing per-`(anime_id, provider)` + global per-provider score (primary = reached-playback rate; soft = resolve-success, p95 latency, stall rate). Publish to Redis (48h TTL, like `read_thresholds`). Files: `internal/service/player_ranking.go`, `internal/repo/clickhouse_store.go` (new query), router.
- `services/catalog` — read the published Redis ranking and include `dailyTop[]` (+ `firstParty`, `curatedTier`) in the per-anime source payload the player fetches.
- Verifiable on its own: the recompute endpoint + a `GET` returning the ranking.

**Stage 2c — ranking-aware default + same-day override + playback-time fallback:**
- `UnifiedPlayer.vue` — fold `dailyTop` + the Redis `srcfix:{animeId}` override into the decision order (saved combo → override → daily top → curated tier), via a `pickSmartDefault` variant that accepts the ranking.
- Same-day override: on a successful **resolve-time** client-side fallback (Stage 1b path), POST a tiny `srcfix` write (new catalog/analytics internal endpoint → Redis `srcfix:{animeId}`, 24h TTL); read it in the source payload.
- Playback-time fallback: promote the Stage 1b dead-source one-shot into the staged "suggestion → opt-in auto-advance" from the spec §7.
- Files: `UnifiedPlayer.vue`, a catalog/analytics `srcfix` endpoint, `useProviderHealth`/ranking merge.

## Effort & Impact (per `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better, indirect)** — 2a itself is invisible to users; it unlocks the +3 in 2c. Operators gain real provider-reliability visibility.
- **CDI = 0.05 * 13** — moderate spread (frontend beacon + analytics + gateway + Grafana), low shift (additive endpoint + reused table), contained effort.
- **MVQ = Griffin 88%/85%** — composes the proven `feErrorLog` beacon + `collect.go` ingestion + existing events table; slop-resistant (handler + beacon are independently unit-tested; no schema risk).
