# aePlayer Playback-Failure Telemetry + Alert — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When aePlayer tries and cannot play an episode (all providers exhausted, or the first-party `ae` source failed), automatically ship a full diagnostic bundle to ClickHouse, and page the maintenance channel when it happens more than once an hour.

**Architecture:** A pure classifier decides — from the current failure context — whether the moment is a terminal playback failure worth recording. `AePlayer.vue`'s `advanceToNextSource` calls it once per failure and, on a positive decision, emits a new `playback_failed` telemetry event (with the diagnostic bundle) through the existing `playerTelemetry.ts` → `POST /api/analytics/player-events` path. The analytics handler maps it to `effect_kind='player_failed'` in `analytics.events`. A Grafana ClickHouse rule counts those rows over a rolling hour and notifies `maintenance-webhook`.

**Tech Stack:** Vue 3 + TypeScript (Vitest) frontend; Go analytics service (ClickHouse sink); Grafana unified alerting (provisioned YAML).

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-11-aeplayer-playback-failure-alert-design.md` — the source of truth for behavior.
- **Never `git add -A`** in this repo — `.claude/worktrees/`, `tmp/`, and `services/scraper/scraper-api` are untracked-but-not-gitignored. Stage explicit paths only.
- **Every commit** ends with these three trailers, verbatim:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Work in the worktree** `/data/animeenigma/.claude/worktrees/aeplayer-failure-alert` — never edit via absolute `/data/animeenigma/...` (that hits the base tree).
- **First-party source id** = provider string `'ae'` (already in the analytics provider whitelist).
- **Terminal-failure telemetry is suppressed in hacker mode** and for the content-gap reason `source missing the requested episode`.
- FE tests: `bunx vitest run <path>` from `frontend/web`. Backend tests: `go test ./services/analytics/...` from repo root.
- Grafana provisioned-alert facts: active file is `docker/grafana/provisioning/alerting/rules.yml`; ClickHouse alert queries MUST use numeric `format: 1`; datasource uid `aenigma-clickhouse`; deploy = `make restart-grafana`.

---

### Task 1: FE telemetry — accept `playback_failed` events with a diagnostic `detail`

**Files:**
- Modify: `frontend/web/src/utils/playerTelemetry.ts`
- Test: `frontend/web/src/utils/playerTelemetry.spec.ts`

**Interfaces:**
- Produces: `recordPlayerEvent(e: PlayerEvent)` now accepts `kind: 'playback_failed'` and an optional `detail?: Record<string, unknown>` field that is serialized verbatim in the batch body. Existing `resolve`/`stall`/`playback_start_rejected` behavior unchanged.

- [ ] **Step 1: Write the failing test**

Add to `frontend/web/src/utils/playerTelemetry.spec.ts` (inside the existing top-level `describe`). `Blob` bodies are opaque in jsdom, so drive the fetch-keepalive fallback (stub a `navigator` with no `sendBeacon`) and read the JSON body off the `fetch` spy:

```ts
it('accepts playback_failed and serializes its detail bundle', async () => {
  vi.stubGlobal('navigator', {}) // no sendBeacon → fetch keepalive path
  const fetchSpy = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('fetch', fetchSpy)

  recordPlayerEvent({
    kind: 'playback_failed',
    provider: 'ae',
    anime_id: 'abc',
    episode: 3,
    error_kind: 'stream_error',
    detail: { reason: 'ae_failed', all_exhausted: false, engine: { bw_bps: 1234 } },
  })
  flushPlayerTelemetry('manual')

  expect(fetchSpy).toHaveBeenCalledTimes(1)
  const body = JSON.parse(fetchSpy.mock.calls[0][1].body as string)
  expect(body.events[0].kind).toBe('playback_failed')
  expect(body.events[0].detail.reason).toBe('ae_failed')
  expect(body.events[0].detail.engine.bw_bps).toBe(1234)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/utils/playerTelemetry.spec.ts -t "playback_failed"`
Expected: FAIL — `recordPlayerEvent` drops the event because `'playback_failed'` is not an allowed kind, so `fetch` is never called.

- [ ] **Step 3: Write minimal implementation**

In `frontend/web/src/utils/playerTelemetry.ts`, extend the interface and the kind guard.

Change the `PlayerEvent` interface's `kind` union and add `detail`:

```ts
export interface PlayerEvent {
  kind: 'resolve' | 'stall' | 'playback_start_rejected' | 'playback_failed'
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
  /** Rich diagnostic bundle for kind:'playback_failed' — serialized verbatim. */
  detail?: Record<string, unknown>
  ts?: string
}
```

In `recordPlayerEvent`, widen the kind validation:

```ts
    if (
      e.kind !== 'resolve' &&
      e.kind !== 'stall' &&
      e.kind !== 'playback_start_rejected' &&
      e.kind !== 'playback_failed'
    )
      return
```

No other change: `tryPush` already spreads the whole event, so `detail` flows into the batch body.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/utils/playerTelemetry.spec.ts`
Expected: PASS (the new test and all existing ones).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/playerTelemetry.ts frontend/web/src/utils/playerTelemetry.spec.ts
git commit -F - <<'EOF'
feat(aeplayer): accept playback_failed telemetry kind + diagnostic detail

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 2: Analytics handler — map `playback_failed` → `effect_kind='player_failed'` and merge `detail`

**Files:**
- Modify: `services/analytics/internal/handler/playertelemetry.go`
- Test: `services/analytics/internal/handler/playertelemetry_test.go`
- Verify only (no change): `services/analytics/internal/handler/playertelemetry_whitelist.go` — `'ae'` is present in `knownProviders`.

**Interfaces:**
- Consumes: wire events with `kind:'playback_failed'`, per-event `provider/anime_id/episode/audio/lang/error_kind`, and a `detail` object.
- Produces: one `analytics.events` row with `effect_kind='player_failed'`, `target=<provider>`, `anime_id`, and `properties` = the standard prop map merged with every key of `detail`.

- [ ] **Step 1: Write the failing test**

Add to `services/analytics/internal/handler/playertelemetry_test.go` (follow the existing test's construction of a handler over a fake sink — reuse whatever `newTestSink`/capture helper the file already defines; if the file captures via a `*captureSink` with an `Events()` accessor, mirror it):

```go
func TestPlayerTelemetry_PlaybackFailed(t *testing.T) {
	sink := newCaptureSink() // existing helper in this test file
	h := NewPlayerTelemetryHandler(sink)

	body := `{"events":[{
		"kind":"playback_failed","provider":"ae","anime_id":"a1","episode":3,
		"audio":"dub","lang":"en","error_kind":"stream_error",
		"detail":{"reason":"ae_failed","all_exhausted":false,"engine":{"bw_bps":1234}}
	}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/analytics/player-events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	evs := sink.Events()
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1", len(evs))
	}
	ev := evs[0]
	if ev.EffectKind != "player_failed" {
		t.Errorf("effect_kind = %q, want player_failed", ev.EffectKind)
	}
	if ev.Target != "ae" {
		t.Errorf("target = %q, want ae", ev.Target)
	}
	if !strings.Contains(ev.Properties, `"reason":"ae_failed"`) ||
		!strings.Contains(ev.Properties, `"bw_bps":1234`) {
		t.Errorf("properties missing merged detail: %s", ev.Properties)
	}
}
```

If the existing test file uses a different sink helper name, match it exactly (grep the file first).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/analytics/internal/handler/ -run TestPlayerTelemetry_PlaybackFailed -v`
Expected: FAIL — `playback_failed` hits the `default:` case and is skipped, so `len(evs) == 0`.

- [ ] **Step 3: Write minimal implementation**

In `services/analytics/internal/handler/playertelemetry.go`:

Add a `Detail` field to `wirePlayerEvent` (after the shared-metadata block):

```go
	// Rich diagnostic bundle for kind "playback_failed" — merged verbatim into
	// Properties. Bounded by the handler's 256 KB body limit.
	Detail json.RawMessage `json:"detail,omitempty"`
```

Add a case to the kind switch (alongside `resolve`/`stall`/`playback_start_rejected`):

```go
		case "playback_failed":
			// Terminal aePlayer failure (all providers exhausted, or the
			// first-party `ae` source failed). Duration is not meaningful here.
			effectKind = "player_failed"
```

Merge `detail` into the prop map. Replace the `propsBytes, _ := json.Marshal(propMap)` line with:

```go
		if len(we.Detail) > 0 {
			var detail map[string]any
			if err := json.Unmarshal(we.Detail, &detail); err == nil {
				for k, v := range detail {
					propMap[k] = v
				}
			}
		}
		propsBytes, _ := json.Marshal(propMap)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/analytics/internal/handler/ -run TestPlayerTelemetry -v`
Expected: PASS (new test + existing player-telemetry tests).

- [ ] **Step 5: Verify `ae` survives the whitelist**

Run: `grep -n '"ae"' services/analytics/internal/handler/playertelemetry_whitelist.go`
Expected: a match inside `knownProviders` (no code change needed). If — and only if — it is absent, add `"ae": {},` to the map and re-run Step 4.

- [ ] **Step 6: Commit**

```bash
git add services/analytics/internal/handler/playertelemetry.go services/analytics/internal/handler/playertelemetry_test.go
git commit -F - <<'EOF'
feat(analytics): ingest player_failed events, merge diagnostic detail into properties

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 3a: Pure playback-failure classifier

**Files:**
- Create: `frontend/web/src/components/player/aePlayer/playbackFailure.ts`
- Test: `frontend/web/src/components/player/aePlayer/playbackFailure.spec.ts`

**Interfaces:**
- Produces:
  - `EPISODE_GAP_REASON = 'source missing the requested episode'`
  - `classifyPlaybackFailure(i: FailureInputs): FailureDecision`
  - `mapErrorKind(reason: string): string`
  - types `FailureInputs`, `FailureDecision`, `FailureTag = 'ae_failed' | 'all_exhausted'`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/components/player/aePlayer/playbackFailure.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import {
  classifyPlaybackFailure,
  mapErrorKind,
  EPISODE_GAP_REASON,
  type FailureInputs,
} from './playbackFailure'

const base: FailureInputs = {
  reason: 'resolve failed',
  failingProvider: 'gogoanime',
  hackerMode: false,
  roomPinned: false,
  providerAutoSelected: true,
  candidateExists: true,
  attemptsExceeded: false,
}

describe('classifyPlaybackFailure', () => {
  it('does not emit while a candidate exists (failover will recover)', () => {
    expect(classifyPlaybackFailure(base).emit).toBe(false)
  })

  it('emits all_exhausted when the auto chain has no candidate left', () => {
    const d = classifyPlaybackFailure({ ...base, candidateExists: false })
    expect(d).toEqual({ emit: true, tag: 'all_exhausted', exhausted: true })
  })

  it('emits all_exhausted when the switch-attempt cap is hit', () => {
    const d = classifyPlaybackFailure({ ...base, attemptsExceeded: true })
    expect(d).toEqual({ emit: true, tag: 'all_exhausted', exhausted: true })
  })

  it('emits ae_failed when the first-party source fails, even with a candidate', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae' })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: false })
  })

  it('emits ae_failed (exhausted) when ae was the last candidate', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae', candidateExists: false })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: true })
  })

  it('never emits in hacker mode', () => {
    expect(classifyPlaybackFailure({ ...base, candidateExists: false, hackerMode: true }).emit).toBe(false)
    expect(classifyPlaybackFailure({ ...base, failingProvider: 'ae', hackerMode: true }).emit).toBe(false)
  })

  it('never emits for the content-gap reason', () => {
    expect(classifyPlaybackFailure({ ...base, candidateExists: false, reason: EPISODE_GAP_REASON }).emit).toBe(false)
    expect(classifyPlaybackFailure({ ...base, failingProvider: 'ae', reason: EPISODE_GAP_REASON }).emit).toBe(false)
  })

  it('does not emit for a manual non-ae pick failure', () => {
    expect(classifyPlaybackFailure({ ...base, providerAutoSelected: false, candidateExists: false }).emit).toBe(false)
  })

  it('emits ae_failed for a manual ae pick failure', () => {
    const d = classifyPlaybackFailure({ ...base, failingProvider: 'ae', providerAutoSelected: false })
    expect(d).toEqual({ emit: true, tag: 'ae_failed', exhausted: false })
  })
})

describe('mapErrorKind', () => {
  it('maps known failure reasons', () => {
    expect(mapErrorKind('silent stall')).toBe('stall_timeout')
    expect(mapErrorKind('playback fatal')).toBe('playback_fatal')
    expect(mapErrorKind('resolve failed')).toBe('stream_error')
    expect(mapErrorKind('anything else')).toBe('stream_error')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/playbackFailure.spec.ts`
Expected: FAIL — module `./playbackFailure` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `frontend/web/src/components/player/aePlayer/playbackFailure.ts`:

```ts
// Pure decision logic for aePlayer terminal playback-failure telemetry.
// Kept framework-free so it is unit-testable without mounting the player.
// See docs/superpowers/specs/2026-07-11-aeplayer-playback-failure-alert-design.md.

export type FailureTag = 'ae_failed' | 'all_exhausted'

/** The `advanceToNextSource` reason for a source simply not carrying the
 *  requested episode (incl. ae's partial library / a not-yet-aired episode).
 *  This is a content/scheduling gap, NOT a playback failure. */
export const EPISODE_GAP_REASON = 'source missing the requested episode'

export interface FailureInputs {
  /** The `advanceToNextSource` reason string. */
  reason: string
  /** The provider being abandoned (`state.combo.value.provider`). */
  failingProvider: string
  hackerMode: boolean
  roomPinned: boolean
  providerAutoSelected: boolean
  /** Whether the auto-failover chain still has an untried candidate. */
  candidateExists: boolean
  /** Whether the per-attempt switch cap has been reached. */
  attemptsExceeded: boolean
}

export interface FailureDecision {
  emit: boolean
  tag?: FailureTag
  exhausted?: boolean
}

/** Decide whether this failure is a terminal, alert-worthy playback failure. */
export function classifyPlaybackFailure(i: FailureInputs): FailureDecision {
  // Owner debugging / content gaps never count.
  if (i.hackerMode) return { emit: false }
  if (i.reason === EPISODE_GAP_REASON) return { emit: false }

  const isAe = i.failingProvider === 'ae'
  // Will the auto chain actually switch to another source?
  const willSwitch =
    i.candidateExists && i.providerAutoSelected && !i.roomPinned && !i.attemptsExceeded
  // The auto chain ran out → the viewer sees the error overlay.
  const exhausted = !willSwitch && i.providerAutoSelected && !i.roomPinned

  if (isAe) return { emit: true, tag: 'ae_failed', exhausted }
  if (exhausted) return { emit: true, tag: 'all_exhausted', exhausted: true }
  return { emit: false }
}

/** Map an `advanceToNextSource` reason to a coarse error_kind. */
export function mapErrorKind(reason: string): string {
  if (reason === 'silent stall') return 'stall_timeout'
  if (reason === 'playback fatal') return 'playback_fatal'
  return 'stream_error'
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/playbackFailure.spec.ts`
Expected: PASS (all cases).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/playbackFailure.ts frontend/web/src/components/player/aePlayer/playbackFailure.spec.ts
git commit -F - <<'EOF'
feat(aeplayer): pure classifier for terminal playback-failure telemetry

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 3b: Wire the classifier + diagnostic bundle into `AePlayer.vue`

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.playbackfailed.spec.ts` (create)

**Interfaces:**
- Consumes: `classifyPlaybackFailure`, `mapErrorKind`, `FailureInputs` (Task 3a); `recordPlayerEvent` (Task 1).
- Produces: a `playback_failed` event via `recordPlayerEvent` at most once per `(tag, anime_id, episode)` per failure streak, carrying the diagnostic bundle in `detail`.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.playbackfailed.spec.ts`. Model the mocks on `AePlayer.aeLangRouting.spec.ts` (same file directory), with ONE provider whose `resolveStream` rejects, so the very first failure exhausts the chain. Spy on `recordPlayerEvent`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

const recordSpy = vi.fn()
vi.mock('@/utils/playerTelemetry', () => ({
  recordPlayerEvent: (...a: unknown[]) => recordSpy(...a),
  flushPlayerTelemetry: vi.fn(),
}))

const gogoCap = {
  provider: 'gogoanime', display_name: 'GogoAnime',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 100, group: 'en' as const, audios: ['sub'] as ('sub' | 'dub')[],
}
const report: CapabilityReport = {
  anime_id: 'anime-uuid',
  families: [{ family: 'others', providers: [gogoCap] }],
}

vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({
    report: ref(report),
    capMap: ref(new Map<string, ProviderCap>([['gogoanime', gogoCap]])),
  }),
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: vi.fn().mockResolvedValue([{ key: 1, label: 1, number: 1 }]),
    listTeams: vi.fn().mockResolvedValue([]),
    resolveStream: vi.fn().mockRejectedValue(new Error('boom')), // always fails
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
// Reuse the engine/other mocks from AePlayer.aeLangRouting.spec.ts verbatim
// (useVideoEngine, useWatchPreferences, router, i18n, etc.). Copy that spec's
// full mock preamble here so the component mounts.

describe('AePlayer terminal playback-failure telemetry', () => {
  beforeEach(() => recordSpy.mockClear())

  it('emits playback_failed with reason all_exhausted when the only provider fails', async () => {
    const wrapper = mount(/* AePlayer with the standard props from aeLangRouting spec */)
    await flushPromises()

    const failed = recordSpy.mock.calls
      .map((c) => c[0])
      .filter((e: { kind: string }) => e.kind === 'playback_failed')
    expect(failed.length).toBe(1)
    expect(failed[0].provider).toBe('gogoanime')
    expect(failed[0].detail.reason).toBe('all_exhausted')
    expect(failed[0].detail.all_exhausted).toBe(true)
    expect(failed[0].detail.engine).toBeTruthy()
  })
})
```

Note for the implementer: copy the complete mock preamble and the `mount(AePlayer, { props: … })` call from `AePlayer.aeLangRouting.spec.ts` so the component has every dependency it needs; only the three mocks shown above (telemetry spy, single failing provider) differ.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.playbackfailed.spec.ts`
Expected: FAIL — no `playback_failed` event is recorded yet.

- [ ] **Step 3: Add imports + module state in `AePlayer.vue`**

Near the other imports (by `import { recordPlayerEvent } from '@/utils/playerTelemetry'`, line ~541):

```ts
import {
  classifyPlaybackFailure,
  mapErrorKind,
  type FailureInputs,
} from './playbackFailure'
```

Near the failover state (`const triedSources = new Set<string>()`, line ~812), add:

```ts
// Terminal playback-failure telemetry state. `attemptTrail` is the cross-source
// failover history (the engine's edgeTrail only covers per-CDN edges within one
// source); `emittedFailureKeys` de-dups so a retry streak on one broken episode
// records at most one row per (tag, episode).
const attemptTrail: Array<{ provider: string; server: string; reason: string }> = []
const emittedFailureKeys = new Set<string>()
```

- [ ] **Step 4: Reset the new state in `resetSourceSwitching`**

Replace `resetSourceSwitching` (line ~821) with:

```ts
function resetSourceSwitching() {
  triedSources.clear()
  sourceSwitchAttempts = 0
  switchingSource = false
  attemptTrail.length = 0
  emittedFailureKeys.clear()
}
```

- [ ] **Step 5: Add the bundle builder + reporter helpers**

Immediately after `resetSourceSwitching`, add:

```ts
// Full diagnostic bundle ("all logs"), assembled regardless of hacker mode
// (debugStats stays hacker-gated for the HUD; this is the always-on copy).
function buildDiagnosticBundle() {
  const frs = engine.fragStats.value
  const last = frs[frs.length - 1]
  const edge = engine.servedEdge.value
  const trail = engine.edgeTrail.value
  return {
    combo: { ...state.combo.value },
    engine: {
      bw_bps: engine.bandwidthEstimate.value,
      level:
        engine.currentLevelLabel.value ||
        (currentStream.value?.type === 'mp4' ? 'mp4' : ''),
      frag_size_kb: last ? Math.round(last.size / 1024) : 0,
      frag_load_ms: last ? Math.round(last.loadMs) : 0,
      served_edge: edge,
      edge_trail: edge ? formatEdgeTrail(trail) : '',
      edge_rotations: edge && trail ? trail.split(',').length - 1 : 0,
      buffer_ahead_s: playbackStats.value.bufferAheadSec,
      buffer_behind_s: playbackStats.value.bufferBehindSec,
      video_ready_state: videoRef.value?.readyState ?? 0,
    },
    attempt_trail: attemptTrail.slice(-30),
    capability_snapshot: rows.value
      .slice(0, 40)
      .map((r) => ({ provider: r.id, group: r.group, state: r.state })),
    client: {
      ua: typeof navigator !== 'undefined' ? navigator.userAgent : '',
      viewport:
        typeof window !== 'undefined' ? `${window.innerWidth}x${window.innerHeight}` : '',
      connection:
        (typeof navigator !== 'undefined' &&
          (navigator as { connection?: { effectiveType?: string } }).connection
            ?.effectiveType) ||
        '',
    },
  }
}

// Classify the current failure; on a positive, de-duped decision, emit one
// playback_failed telemetry event with the diagnostic bundle.
function reportIfTerminal(inputs: FailureInputs) {
  const d = classifyPlaybackFailure(inputs)
  if (!d.emit || !d.tag) return
  const key = `${d.tag}:${props.animeId}:${selectedEpisode.value?.number ?? ''}`
  if (emittedFailureKeys.has(key)) return
  emittedFailureKeys.add(key)
  recordPlayerEvent({
    kind: 'playback_failed',
    provider: inputs.failingProvider,
    anime_id: props.animeId,
    episode: selectedEpisode.value?.number,
    audio: state.combo.value.audio,
    lang: state.combo.value.lang,
    error_kind: mapErrorKind(inputs.reason),
    detail: {
      ...buildDiagnosticBundle(),
      reason: d.tag,
      all_exhausted: d.exhausted ?? false,
      is_first_party: inputs.failingProvider === 'ae',
      fail_reason: inputs.reason,
    },
  })
}
```

- [ ] **Step 6: Refactor `advanceToNextSource` to hoist candidate computation and call `reportIfTerminal`**

Replace the body of `advanceToNextSource` (lines ~845-909) with the version below. It computes the candidate up front (unchanged logic), fires `reportIfTerminal` once from pre-mutation state, then runs the original control flow reusing the hoisted vars:

```ts
async function advanceToNextSource(reason: string): Promise<boolean> {
  const failingProvider = state.combo.value.provider
  const server = state.combo.value.server || resolvedServers.value[0]?.id || ''
  const curKey = `${failingProvider}:${server}`

  // Compute the next candidate WITHOUT committing — hacker mode only records the
  // intent, and the failure classifier needs to know if any candidate remains.
  const triedWithCurrent = new Set(triedSources)
  triedWithCurrent.add(curKey)
  const nextServer = resolvedServers.value.find(
    (s) => !triedWithCurrent.has(`${failingProvider}:${s.id}`),
  )
  let toProvider: string | null = null
  let switchServerId: string | null = null
  if (nextServer) {
    toProvider = failingProvider
    switchServerId = nextServer.id
  } else {
    const triedProviders = new Set([...triedWithCurrent].map((k) => k.split(':')[0]))
    toProvider = pickSmartDefault(rows.value.filter((r) => !triedProviders.has(r.id)))?.id ?? null
  }
  const candidateExists = Boolean(switchServerId) || Boolean(toProvider)

  // Record the cross-source failover history, then decide (from pre-mutation
  // state) whether this is a terminal, alert-worthy playback failure.
  attemptTrail.push({ provider: failingProvider, server, reason })
  reportIfTerminal({
    reason,
    failingProvider,
    hackerMode: state.hackerMode.value,
    roomPinned: roomPinned.value,
    providerAutoSelected: providerAutoSelected.value,
    candidateExists,
    attemptsExceeded: sourceSwitchAttempts >= MAX_SOURCE_SWITCHES,
  })

  // ── Original control flow (unchanged) ──────────────────────────────────────
  if (roomPinned.value) return false
  if (!providerAutoSelected.value) return false
  if (sourceSwitchAttempts >= MAX_SOURCE_SWITCHES) return false

  const provider = failingProvider
  if (state.hackerMode.value) {
    recordFallbackIntent({ from: provider, to: toProvider, reason, acted: false })
    const target = switchServerId ? `${provider} · server ${switchServerId}` : toProvider
    sourceError.value = target
      ? `Hacker mode: auto-switch suppressed — would try ${target} (${reason}). Pick a source manually.`
      : `Hacker mode: auto-switch suppressed — ${provider} failed (${reason}), no fallback candidate.`
    return false
  }

  if (!toProvider) return false

  triedSources.add(curKey)
  sourceSwitchAttempts++
  switchingSource = true
  if (switchServerId) {
    state.setServer(switchServerId)
  } else {
    providerAutoSelected.value = true
    state.setProvider(toProvider, '')
  }
  recordFallbackIntent({ from: provider, to: toProvider, reason, acted: true })
  recordDecision(
    switchServerId
      ? `auto-failover — switched server (${reason})`
      : `auto-failover — previous source failed (${reason})`,
  )
  return true
}
```

- [ ] **Step 7: Run the new spec + the touched existing specs**

Run: `cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.playbackfailed.spec.ts src/components/player/aePlayer/__tests__/AePlayer.aeLangRouting.spec.ts src/components/player/aePlayer/playbackFailure.spec.ts`
Expected: PASS. If the mount test cannot deterministically drive the failure, assert the wiring by unit-testing `classifyPlaybackFailure` (already covered) and keep the mount test as a smoke check that `recordPlayerEvent` is invoked with `kind:'playback_failed'` after `flushPromises()`.

- [ ] **Step 8: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit -p tsconfig.app.json 2>&1 | head -30`
Expected: no new errors referencing `AePlayer.vue` or `playbackFailure.ts`.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/__tests__/AePlayer.playbackfailed.spec.ts
git commit -F - <<'EOF'
feat(aeplayer): emit playback_failed telemetry on exhausted chain / ae failure

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 4: Grafana alert — `AePlayerPlaybackFailures` (>1/hour → maintenance channel)

**Files:**
- Modify: `docker/grafana/provisioning/alerting/rules.yml` (the ACTIVE provisioned rules)
- Modify: `infra/grafana/alerts/` — mirror the rule into the human source-of-truth copy (match the existing file used for scraper/anomaly rules; grep for `ScraperStreamCascadeLatency` under `infra/grafana/alerts/` to find it)

**Interfaces:**
- Consumes: `analytics.events` rows with `effect_kind='player_failed'` (Task 2).

- [ ] **Step 1: Add the rule to `rules.yml`**

Insert this block into `docker/grafana/provisioning/alerting/rules.yml`, immediately after the `ScraperStreamCascadeLatency` rule (after its `reason: stream_cascade_latency` annotation line, before `- uid: provider-fleet-no-auto-playable`). Keep the surrounding indentation identical to the sibling rules:

```yaml
      # AePlayerPlaybackFailures (warning) — >1 terminal aePlayer failure in 1h
      - uid: aeplayer-playback-failures
        title: AePlayerPlaybackFailures
        noDataState: OK
        execErrState: Error
        condition: C
        data:
          - refId: A
            relativeTimeRange:
              from: 3600
              to: 0
            datasourceUid: aenigma-clickhouse
            model:
              refId: A
              datasource:
                type: grafana-clickhouse-datasource
                uid: aenigma-clickhouse
              queryType: sql
              format: 1
              intervalMs: 1000
              maxDataPoints: 43200
              rawSql: |
                SELECT count() AS value
                FROM analytics.events
                WHERE effect_kind = 'player_failed'
                  AND timestamp >= now() - INTERVAL 1 HOUR
          - refId: B
            relativeTimeRange:
              from: 3600
              to: 0
            datasourceUid: __expr__
            model:
              refId: B
              type: reduce
              expression: A
              reducer: last
          - refId: C
            relativeTimeRange:
              from: 3600
              to: 0
            datasourceUid: __expr__
            model:
              refId: C
              type: threshold
              expression: B
              conditions:
                - evaluator:
                    params: [1]
                    type: gt
                  operator:
                    type: and
        for: 0s
        labels:
          severity: warning
          component: aeplayer
          reason: playback_failed
        annotations:
          summary: 'aePlayer playback failures: {{ $values.B }} viewers could not watch in the last hour'
          description: 'More than one terminal aePlayer playback failure in the last hour (all providers exhausted, or the first-party `ae` source failed). Drill down in ClickHouse: SELECT properties FROM analytics.events WHERE effect_kind = ''player_failed'' AND timestamp >= now() - INTERVAL 1 HOUR ORDER BY timestamp DESC.'
          reason: playback_failed
```

- [ ] **Step 2: Verify the routing already covers this rule**

Run: `grep -n "receiver: maintenance-webhook\|severity" docker/grafana/provisioning/alerting/policies.yml`
Expected: the default route (or a `severity` match) delivers `warning`/`component` alerts to `maintenance-webhook`. The `ScraperStreamCascadeLatency` (also `severity: warning`) already routes there, so no policy change is needed. If `ScraperStreamCascadeLatency` were muted (it is not — only the `severity: diagnostic` per-provider rules route through the `always` mute timing), this rule must NOT carry `severity: diagnostic`. Confirm this rule uses `severity: warning`.

- [ ] **Step 3: Mirror into the infra source-of-truth**

Run: `grep -rln "ScraperStreamCascadeLatency" infra/grafana/alerts/`
Then add the same rule (Step 1 block, matching that file's YAML shape) to the file it reports. This copy is documentation/keep-in-sync only — it is not loaded by Grafana.

- [ ] **Step 4: Validate the rule non-destructively against live Grafana**

Grafana admin is at `127.0.0.1:3004` (`admin` / `$GRAFANA_ADMIN_PASSWORD` from `docker/.env`). Post the single rule via the provisioning API with provenance disabled, poll for health, then delete it:

```bash
set -a; . docker/.env; set +a
GRAF="http://admin:${GRAFANA_ADMIN_PASSWORD}@127.0.0.1:3004"
# Build the one-rule provisioning payload from the YAML you added (uid aeplayer-playback-failures).
curl -s -XPOST "$GRAF/api/v1/provisioning/alert-rules" \
  -H 'Content-Type: application/json' -H 'X-Disable-Provenance: true' \
  -d @/tmp/claude-0/-data-animeenigma/95c1b707-02c9-414e-8c7e-8d0136d64788/scratchpad/aeplayer-alert.json
sleep 3
curl -s "$GRAF/api/prometheus/grafana/api/v1/rules" | grep -o '"name":"AePlayerPlaybackFailures"[^}]*"health":"[a-z]*"'
# Expect health:"ok". Then remove the probe rule:
curl -s -XDELETE "$GRAF/api/v1/provisioning/alert-rules/aeplayer-playback-failures" -H 'X-Disable-Provenance: true'
```

Write the JSON body (`/tmp/claude-0/-data-animeenigma/95c1b707-02c9-414e-8c7e-8d0136d64788/scratchpad/aeplayer-alert.json`) as the single-rule provisioning shape (title, `condition: C`, the same 3 `data` refs, `folderUID`/`ruleGroup` matching a sibling — read one back with `curl -s "$GRAF/api/v1/provisioning/alert-rules/scraper-stream-cascade-latency"` to copy the envelope). Expected: the poll prints `"health":"ok"`.

- [ ] **Step 5: Commit**

```bash
git add docker/grafana/provisioning/alerting/rules.yml infra/grafana/alerts/
git commit -F - <<'EOF'
feat(grafana): alert on >1 aePlayer playback failure per hour → maintenance channel

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

### Task 5: Full verification + rollout

**Files:** none (build/deploy).

- [ ] **Step 1: FE gate**

Run: `bin/ae-fe-verify.sh frontend/web/src/utils/playerTelemetry.ts frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/playbackFailure.ts`
Expected: DS-lint + eslint(touched) + `bun run build` + touched vitest specs all green.

- [ ] **Step 2: Backend gate**

Run: `go test ./services/analytics/... && go build ./services/analytics/...`
Expected: PASS / clean build.

- [ ] **Step 3: Land** (via the standard flow — commit history already atomic)

Run: `bin/ae-land.sh` is not needed if already committed; instead rebase + push:
`git -C /data/animeenigma/.claude/worktrees/aeplayer-failure-alert pull --rebase origin main && git push origin HEAD:main`
Expected: fast-forward push to `main`. On conflict, STOP and resolve by path.

- [ ] **Step 4: Deploy**

Run: `bin/ae-deploy.sh analytics web` then `make restart-grafana`
Expected: `animeenigma-analytics` + `animeenigma-web` containers Up; Grafana reloaded with the new rule (`curl -s "$GRAF/api/prometheus/grafana/api/v1/rules" | grep -c AePlayerPlaybackFailures` → `1`).

- [ ] **Step 5: Live smoke (optional)**

Watch a title with no working source (or temporarily force a failure) and confirm a `player_failed` row lands:
`clickhouse-client -q "SELECT effect_kind, target, JSONExtractString(properties,'reason') FROM analytics.events WHERE effect_kind='player_failed' AND timestamp > now() - INTERVAL 10 MINUTE"` (via the analytics/clickhouse container).

---

## Self-Review

- **Spec coverage:** Trigger A/B (Task 3a classifier + 3b wiring) · hacker-mode + content-gap exclusion (3a) · dedup (3b `emittedFailureKeys`) · diagnostic bundle §4 (3b `buildDiagnosticBundle`) · backend `player_failed` + detail merge (Task 2) · `ae` whitelist (Task 2 Step 5) · alert ≥2/1h → maintenance-webhook (Task 4). All covered.
- **Type consistency:** `FailureInputs`/`FailureDecision`/`FailureTag` defined in Task 3a and consumed unchanged in 3b; `recordPlayerEvent` `detail` field defined in Task 1 and used in 3b; backend `Detail json.RawMessage` matches the FE `detail` object key.
- **No placeholders:** every code/step is concrete except the two explicit "copy the sibling's shape" instructions (the AePlayer mount-mock preamble and the Grafana provisioning envelope), which are unavoidable reuse of existing fixtures and are pointed at exact source files.
