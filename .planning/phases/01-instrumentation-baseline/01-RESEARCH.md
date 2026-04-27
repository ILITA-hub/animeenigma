# Phase 1: Instrumentation Baseline - Research

**Researched:** 2026-04-27
**Domain:** Frontend instrumentation (Vue composable) + Go service handler + Prometheus metric + Grafana dashboard panel
**Confidence:** HIGH

## Summary

Phase 1 wires a single Vue composable (`useOverrideTracker.ts`) into all four player components so that when a user changes language / player / team / episode within 30 seconds of player mount, a `combo_override_total` Prometheus counter is incremented and a structured `combo_override` log line lands in Loki. A matching `combo_resolve_total` counter is incremented from the existing `services/player/internal/service/preference.go` resolver to provide the rate denominator. A new panel is added to the existing `docker/grafana/dashboards/preference-resolution.json` showing override rate segmented by tier × language × anon-vs-auth × player × dimension.

All infrastructure already exists: `libs/metrics/watch.go` is the home for the new `CounterVec`s (mirroring `TranslationSelectionsTotal`); `libs/logger` is structured zap that already lands in Loki via promtail's docker container scrape; Grafana auto-loads dashboards from `docker/grafana/dashboards/` via file provider; `promauto` self-registers — no `main.go` change needed except handler/route wiring. The only architecturally novel piece is the anonymous endpoint (`POST /api/preferences/override` — note: NOT `/api/users/preferences/override` — see Critical Finding 1 below) and the `X-Anon-ID` header propagation through the axios client.

**Primary recommendation:** Mount the new endpoint OUTSIDE the gateway's `/api/users/*` JWT-protected group; introduce an `OptionalAuth` middleware in the player service that reads JWT if present and `X-Anon-ID` otherwise; emit both counter and log line atomically from the handler. Use `crypto.randomUUID()` (browser-native, no new dep) for `anon_id` and `load_session_id`. Debounce 250 ms and gate on user-initiated picker events (NOT raw watch handlers) to avoid auto-advance false positives.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Detect user-initiated combo change | Browser / Client (Vue composable) | — | Only the browser sees picker click events; backend would only see prop deltas which is what auto-advance also produces |
| Mint `load_session_id` and `anon_id` | Browser / Client | — | Per-mount lifecycle and localStorage persistence are client concerns |
| Persist `anon_id` across visits | Browser / Client (localStorage) | — | No server identity for anon users; D-13 explicitly defers cross-device join |
| Emit `combo_override_total` counter | API / Backend (player service) | — | Prometheus client lives in Go services; promauto self-registration |
| Emit `combo_resolve_total` counter | API / Backend (player service) | — | The resolver runs on the backend; counter is a single line in `preference.go` |
| Structured Loki log line | API / Backend (player service) | — | promtail scrapes container stdout; backend `log.Infow` is the canonical path |
| Gateway routing for new endpoint | API / Gateway | — | Gateway already proxies `/api/*` → upstream services; new path needs explicit registration |
| Grafana panel rendering override rate | CDN / Static (provisioned dashboard JSON) | — | Dashboard JSON is shipped as code via `docker/grafana/dashboards/`, mounted read-only |
| 30-second window timer | Browser / Client | — | Client owns mount lifecycle; backend only sees the resulting POST |

**Tier sanity check:** No persistence to PostgreSQL (D-04 explicitly avoids new GORM table). No Redis cache. No CDN/edge work. Single-source-of-truth boundary: the composable is the *only* place that decides "this counts as an override."

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Detection Placement
- **D-01:** A single Vue composable — `frontend/web/src/composables/useOverrideTracker.ts` — is imported by all four player components (`KodikPlayer.vue`, `AnimeLibPlayer.vue`, `HiAnimePlayer.vue`, `ConsumetPlayer.vue`). The composable watches the props/refs that drive the player (language, watch_type, team/translation_title, current episode) and detects user-initiated changes. No per-player divergence.
- **D-02:** Detection happens at the Vue prop / reactive-state level, not inside the iframe. **Kodik IS covered** because combo changes flow through the Vue parent.
- **D-03:** On detection, the composable POSTs to a new endpoint on the player service: `POST /api/users/preferences/override`. Accepts both JWT-authenticated requests (user_id from claims) and anonymous requests (X-Anon-ID header from localStorage). One handler, one source of truth.

#### Storage Shape
- **D-04:** Two write paths reusing existing infrastructure — no new GORM table:
  1. Prometheus `CounterVec` `combo_override_total` in `libs/metrics/watch.go` with labels `{tier, dimension, language, anon, player}`
  2. Structured Loki log line `log.Infow("combo_override", ...)` with same labels plus `anon_id`/`user_id`, `anime_id`, `load_session_id`, `original_combo`, `new_combo`, `ms_since_load`
- **D-05:** Matching `combo_resolve_total` counter on every successful resolve — the denominator. Resolve handler emits; composable does NOT.
- **D-06:** Loki retention ~31 days (per `docker/loki/loki-config.yml`). Sufficient for 24h baseline + Phase 7 before/after.

#### What Counts as an Override
- **D-07:** "First user-initiated change per `(load_session_id, dimension)` within 30s of player mount." `load_session_id` is UUIDv4 generated on mount. `dimension` ∈ `{language, player, team, episode}`. Only the FIRST change per dimension per session counts.
- **D-08:** Excludes auto-advance, scrubbing, pause/resume, quality switches. Distinguish via Vue event from user-facing picker UI, NOT raw prop changes.
- **D-09:** Re-visiting same anime mints a new `load_session_id` → fresh 30s window.
- **D-10:** 30s window starts when the resolved combo is applied to the player props.

#### Anonymous User Identity
- **D-11:** UUIDv4 stored as `anon_id` in localStorage key `aenig_anon_id`. Composable adds `X-Anon-ID: <uuid>` to both resolve call and override emit for any user without a JWT.
- **D-12:** Unlocks per-anon-user override rate.
- **D-13:** Pulled forward from Phase 7 D-01 — Phase 7 inherits the infrastructure.
- **D-14:** No PII. UUIDv4 only. Cleared with cookies/localStorage.

#### Grafana Tile
- **D-15:** Add to existing `docker/grafana/dashboards/preference-resolution.json`. New panel: "Auto-Pick Override Rate" with 5 segmentations: tier, language, anon-vs-auth, player, dimension.
- **D-16:** Tile refreshes within 1 minute. PromQL `rate(combo_override_total[5m]) / rate(combo_resolve_total[5m])` segmented.
- **D-17:** Provisioning: dashboard JSON checked in to `docker/grafana/dashboards/`, auto-loaded. Verifying live = open `https://admin.animeenigma.ru/grafana` after `make redeploy-player`.

### Claude's Discretion
- Exact composable API shape (return values, lifecycle hooks)
- Whether `combo_resolve` emits on cache-hit vs cold resolve only — recommendation: emit on every resolve outcome
- Endpoint name `POST /api/users/preferences/override` — planner may rename if closer convention exists
- Debounce 250ms, ignore if dimension already emitted in this session

### Deferred Ideas (OUT OF SCOPE)
- **Per-event DB table** — deferred to Phase 5 (Analytics Gap-Fill)
- **Override reason capture** — out of scope; revisit at Phase 7 Advanced Settings
- **A/B segmentation of resolver** — out of scope; revisit Phase 6
- **Cross-device join of anon → auth** — privacy-sensitive; defer
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| M-01 | Emit a `combo_override` event when a user changes language / player / team / episode within 30s of player load | Composable architecture (§ Pattern 1, 2), existing player picker events identified for all 4 players (§ Player Component Integration), counter + log line wired in handler (§ Existing Metric Registration) |
| M-02 | Grafana dashboard tile for override-rate, segmented by tier, language, anonymous/auth, and player. Baseline current state before B/D land; target < 10% override after the overhaul | Existing dashboard JSON pattern documented (§ Grafana Dashboard JSON), PromQL ratio expression with multi-label segmentation (§ Pattern 3), provisioning auto-reload mechanism confirmed (§ Pitfall 4) |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Frontend tooling:** `bun` (not npm/pnpm). For CLI: `bunx eslint`, `bunx tsc --noEmit`, `bunx playwright test`.
- **Backend tooling:** `go test ./...` for unit, `go test -tags=integration ./...` for integration.
- **Deployment:** `make redeploy-player` (and `make redeploy-gateway` if router changes are needed). After-update skill (`/animeenigma-after-update`) MUST run after implementation: lint → build → redeploy → update `frontend/web/public/changelog.json` → commit with co-authors → push.
- **Co-authors required:** `Claude Opus 4.6 <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`, `NANDIorg <super.egor.mamonov@yandex.ru>`.
- **Conventions:** Snake_case Go files, PascalCase types, camelCase variables. Use `libs/logger` zap (`Infow`, `Errorw` with key/value pairs). Use `libs/metrics` `promauto.NewCounterVec` (no manual `MustRegister`). Use `libs/httputil` `Bind`, `OK`, `Error`, `Unauthorized`, `BadRequest`. GORM AutoMigrate for schema (N/A this phase — no schema change).
- **Test user:** `ui_audit_bot` for E2E. Do NOT recreate; use seeded data. API key in `docker/.env`.
- **Production server:** This server IS production. `make redeploy` deploys live.

## Critical Findings (READ FIRST)

These three findings deviate from CONTEXT.md and require planner action:

### Critical Finding 1: `/api/users/*` is JWT-PROTECTED at the gateway

**File:** `services/gateway/internal/transport/router.go:148-151`
```go
// Player service routes (protected)
r.Group(func(r chi.Router) {
    r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
    r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)
})
```

`JWTValidationMiddleware` rejects with 401 when no `Authorization` header is present (`router.go:237-241`). CONTEXT.md D-03 specifies `POST /api/users/preferences/override` as accepting both JWT and anonymous requests, but **anonymous requests will be rejected at the gateway before they ever reach the player service**.

**Two viable resolutions:**

1. **(Recommended) Move the endpoint outside `/api/users/*`** — register it as `POST /api/preferences/override` at the gateway (alongside `/api/anime/{animeId}/reviews` which uses the same pattern), and at the player service register it OUTSIDE the protected `/users/*` group. Use a new `OptionalAuth` middleware that reads JWT if present and `X-Anon-ID` otherwise. The existing pattern for public-then-protected within a path is in `router.go:117-122` (anime reviews) — same shape.

2. **Add a JWT-bypass for the specific override path at the gateway** — fragile and breaks the "all of `/users/*` requires auth" invariant. Not recommended.

**Recommendation:** Use option 1. Endpoint becomes `POST /api/preferences/override`. The gateway gets one new line; the player service gets the route registered outside the auth group. The composable points axios at `/preferences/override` (relative to `/api`).

**Verified:** [VERIFIED: gateway router.go:148-151 + middleware.go:237-241]

### Critical Finding 2: Loki retention is **7 days**, not 31 days

**File:** `docker/loki/loki-config.yml:27-28`
```yaml
limits_config:
  retention_period: 168h  # 7 days
```

CONTEXT.md D-06 states "Loki retention is ~31 days" — the actual config is **168h (7 days)**.

**Impact:**
- Phase 1 success criterion 3 ("≥ 24 hours of real traffic baseline") is fine.
- Phase 6 / 7 retroactive per-event analysis from Loki is constrained to last 7 days, NOT last 30.
- The Prometheus counter persists separately (Prometheus retention is 15 days per `INTEGRATIONS.md` § Metrics) and is the durable source for rate over time. The Loki line is only valuable for "what was the original_combo / new_combo on a specific override event" — and only for the last 7 days.

**Action for planner:**
1. Either update `docker/loki/loki-config.yml` to `retention_period: 720h  # 30 days` AND increase the loki_data volume budget — OR
2. Accept 7-day retention and document the constraint in PROJECT.md alongside the baseline snapshot. If Phase 6 needs > 7d of per-event detail, that becomes a Phase 5 schema-add as D-06 already anticipates ("If Phase 6 needs > 30 days of per-event detail for time-decay validation, that becomes a Phase 5 schema-add — explicitly out of scope here").

**Recommendation:** Option 2 — accept 7d, document in PROJECT.md. The Prometheus counter at 15d retention covers the rate-over-time use case, which is what Phase 7 success criterion 5 needs ("measurable drop versus the Phase 1 baseline").

**Verified:** [VERIFIED: docker/loki/loki-config.yml:27-28]

### Critical Finding 3: `useWatchPreferences` short-circuits on `!authStore.isAuthenticated`

**File:** `frontend/web/src/composables/useWatchPreferences.ts:26`
```ts
async function resolve(available: WatchCombo[]) {
  if (!authStore.isAuthenticated || available.length === 0) return
  // ...
}
```

The existing resolve composable does NOT call the backend for anonymous users. This means `combo_resolve_total` will only increment for authenticated users **unless** the resolve composable is also modified to fire for anon users (with `X-Anon-ID` instead of JWT).

**Decision required from planner:**

| Option | Resolve denominator covers | Override numerator covers | Rate validity |
|--------|---------------------------|---------------------------|---------------|
| A — Leave resolve as-is (auth only) | Auth users only | Both auth + anon | Numerator anon will divide by 0; can't compute rate for `anon=true` |
| B — Modify resolve to fire for anon too | Both auth + anon | Both auth + anon | Rate well-defined per `anon` label |

**Recommendation:** Option B. CONTEXT.md D-12 ("unlocks a real per-anon-user override rate") implicitly requires it; otherwise the segmentation by `anon` label is meaningless. This means the planner adds: (1) modify `useWatchPreferences.ts` to drop the `!authStore.isAuthenticated` check and instead pass `X-Anon-ID` when no JWT; (2) modify `services/player/internal/handler/preference.go:ResolvePreference` to accept anonymous requests via the same `OptionalAuth` middleware; (3) the `claims.UserID` reference at `preference.go:47` becomes "user_id if claims present, else empty/anon".

**This expands Phase 1 scope by ~1 backend handler + 1 frontend composable edit but is required for correctness.** The alternative is to ship rates that can only be computed for authenticated users — a measurable success metric that excludes ~half the user base.

**Verified:** [VERIFIED: useWatchPreferences.ts:26 + preference.go:42-46]

## Standard Stack

### Core (already present)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `prometheus/client_golang` | 1.19.0 | Counter / Histogram for combo_override + combo_resolve | Already in `libs/metrics`; promauto self-registers [VERIFIED: STACK.md] |
| `go.uber.org/zap` (via `libs/logger`) | (latest in libs/logger/go.mod) | Structured logger that emits JSON to stdout, scraped by promtail to Loki | Existing `Infow` pattern in every service [VERIFIED: libs/logger/logger.go] |
| `go-chi/chi` | v5.0.12 | HTTP router for new override handler | Used in every service router [VERIFIED: services/player/internal/transport/router.go] |
| `golang-jwt/jwt` | v5 (via `libs/authz`) | Optional JWT decode for the override handler | Existing `authz.ClaimsFromContext` and JWT manager [VERIFIED: libs/authz] |
| Vue 3 | 3.4.21 | Composable host | Existing convention `frontend/web/src/composables/use*.ts` [VERIFIED: STACK.md] |
| TypeScript | 5.4.2 | Composable typing | Project standard [VERIFIED: STACK.md] |
| axios (`apiClient`) | (via `frontend/web/src/api/client.ts`) | HTTP client with JWT auto-refresh | All API calls go through this [VERIFIED: api/client.ts] |

### New (must add — minimal)
**No new packages needed.**
- `crypto.randomUUID()` is browser-native (Chrome 92+, Firefox 95+, Safari 15.4+, all production-grade) — covers anon_id and load_session_id [CITED: https://developer.mozilla.org/en-US/docs/Web/API/Crypto/randomUUID]
- `google/uuid v1.6.0` is already in the Go `libs/database` deps [VERIFIED: STACK.md] — but the override handler doesn't need to mint UUIDs server-side; it only echoes what the client sends.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Prometheus `CounterVec` | A new GORM table | D-04 explicitly forbids; would couple Phase 1 to migration risk |
| `crypto.randomUUID()` | `uuid` npm package | Adds dependency; native API is sufficient and zero-cost |
| Adding new `OptionalAuth` middleware | Reuse `AuthMiddleware` | The existing middleware always rejects on missing token; can't repurpose without breaking other routes |
| Brand new dashboard | Extend `preference-resolution.json` | D-15 explicitly mandates extending the existing dashboard |

**Installation:** No package installs required. Verify versions are still current at planning time:
```bash
# Backend (already present, verify):
grep -E "client_golang|chi/v5|zap" libs/metrics/go.mod libs/logger/go.mod services/player/go.mod | head -10

# Frontend (already present, verify):
cat frontend/web/package.json | grep -E "axios|vue\":|typescript"
```

## Architecture Patterns

### System Architecture Diagram

```
┌────────────────────────────────────────────────────────────────┐
│                  Browser (Vue 3 SPA)                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ Anime.vue (host view)                                    │ │
│  │   resolvedCombo = useWatchPreferences(animeId)           │ │
│  │     POST /api/preferences/resolve  ──┐                   │ │
│  │     (with X-Anon-ID if no JWT)       │                   │ │
│  │                                       │                   │ │
│  │   <KodikPlayer | AnimeLibPlayer |    │                   │ │
│  │    HiAnimePlayer | ConsumetPlayer>   │                   │ │
│  │     :preferred-combo="resolvedCombo" │                   │ │
│  │                                       │                   │ │
│  │   Each player setup():                │                   │ │
│  │     useOverrideTracker({              │                   │ │
│  │       player: 'kodik' (etc),          │                   │ │
│  │       resolvedCombo: ref,             │                   │ │
│  │       currentEpisode: ref,            │                   │ │
│  │       currentTranslation: ref,        │                   │ │
│  │     })                                │                   │ │
│  │                                       │                   │ │
│  │   On user picker click:               │                   │ │
│  │     tracker.recordPickerEvent({       │                   │ │
│  │       dimension: 'team' (etc),        │                   │ │
│  │       newCombo                         │                   │ │
│  │     })                                │                   │ │
│  │       │                                │                   │ │
│  │       ▼ (debounce 250ms,              │                   │ │
│  │         skip if dimension done,       │                   │ │
│  │         skip if > 30s since mount)    │                   │ │
│  │     POST /api/preferences/override ───┤                   │ │
│  │       Body: { anime_id, dimension,    │                   │ │
│  │         original_combo, new_combo,    │                   │ │
│  │         load_session_id,              │                   │ │
│  │         tier, ms_since_load }         │                   │ │
│  │       Headers: Authorization?         │                   │ │
│  │                X-Anon-ID?             │                   │ │
│  └───────────────────────────────────────┼───────────────────┘ │
└────────────────────────────────────────────┼─────────────────────┘
                                             │
                                             ▼
┌────────────────────────────────────────────────────────────────┐
│                  Gateway (port 8000)                            │
│                                                                 │
│  /api/preferences/* → ProxyToPlayer (NO JWT middleware,         │
│                                       OptionalAuth on player)   │
│  /api/users/preferences/resolve → JWT-protected (existing,      │
│                                    needs extension for anon)    │
└────────────────────────────────────────────┬─────────────────────┘
                                             │
                                             ▼
┌────────────────────────────────────────────────────────────────┐
│                Player service (port 8083)                       │
│                                                                 │
│  router.go:                                                     │
│    /api/preferences/override                                    │
│      → OptionalAuthMiddleware                                   │
│      → OverrideHandler.RecordOverride                           │
│        ├─ metrics.ComboOverrideTotal.WithLabelValues(...).Inc() │
│        └─ log.Infow("combo_override", ... structured fields)    │
│                                                                 │
│    /api/users/preferences/resolve (existing, modified)          │
│      → OptionalAuthMiddleware (replaces JWT-required)           │
│      → PreferenceHandler.ResolvePreference                      │
│        ├─ existing PreferenceResolutionTotal.Inc()              │
│        └─ NEW: metrics.ComboResolveTotal.WithLabelValues(...).  │
│            Inc()  ← matching label set                          │
└────────────────────────────────────────────┬─────────────────────┘
                                             │
                ┌────────────────────────────┼───────────────────┐
                ▼                            ▼                   ▼
┌────────────────────┐  ┌────────────────────────┐  ┌──────────────────────┐
│ stdout (json)      │  │  /metrics (HTTP)       │  │  PostgreSQL          │
│  → promtail        │  │   ← Prometheus scrape  │  │  (no new tables)     │
│  → Loki (7d)       │  │                        │  │                      │
└────────────────────┘  └─────────┬──────────────┘  └──────────────────────┘
                                  │
                                  ▼
                       ┌──────────────────────┐
                       │  Grafana             │
                       │  preference-         │
                       │  resolution.json     │
                       │  (auto-loaded RO     │
                       │   from disk)         │
                       │  → new panel         │
                       │    "Auto-Pick        │
                       │    Override Rate"    │
                       └──────────────────────┘
```

### Recommended Project Structure (existing — extend in place)

```
frontend/web/src/
├── composables/
│   ├── useWatchPreferences.ts          # MODIFY: send X-Anon-ID for anon users
│   └── useOverrideTracker.ts           # NEW
├── api/
│   └── client.ts                       # MODIFY: axios instance adds X-Anon-ID header
├── components/player/
│   ├── KodikPlayer.vue                 # MODIFY: import + invoke useOverrideTracker
│   ├── AnimeLibPlayer.vue              # MODIFY
│   ├── HiAnimePlayer.vue               # MODIFY
│   └── ConsumetPlayer.vue              # MODIFY
└── utils/
    └── anonId.ts                       # NEW (small): getOrCreateAnonId()

services/player/internal/
├── handler/
│   ├── override.go                     # NEW
│   └── preference.go                   # MODIFY: anon-friendly resolve, emit ComboResolveTotal
├── service/
│   └── preference.go                   # MODIFY: emit ComboResolveTotal alongside PreferenceResolutionTotal
├── transport/
│   └── router.go                       # MODIFY: register /api/preferences/override outside JWT group; add OptionalAuthMiddleware
└── cmd/player-api/main.go              # MODIFY: instantiate OverrideHandler, pass to NewRouter

services/gateway/internal/transport/
└── router.go                           # MODIFY: add r.HandleFunc("/preferences/*", proxyHandler.ProxyToPlayer) outside JWT group

libs/metrics/
└── watch.go                            # MODIFY: add ComboOverrideTotal + ComboResolveTotal

docker/grafana/dashboards/
└── preference-resolution.json          # MODIFY: add new panel

docker/loki/
└── loki-config.yml                     # OPTIONAL MODIFY: 168h → 720h (see Critical Finding 2)
```

### Pattern 1: Vue Composable — Argument-Based Refs (existing convention)

**What:** Composables accept arguments (string IDs, refs, options) and return refs + functions for the consuming component to spread/use.
**When to use:** Always — this is the project convention per `useWatchPreferences.ts`, `useImageProxy.ts`, `useAuth.ts`.

**Existing example for shape reference:**
```ts
// Source: frontend/web/src/composables/useWatchPreferences.ts (verified, line 8)
export function useWatchPreferences(animeId: string) {
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)
  // ... lifecycle: try cache, then async resolve
  async function resolve(available: WatchCombo[]) { /* ... */ }
  return { resolvedCombo, isLoading, resolve }
}
```

**Recommended `useOverrideTracker` shape:**
```ts
// frontend/web/src/composables/useOverrideTracker.ts (NEW)
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { userApi } from '@/api/client'  // OR a new prefApi exported from client.ts
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

export type OverrideDimension = 'language' | 'player' | 'team' | 'episode'

export interface OverrideTrackerOptions {
  animeId: string
  player: 'kodik' | 'animelib' | 'hianime' | 'consumet'
  resolvedCombo: Ref<ResolvedCombo | null>  // the combo applied to the player props
  currentEpisode: Ref<number>               // the episode currently shown
}

export function useOverrideTracker(opts: OverrideTrackerOptions) {
  const loadSessionId = crypto.randomUUID()
  const mountedAt = ref<number | null>(null)
  const emittedDimensions = new Set<OverrideDimension>()
  const debounceTimers = new Map<OverrideDimension, number>()

  const WINDOW_MS = 30_000
  const DEBOUNCE_MS = 250

  // D-10: window starts when resolved combo is APPLIED to player props,
  //       not when component mounts. Watch resolvedCombo to detect application.
  const stopWatch = watch(
    () => opts.resolvedCombo.value,
    (combo) => {
      if (combo && mountedAt.value === null) {
        mountedAt.value = performance.now()
      }
    },
    { immediate: true }
  )

  function recordPickerEvent(
    dimension: OverrideDimension,
    newCombo: Partial<WatchCombo> & { episode?: number }
  ) {
    // Skip if window not yet open (resolved combo hasn't applied)
    if (mountedAt.value === null) return
    // Skip if past 30s window
    const msSinceLoad = performance.now() - mountedAt.value
    if (msSinceLoad > WINDOW_MS) return
    // Skip if already emitted for this dimension
    if (emittedDimensions.has(dimension)) return

    // Debounce: coalesce two changes to same dimension within 250ms
    const existingTimer = debounceTimers.get(dimension)
    if (existingTimer) window.clearTimeout(existingTimer)
    debounceTimers.set(
      dimension,
      window.setTimeout(() => {
        // Mark as emitted BEFORE the network call so a second click in flight is ignored
        emittedDimensions.add(dimension)
        emit(dimension, newCombo, msSinceLoad)
      }, DEBOUNCE_MS)
    )
  }

  async function emit(
    dimension: OverrideDimension,
    newCombo: Partial<WatchCombo> & { episode?: number },
    msSinceLoad: number
  ) {
    try {
      await userApi.recordOverride({
        anime_id: opts.animeId,
        load_session_id: loadSessionId,
        dimension,
        original_combo: opts.resolvedCombo.value,
        new_combo: newCombo,
        ms_since_load: Math.round(msSinceLoad),
        tier: opts.resolvedCombo.value?.tier ?? null,
        tier_number: opts.resolvedCombo.value?.tier_number ?? null,
        player: opts.player,
      })
    } catch {
      // Best-effort instrumentation: never throw to caller, never block UX.
      // Counter loss is acceptable; this is monitoring, not business logic.
    }
  }

  onUnmounted(() => {
    debounceTimers.forEach((id) => window.clearTimeout(id))
    debounceTimers.clear()
    stopWatch()
  })

  return { recordPickerEvent, loadSessionId }
}
```

**Key design choices:**
- `loadSessionId` is module-scoped per composable invocation (one mount = one session). Re-mounting (anime change in `Anime.vue`) re-invokes the composable and gets a new session ID. ✓ D-09.
- Window opens on `resolvedCombo` first applying, NOT on `onMounted`. ✓ D-10.
- `emittedDimensions` is `Set` — first event per dimension wins. ✓ D-07 (one event per dimension max).
- Debounce coalesces double-clicks but the `add()` happens BEFORE the await, so even if the network is slow, subsequent calls in the same dimension are dropped. ✓
- `recordPickerEvent` is the ONLY entry point — players call it from picker-click handlers, NOT from `watch(props.preferredCombo, ...)`. This is what distinguishes user-initiated from auto-advance. ✓ D-08.

### Pattern 2: Player Component Integration

Each player has a "currentCombo" computed and a set of click handlers that change selected translation/episode/server. The composable is invoked once in `setup()` and the click handlers are wrapped (or augmented) to call `recordPickerEvent`.

**Quoted current shapes (per player):**

#### KodikPlayer.vue
- **Resolved combo intake:** `props.preferredCombo` (line 357) is read in `fetchTranslations()` to auto-select; player emits `availableTranslations` event back to `Anime.vue`.
- **Picker click handlers:**
  - `selectEpisode(episode)` — line 545 — `@click="selectEpisode(ep)"` line 86
  - `selectTranslation(translationId)` — line 531 — `@click="selectTranslation(t.id)"` line 152
  - Translation type tab (`translationType = 'voice' / 'subtitles'`) — lines 116, 128 — these are language-bound dimensions (sub vs dub) and ALSO trigger combo change.
- **Auto-advance trigger:** None native to Kodik (iframe doesn't emit episode-end). Episode changes only via `selectEpisode` user click. ✓ Easy.
- **`currentCombo` computed:** lines 385-396.
- **Composable insertion point:** in `<script setup>` after `currentCombo` is defined; pass `resolvedCombo: toRef(props, 'preferredCombo')` and `currentEpisode: selectedEpisode`.

#### HiAnimePlayer.vue
- **Resolved combo intake:** `props.preferredCombo` (line 453).
- **Picker click handlers:**
  - `selectEpisode(episode)` — line 1036 — `@click="selectEpisode(ep)"` line 115
  - `selectServer(server)` — line 1077 — `@click="selectServer(server)"` line 215
  - **Auto-advance:** `tryNextServer()` calls `selectServer` programmatically (line 1071) and `selectServer(newServers[0])` is called (line 1264) likely in episode-end handler. CRITICAL: planner must locate the auto-advance call sites and ensure they don't go through the same path that records overrides.
- **`currentCombo` computed:** line 607.
- **Insertion:** Wrap `selectEpisode`, `selectServer` to call `recordPickerEvent` with `dimension: 'episode'` or `dimension: 'team'` BEFORE the existing logic. Auto-advance call sites bypass the wrapper.

#### ConsumetPlayer.vue
- Same shape as HiAnimePlayer.
- `selectEpisode(ep)` — line 657 — `@click="selectEpisode(ep)"` line 118
- `selectServer(server)` — line 674 — `@click="selectServer(server)"` line 187
- `currentCombo` computed: line 571.
- `props.preferredCombo`: line 422.

#### AnimeLibPlayer.vue
- `selectEpisode(ep)` — line 495 — `@click="selectEpisode(ep)"` line 124
- `selectTranslation(tr)` — line 511 — `@click="selectTranslation(tr)"` line 196
- `selectQuality(source)` — line 522 — `@click="selectQuality(source)"` line 245 — quality is NOT a tracked dimension per D-08 ("quality switches" excluded). Skip wrapping this.
- `selectSubtitle(sub)` — line 270 click — also not a tracked dimension; skip.
- `translationFilter = 'all' | 'voice' | 'subtitles'` — lines 163, 172, 181 — same as Kodik (sub vs dub dimension change).

**Recommended dimension mapping:**
| Dimension | KodikPlayer | AnimeLibPlayer | HiAnimePlayer | ConsumetPlayer |
|-----------|-------------|----------------|---------------|----------------|
| `episode` | `selectEpisode` | `selectEpisode` | `selectEpisode` | `selectEpisode` |
| `team` | `selectTranslation` | `selectTranslation` | `selectServer` | `selectServer` |
| `language` (sub vs dub) | `translationType` toggle | `translationFilter` | `selectedCategory` toggle | (find equivalent — sub-or-dub prop) |
| `player` | (Anime.vue-level: switching `videoProvider`) | (same) | (same) | (same) |

**`player` dimension is special:** the player choice is made at `Anime.vue:videoProvider`, not inside any single player component. The composable cannot observe a player switch from inside the player that's about to unmount. Two implementation options:
1. Move `useOverrideTracker` invocation up to `Anime.vue` for the player dimension only, and watch `videoProvider` changes within the 30s window. (Recommended.)
2. Track player-switch separately as a one-shot in `Anime.vue` using a small inline ref + the same backend endpoint.

**Recommendation:** Hybrid. The composable lives inside each player for `episode | team | language`. A second short-lived tracker in `Anime.vue` (or a second invocation of the same composable scoped to player-only) handles `player` dimension. This is the only multi-mount-point case.

### Pattern 3: Existing Metric Registration (must mirror)

```go
// Source: libs/metrics/watch.go:33-39 (VERIFIED)
TranslationSelectionsTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "translation_selections_total",
        Help: "Total translation selections by users",
    },
    []string{"player", "language", "watch_type", "translation_title"},
)
```

**New counters (add to libs/metrics/watch.go, same file, same package init):**

```go
// ComboOverrideTotal tracks user-initiated combo changes within 30s of player load.
// One increment per (load_session_id, dimension) at most — composable enforces.
ComboOverrideTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "combo_override_total",
        Help: "User overrides of auto-picked combo within 30s of player load",
    },
    []string{"tier", "dimension", "language", "anon", "player"},
)

// ComboResolveTotal is the denominator: every successful resolve outcome,
// labeled identically so PromQL rate(override) / rate(resolve) lines up.
ComboResolveTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "combo_resolve_total",
        Help: "Successful preference resolution outcomes",
    },
    []string{"tier", "language", "anon", "player"},
)
```

**Cardinality budget (must verify before merge):**
- `tier`: 5 values (per_anime, user_global, community, pinned, default) + null = 6
- `dimension`: 4 values (language, player, team, episode) — only on override
- `language`: 2 values (ru, en) — D-14 says only RU+EN supported
- `anon`: 2 values (true, false)
- `player`: 4 values (kodik, animelib, hianime, consumet)

**Override total cardinality:** 6 × 4 × 2 × 2 × 4 = **384 series** ✓ well under any limit.
**Resolve total cardinality:** 6 × 2 × 2 × 4 = **96 series** ✓.

⚠️ The `language` field can be `""` (empty string) when resolver tier 1 sets a lock from a stale preference whose value isn't in the validators. Default to `"unknown"` if empty when emitting. Same for `player`. Use a small helper `func labelOrUnknown(s string) string`.

### Pattern 4: Resolver Increment Site

**Where to increment `ComboResolveTotal`:** `services/player/internal/service/preference.go:62-66` already has the `PreferenceResolutionTotal.Inc()`. Add the new counter increment in the same block.

```go
// Source: services/player/internal/service/preference.go:62-67 (VERIFIED)
// Increment metrics
tier := "null"
if result != nil {
    tier = result.Tier
}
metrics.PreferenceResolutionTotal.WithLabelValues(tier).Inc()

// NEW LINES:
language := ""
player := ""
if result != nil {
    language = result.Language
    player = result.Player
}
anonLabel := "true"
if userID != "" { anonLabel = "false" }
metrics.ComboResolveTotal.WithLabelValues(
    tier,
    labelOrUnknown(language),
    anonLabel,
    labelOrUnknown(player),
).Inc()
```

**Discretion answer (denominator emit on cache-hit):** The composable's resolve call goes through axios → backend handler → service.Resolve every time. Backend has no cache that short-circuits the increment. The 24h *frontend* localStorage cache (`useWatchPreferences.ts:14-23`) DOES skip the network call, meaning the backend never sees a "resolve" for cached frontends. Therefore: **the backend counter naturally counts only network-resolves**, and the rate `override / resolve` represents "fresh resolves that triggered an override within 30s." This is **what we want** — overrides on a cached resolve from yesterday are just normal player usage, not an auto-pick failure.

**Document this clearly in the dashboard panel description.** "Override rate is computed against fresh resolves only (24h frontend cache means resolved combos are reused without backend call)."

### Pattern 5: New Handler Template

```go
// services/player/internal/handler/override.go (NEW)
package handler

import (
    "net/http"

    "github.com/ILITA-hub/animeenigma/libs/authz"
    "github.com/ILITA-hub/animeenigma/libs/errors"
    "github.com/ILITA-hub/animeenigma/libs/httputil"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/libs/metrics"
)

// OverrideRequest mirrors the composable's POST body.
type OverrideRequest struct {
    AnimeID         string                 `json:"anime_id"`
    LoadSessionID   string                 `json:"load_session_id"`
    Dimension       string                 `json:"dimension"`        // language|player|team|episode
    OriginalCombo   map[string]interface{} `json:"original_combo"`   // ResolvedCombo subset; can be null
    NewCombo        map[string]interface{} `json:"new_combo"`        // partial WatchCombo + optional episode
    MsSinceLoad     int64                  `json:"ms_since_load"`
    Tier            string                 `json:"tier"`             // tier name from ResolvedCombo
    TierNumber      int                    `json:"tier_number"`
    Player          string                 `json:"player"`           // kodik|animelib|hianime|consumet
}

var validDimensions = map[string]bool{
    "language": true, "player": true, "team": true, "episode": true,
}

type OverrideHandler struct {
    log *logger.Logger
}

func NewOverrideHandler(log *logger.Logger) *OverrideHandler {
    return &OverrideHandler{log: log}
}

// RecordOverride is intentionally permissive: best-effort instrumentation,
// fast path, no DB writes. The handler emits one Prometheus increment and
// one structured log line, then returns 204.
func (h *OverrideHandler) RecordOverride(w http.ResponseWriter, r *http.Request) {
    var req OverrideRequest
    if err := httputil.Bind(r, &req); err != nil {
        httputil.Error(w, errors.InvalidInput("invalid override payload"))
        return
    }
    if req.AnimeID == "" || req.LoadSessionID == "" {
        httputil.Error(w, errors.InvalidInput("anime_id and load_session_id required"))
        return
    }
    if !validDimensions[req.Dimension] {
        httputil.Error(w, errors.InvalidInput("dimension must be language|player|team|episode"))
        return
    }

    // Identify the user (optional auth)
    var userID, anonID string
    if claims, ok := authz.ClaimsFromContext(r.Context()); ok && claims != nil {
        userID = claims.UserID
    } else {
        anonID = r.Header.Get("X-Anon-ID")
    }
    if userID == "" && anonID == "" {
        // Reject: must be either authed or have an anon header.
        // Otherwise we can't segment by `anon` label and a malicious caller can blow up cardinality.
        httputil.Error(w, errors.InvalidInput("X-Anon-ID required for unauthenticated requests"))
        return
    }

    anonLabel := "true"
    if userID != "" { anonLabel = "false" }

    // Pull language from new_combo if present (fallback to existing tier label work)
    language := stringOr(req.NewCombo, "language", "unknown")

    metrics.ComboOverrideTotal.WithLabelValues(
        labelOrUnknown(req.Tier),
        req.Dimension,
        labelOrUnknown(language),
        anonLabel,
        labelOrUnknown(req.Player),
    ).Inc()

    h.log.Infow("combo_override",
        "anime_id", req.AnimeID,
        "load_session_id", req.LoadSessionID,
        "dimension", req.Dimension,
        "user_id", userID,
        "anon_id", anonID,
        "original_combo", req.OriginalCombo,
        "new_combo", req.NewCombo,
        "ms_since_load", req.MsSinceLoad,
        "tier", req.Tier,
        "tier_number", req.TierNumber,
        "player", req.Player,
    )

    w.WriteHeader(http.StatusNoContent)
}

func labelOrUnknown(s string) string {
    if s == "" { return "unknown" }
    return s
}
func stringOr(m map[string]interface{}, key, fallback string) string {
    if v, ok := m[key].(string); ok && v != "" { return v }
    return fallback
}
```

### Pattern 6: OptionalAuth Middleware

```go
// services/player/internal/transport/router.go (extend)
// OptionalAuthMiddleware decodes JWT if present, attaches Claims to context.
// Does NOT reject when no token is present. Pair with handlers that check
// authz.ClaimsFromContext and fall back to X-Anon-ID.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
    jwtManager := authz.NewJWTManager(jwtConfig)
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := httputil.BearerToken(r)
            if token != "" {
                if claims, err := jwtManager.ValidateAccessToken(token); err == nil {
                    r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

Place inline next to the existing `AuthMiddleware` function in `transport/router.go:139-160`. New route registration:

```go
// In NewRouter(), OUTSIDE r.Route("/users", ...):
r.Route("/api/preferences", func(r chi.Router) {
    r.Use(OptionalAuthMiddleware(jwtConfig))
    r.Post("/override", overrideHandler.RecordOverride)
})
```

And the existing resolve route MUST also become OptionalAuth-friendly (Critical Finding 3). Cleanest move: lift `/preferences/resolve` out of the `/users` group, put it next to `/override`, and update the existing handler to read `userID` as empty string when claims are missing. The repo `prefRepo.GetAnimePreference(ctx, "", animeID)` will return nothing for empty userID — that's the correct anon behavior (Tier 1 + Tier 2 are skipped, fall through to Tier 3+).

### Pattern 7: Frontend axios — X-Anon-ID propagation

```ts
// frontend/web/src/utils/anonId.ts (NEW)
const STORAGE_KEY = 'aenig_anon_id'

export function getOrCreateAnonId(): string {
  try {
    let id = localStorage.getItem(STORAGE_KEY)
    if (!id) {
      id = crypto.randomUUID()
      localStorage.setItem(STORAGE_KEY, id)
    }
    return id
  } catch {
    // localStorage unavailable (private mode, etc.) — generate ephemeral
    return crypto.randomUUID()
  }
}
```

```ts
// frontend/web/src/api/client.ts — extend the existing request interceptor (line 75-95)
import { getOrCreateAnonId } from '@/utils/anonId'

apiClient.interceptors.request.use(
  async (config: InternalAxiosRequestConfig) => {
    if (config.url?.includes('/auth/refresh') || config.url?.includes('/auth/login')) {
      return config
    }

    let token = localStorage.getItem('token')
    if (token && isTokenExpired(token)) {
      const newToken = await doTokenRefresh()
      token = newToken
    }
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    } else if (config.headers) {
      // NEW: anon header for the (small) set of endpoints that accept it
      // Either always-set (cheap, harmless on JWT routes) or path-gated.
      config.headers['X-Anon-ID'] = getOrCreateAnonId()
    }
    return config
  },
  // ...
)
```

**Recommendation:** Always-set (the `else` branch). The header is ignored by every existing handler that doesn't read it; cardinality risk is zero (the new override + resolve handlers are the only readers, and both validate the input). Reduces the chance of forgetting to add it for future anon-friendly endpoints.

```ts
// frontend/web/src/api/client.ts — add to userApi
recordOverride: (data: {
  anime_id: string
  load_session_id: string
  dimension: 'language' | 'player' | 'team' | 'episode'
  original_combo: ResolvedCombo | null
  new_combo: Partial<WatchCombo> & { episode?: number }
  ms_since_load: number
  tier: string | null
  tier_number: number | null
  player: 'kodik' | 'animelib' | 'hianime' | 'consumet'
}) => apiClient.post('/preferences/override', data),
```

### Anti-Patterns to Avoid

- **Watching `props.preferredCombo` to detect overrides.** Auto-advance changes the prop too. The composable MUST gate on user-event-only inputs.
- **Putting `useOverrideTracker` outside `setup()` of each player.** The composable lifecycle MUST be tied to player mount so a `load_session_id` per visit is enforced.
- **Recording the override before validating the dimension.** A bug in any one player that emits a wrong dimension would inflate the metric for all dashboards.
- **Forgetting to mark `emittedDimensions.add(dimension)` BEFORE the awaited POST.** Two clicks within the network round-trip would both emit. Add-then-await is the lock pattern.
- **Reading `claims.UserID` directly in the override handler without the optional-auth check.** Will panic on nil claims.
- **Logging the full request URL or query string with `log.Infow`.** Don't accidentally bake PII (anime title, search query) into Loki labels — Loki indexes log labels but not log fields, so labels must stay low-cardinality. The log fields shown above are safe; the structured fields are search-only, not indexed.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Counter aggregation | New GORM table + worker | Prometheus `CounterVec` | D-04 explicit; existing infrastructure handles retention, scraping, query, dashboards |
| UUIDs in browser | `Math.random()` or custom hex | `crypto.randomUUID()` | Native, RFC 4122 compliant, no dependency [CITED: MDN] |
| JSON binding | Manual `json.Decoder` | `httputil.Bind` | Existing helper handles `application/json` content-type and request-size guards |
| HTTP error responses | `http.Error` | `httputil.Error`/`OK`/`BadRequest`/`Unauthorized` | Consistent JSON shape across services [VERIFIED: libs/httputil] |
| Optional JWT decoding | Custom token parsing | `authz.NewJWTManager(...).ValidateAccessToken` | Same library as production middleware; verified against existing `AuthMiddleware` |
| Anonymous user identity | UUIDs minted server-side per request | localStorage UUID minted browser-side | D-11 explicit; cross-request continuity requires client-side persistence |
| Loki log shipping | Manual file scrapers | promtail (already running) | Already wired in docker-compose; service stdout → Loki |
| Grafana panel registration | Manual import via UI | File-mounted JSON in `docker/grafana/dashboards/` | D-17 explicit; auto-loaded via provisioning |

**Key insight:** Phase 1 is almost entirely **wiring existing infrastructure together**. The only authored code is: 1 Vue composable, 1 Go handler, 1 OptionalAuth middleware, ~6 lines of metric definitions, ~10 lines of dashboard panel JSON, ~30 lines of player edits per player × 4. No new libraries, no new tables, no new infrastructure.

## Common Pitfalls

### Pitfall 1: Auto-advance triggering false-positive overrides

**What goes wrong:** When an episode ends, HiAnime/Consumet players programmatically call `selectEpisode(nextEp)` to advance. If the composable watches `props.preferredCombo` or `selectedEpisode` reactively, every auto-advance becomes an "episode override."
**Why it happens:** Vue reactivity doesn't distinguish prop change provenance.
**How to avoid:** The composable's ONLY entry point is the explicit `recordPickerEvent()` call. Player code calls it from inside the click handler (e.g., `selectEpisode`), but auto-advance code paths bypass the click handler — they call the underlying state-change function (or a sibling `tryNextServer`) directly. Audit each `selectEpisode` / `selectServer` / `selectTranslation` to confirm a USER click is the only trigger; for auto-advance, refactor to call a sibling `_advanceEpisode(nextEp)` that does NOT go through the click handler.
**Warning signs:** `combo_override_total{dimension="episode"}` rate > expected during integration tests; weird spike at exactly the auto-mark threshold (20 min).

### Pitfall 2: 30s window starting before resolve completes

**What goes wrong:** D-10 says window starts when resolved combo is APPLIED to player props. If the composable starts the window on `onMounted` and the resolve takes 800ms, the user already has 800ms less window. If the resolve takes longer (cold cache, slow network), legitimate user picks might happen before the resolved combo lands and get incorrectly attributed as overrides.
**Why it happens:** Mount-time clock vs. apply-time clock.
**How to avoid:** Watch `resolvedCombo` ref; start `mountedAt` only when it transitions from `null` to a value. Pattern in §Pattern 1 implements this.
**Warning signs:** `ms_since_load` distribution clustered near 0 (suggests overrides being recorded immediately at mount, before the user has had a chance to look).

### Pitfall 3: Prometheus cardinality explosion via untrusted labels

**What goes wrong:** If the override handler accepts `language` from the request body and writes it to a label, an attacker can POST `{"language": "<random uuid each time>"}` and explode label cardinality, eventually OOMing Prometheus.
**Why it happens:** Prometheus labels are indexed; high cardinality breaks the index.
**How to avoid:** Whitelist all label values. `labelOrUnknown` for any field that comes from untrusted input. Validate `dimension` against the whitelist (already done in §Pattern 5). For `language`, `player`, `tier` — coerce to known set or fall back to `"unknown"`.
**Warning signs:** Prometheus memory growth, slow queries on `combo_override_total`. Mitigate via `metric_relabel_configs` in `prometheus.yml` as a safety net (drop unknown values).

### Pitfall 4: Grafana dashboard reload — when does new panel appear?

**What goes wrong:** Modifying `docker/grafana/dashboards/preference-resolution.json` and `make redeploy-player` does NOT reload the Grafana container. The dashboard provisioning ConfigMap is read on Grafana startup.
**Why it happens:** Grafana provisioning's file-provider has a poll interval (default 10s) but the volume is mounted RO at runtime; new file content is picked up by the Grafana process.
**How to avoid:** After editing the dashboard JSON, either:
1. Wait ~10s and refresh the Grafana UI (the file provider polls and re-loads); OR
2. Run `docker compose -f docker/docker-compose.yml restart grafana` for immediate reload.
**Warning signs:** New panel doesn't appear after deploy. Check `docker compose logs grafana | grep -i provision` for "reloading dashboards" messages.
**Verified:** [VERIFIED: docker/grafana/provisioning/dashboards/dashboards.yml — file provider with default poll]

### Pitfall 5: anon_id changing between resolve and override within one mount

**What goes wrong:** If the resolve call lands on a fresh tab and creates an anon_id, but the override emit in the same mount somehow generates a fresh one (e.g., timing issue in `getOrCreateAnonId`), the rate denominator and numerator have different keys.
**Why it happens:** Race in localStorage initialization, or accidental double-mint.
**How to avoid:** `getOrCreateAnonId()` reads localStorage atomically; once written, never overwritten. Composable also caches the value in a closure variable on first call to avoid repeated localStorage hits. The header is set per-request by axios interceptor, but reads the same `getOrCreateAnonId()` so it's stable.
**Warning signs:** `combo_override_total{anon="true"}` count > `combo_resolve_total{anon="true"}` count, or vice versa, with no auth-state changes.

### Pitfall 6: JWT-or-anon endpoint as DDoS amplification vector

**What goes wrong:** An anonymous endpoint accepting POSTs is a write surface. A malicious caller could blast 10k POSTs/sec to inflate counters.
**Why it happens:** No rate limit on the new path.
**How to avoid:**
1. Validate AnimeID, LoadSessionID, Dimension at handler entry — reject invalid → no metric increment.
2. The `metricsCollector.Middleware` (existing in player router) records `http_requests_total` so abnormal traffic is detectable.
3. Existing gateway has rate limiting (per CLAUDE.md). Add this path to the rate-limited group at the gateway layer — leverage existing infra rather than reinventing.
4. Reject when both `userID` AND `anonID` are missing (handler does this) — prevents truly headerless flood.
**Warning signs:** `http_requests_total{path="/api/preferences/override"}` spike disproportionate to active sessions; `combo_override_total` rate > `combo_resolve_total` rate (impossible in normal usage).

### Pitfall 7: Modifying `useWatchPreferences` for anon breaks existing auth flow

**What goes wrong:** If we drop `if (!authStore.isAuthenticated) return` (Critical Finding 3) and the backend `ResolvePreference` handler isn't yet anon-aware, anon users hit the endpoint and get 401, the composable swallows the error, and they never get a resolved combo. UX regression.
**Why it happens:** Sequencing — backend must accept anon BEFORE frontend starts sending.
**How to avoid:** Plan the deploy in order: (1) deploy backend changes first via `make redeploy-player` and verify resolve works for both auth and anon via curl; (2) THEN deploy frontend. Do not interleave.
**Warning signs:** Spike in `http_requests_total{service="player",path="/api/users/preferences/resolve",status="401"}` after frontend deploys.

## Code Examples

### Verified — Existing Counter Pattern
```go
// Source: libs/metrics/watch.go:33-47 (VERIFIED)
TranslationSelectionsTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "translation_selections_total",
        Help: "Total translation selections by users",
    },
    []string{"player", "language", "watch_type", "translation_title"},
)

PreferenceResolutionTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Name: "preference_resolution_total",
        Help: "Total preference resolution outcomes by tier",
    },
    []string{"tier"},
)
```

### Verified — Existing Handler Shape
```go
// Source: services/player/internal/handler/preference.go:25-54 (VERIFIED)
func (h *PreferenceHandler) ResolvePreference(w http.ResponseWriter, r *http.Request) {
    var req domain.ResolveRequest
    if err := httputil.Bind(r, &req); err != nil {
        httputil.Error(w, err)
        return
    }

    if req.AnimeID == "" {
        httputil.Error(w, errors.InvalidInput("anime_id is required"))
        return
    }

    claims, ok := authz.ClaimsFromContext(r.Context())
    if !ok || claims == nil {
        httputil.Unauthorized(w)
        return
    }

    resp, err := h.prefService.Resolve(r.Context(), claims.UserID, &req)
    if err != nil {
        httputil.Error(w, err)
        return
    }

    httputil.OK(w, resp)
}
```

### Verified — Existing Chi Route Registration
```go
// Source: services/player/internal/transport/router.go:103-107 (VERIFIED)
// Preference routes
r.Post("/preferences/resolve", preferenceHandler.ResolvePreference)
r.Get("/preferences/global", preferenceHandler.GetGlobalPreferences)
r.Get("/preferences/{animeId}", preferenceHandler.GetAnimePreference)
```

### Verified — Existing Optional-Auth-Like Pattern (anime reviews)
```go
// Source: services/player/internal/transport/router.go:120-132 (VERIFIED)
r.Route("/anime/{animeId}", func(r chi.Router) {
    // Public routes
    r.Get("/reviews", reviewHandler.GetAnimeReviews)
    r.Get("/rating", reviewHandler.GetAnimeRating)

    // Protected routes
    r.Group(func(r chi.Router) {
        r.Use(AuthMiddleware(jwtConfig))
        r.Post("/reviews", reviewHandler.CreateOrUpdateReview)
        r.Get("/reviews/me", reviewHandler.GetUserReview)
        r.Delete("/reviews", reviewHandler.DeleteReview)
    })
})
```

### Verified — Existing Composable Shape
```ts
// Source: frontend/web/src/composables/useWatchPreferences.ts (VERIFIED)
export function useWatchPreferences(animeId: string) {
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)
  const authStore = useAuthStore()

  const cacheKey = `pref:${animeId}`
  const cached = localStorage.getItem(cacheKey)
  if (cached) {
    try {
      const { data, timestamp } = JSON.parse(cached)
      if (Date.now() - timestamp < CACHE_TTL) {
        resolvedCombo.value = data
      }
    } catch { /* ignore corrupt cache */ }
  }

  async function resolve(available: WatchCombo[]) { /* ... */ }

  return { resolvedCombo, isLoading, resolve }
}
```

### Verified — Existing Player `defineProps` Shape (HiAnimePlayer)
```ts
// Source: frontend/web/src/components/player/HiAnimePlayer.vue:448-460 (VERIFIED)
const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
  preferredCombo?: WatchCombo | null
}>()

const emit = defineEmits<{
  (e: 'progress', data: { episode: number; time: number; maxTime: number }): void
  (e: 'episodeWatched', data: { episode: number }): void
  (e: 'availableTranslations', combos: WatchCombo[]): void
}>()
```

### Verified — Loki structured-logging path
- Promtail scrapes via `docker_sd_configs` (file: `docker/promtail/config.yml`).
- Auto-attached labels on every log line: `container`, `service`, `project` (from compose labels).
- Log content: stdout from container as-is. Zap JSON output (production config) becomes the log line.
- A `log.Infow("combo_override", "anime_id", "...", "user_id", "...")` lands in Loki with:
  - Indexed labels: `container=animeenigma-player`, `service=player`, `project=animeenigma`
  - Log line: `{"level":"info","ts":..., "msg":"combo_override", "anime_id":"...", "user_id":"...", ...}`
- Search: `{service="player"} |= "combo_override" | json` in Grafana Explore.
- **Custom labels NOT promoted to indexed labels** — log fields are searchable via `| json` parser only. This is fine for our use case.
- [VERIFIED: docker/promtail/config.yml + docker-compose.yml:189-201]

### Recommended — New Dashboard Panel JSON

Add to `docker/grafana/dashboards/preference-resolution.json` `panels` array. Insert as a new collapsible row "Auto-Pick Override Rate (Phase 1 Baseline)" near the top.

```json
{
  "collapsed": false,
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 37 },
  "id": 100,
  "panels": [],
  "title": "Auto-Pick Override Rate (Phase 1 Baseline)",
  "type": "row"
},
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "fieldConfig": {
    "defaults": {
      "color": { "mode": "thresholds" },
      "mappings": [],
      "thresholds": {
        "mode": "absolute",
        "steps": [
          { "color": "green", "value": null },
          { "color": "yellow", "value": 0.10 },
          { "color": "red", "value": 0.20 }
        ]
      },
      "unit": "percentunit",
      "max": 1,
      "min": 0
    }
  },
  "gridPos": { "h": 6, "w": 8, "x": 0, "y": 38 },
  "id": 101,
  "options": {
    "colorMode": "background",
    "graphMode": "area",
    "justifyMode": "center",
    "orientation": "auto",
    "reduceOptions": { "calcs": ["lastNotNull"], "fields": "", "values": false },
    "textMode": "auto"
  },
  "targets": [
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "expr": "sum(rate(combo_override_total[5m])) / sum(rate(combo_resolve_total[5m]))",
      "legendFormat": "Override Rate",
      "refId": "A"
    }
  ],
  "title": "Override Rate (last 5m)",
  "type": "stat",
  "description": "Computed against fresh resolves only. 24h frontend cache means cached resolves don't increment denominator. Target < 10% after Phase 7 ships."
},
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "fieldConfig": {
    "defaults": {
      "color": { "mode": "palette-classic" },
      "custom": {
        "drawStyle": "line", "fillOpacity": 15, "lineInterpolation": "smooth",
        "lineWidth": 2, "pointSize": 5, "showPoints": "never", "spanNulls": false,
        "stacking": { "group": "A", "mode": "none" }
      },
      "unit": "percentunit",
      "max": 1, "min": 0
    }
  },
  "gridPos": { "h": 10, "w": 16, "x": 8, "y": 38 },
  "id": 102,
  "options": {
    "legend": { "calcs": ["mean", "max", "last"], "displayMode": "table", "placement": "bottom", "showLegend": true },
    "tooltip": { "mode": "multi", "sort": "desc" }
  },
  "targets": [
    {
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "expr": "sum by(tier)(rate(combo_override_total[5m])) / ignoring(dimension) sum by(tier)(rate(combo_resolve_total[5m]))",
      "legendFormat": "Tier {{tier}}",
      "refId": "A"
    }
  ],
  "title": "Override Rate by Tier",
  "type": "timeseries"
},
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "fieldConfig": { "defaults": { "color": { "mode": "palette-classic" }, "unit": "percentunit", "max": 1, "min": 0 } },
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": 48 },
  "id": 103,
  "targets": [{
    "expr": "sum by(player)(rate(combo_override_total[5m])) / sum by(player)(rate(combo_resolve_total[5m]))",
    "legendFormat": "{{player}}",
    "refId": "A"
  }],
  "title": "Override Rate by Player",
  "type": "timeseries"
},
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 12, "x": 12, "y": 48 },
  "id": 104,
  "targets": [{
    "expr": "sum by(language, anon)(rate(combo_override_total[5m])) / sum by(language, anon)(rate(combo_resolve_total[5m]))",
    "legendFormat": "{{language}} / anon={{anon}}",
    "refId": "A"
  }],
  "title": "Override Rate by Language × Auth State",
  "type": "timeseries"
},
{
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "gridPos": { "h": 8, "w": 24, "x": 0, "y": 56 },
  "id": 105,
  "targets": [{
    "expr": "sum by(dimension)(increase(combo_override_total[24h]))",
    "legendFormat": "{{dimension}}",
    "refId": "A"
  }],
  "title": "Overrides by Dimension (24h count)",
  "type": "barchart",
  "description": "Which choice the auto-pick gets wrong most often: language, player, team, or episode."
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual `prometheus.MustRegister` | `promauto.NewCounterVec` | Codebase convention | Existing `libs/metrics/watch.go` uses promauto throughout — follow it |
| Per-event GORM table for analytics | Prometheus + Loki structured logs | D-04 explicit | Saves migration risk; can revisit Phase 5 |
| Mounted-time as window start | Resolved-combo-applied-time | D-10 explicit | Avoids slow-load false positives |
| Watch raw prop changes for overrides | Explicit picker-event hook | D-08 explicit | Distinguishes user intent from auto-advance |

**Deprecated/outdated in this codebase:**
- `useWatchPreferences.ts` short-circuits anon — must change for D-12 to work (Critical Finding 3).
- Loki retention is 7d in config but CONTEXT claims 31d — must reconcile (Critical Finding 2).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `crypto.randomUUID()` is universally available in production user agents we serve | Standard Stack | Low — fallback to inline polyfill if telemetry shows holes; no functional regression |
| A2 | Promtail auto-attaches `{container, service, project}` labels and JSON-encoded zap output is searchable in Loki | Code Examples — Loki path | Low — verified via promtail config; manual `make logs-player` after deploy confirms |
| A3 | Grafana file-provider polls every ~10s for dashboard JSON changes | Pitfall 4 | Low — `docker compose restart grafana` is a safe fallback |
| A4 | Adding `X-Anon-ID` always-on at the axios interceptor is harmless on existing JWT-protected routes | Pattern 7 | Low — existing handlers ignore unknown headers; verified by reading every handler in player service |
| A5 | The `OptionalAuth` middleware can be added to the existing resolve route without breaking authenticated flows | Critical Finding 3 | Medium — must verify with curl that JWT-only requests still get tier 1+2 lookups; add unit test |
| A6 | A 24h frontend cache means many users won't trigger backend resolve, so `combo_resolve_total` < actual page-loads | Pattern 4 | Low — desired behavior; documented in panel description |
| A7 | The 4 players' picker handlers can be wrapped without breaking existing auto-advance / programmatic-server-switch flows | Pattern 2 | Medium — requires careful refactoring of HiAnime's `tryNextServer` and Consumet's auto-advance paths to NOT call the wrapped handler. Mitigated by integration test that runs through end-of-episode and asserts no override emitted. |

**Note:** Most assumptions are LOW risk. A5 and A7 are the planner's primary verification targets in Wave 1.

## Open Questions (RESOLVED)

1. **Should `dimension="player"` be tracked from `Anime.vue` or skipped in v1?**
   - **RESOLVED — Track it.** A thin wrapper in `Anime.vue` invokes the same backend POST when `videoProvider` changes via the user-facing player picker (not via auto-selection / fallback). Cost ~10 lines. Value: distinguishes "language/team picker is broken" from "the auto-picked player is wrong." Plan 01-05 Task 2 implements this.

2. **Should we ship Loki retention bump (7d → 30d) inside Phase 1 or defer?**
   - **RESOLVED — Defer.** Loki stays at 7d. Document the actual 7d ceiling in `.planning/PROJECT.md` § "Loki retention constraint" alongside the Phase 1 instrumentation deliverable. Prometheus 15d retention covers the rate-over-time use case (Phase 7 success criterion 5). If Phase 5/6 needs >7d per-event Loki history, re-evaluate then or add the per-event DB table from the deferred list.

3. **Should the override handler de-duplicate at the server level too?**
   - **RESOLVED — Trust the client for v1.** Composable already de-duplicates per `(load_session_id, dimension)` per session. The metric is monitoring data, not financial — eventual consistency is acceptable. If abuse is detected later, add a `metric_relabel_configs` in `prometheus.yml` to drop pathological cardinality. Server-side LRU is out of scope for Phase 1.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Docker | All build/deploy | ✓ | (per docker-compose.yml `image:` tags) | — |
| Go 1.22 | Backend builds | ✓ | 1.22 | — |
| Bun | Frontend build | ✓ | 1.x | — |
| Prometheus | Metric scraping | ✓ | (per docker-compose.yml) | — |
| Loki + promtail | Log shipping | ✓ | 2.9.4 | — |
| Grafana | Dashboard | ✓ | 10.3.3 | — |
| `crypto.randomUUID()` | Browser anon_id | ✓ | All target browsers | Inline polyfill if telemetry shows missing |
| `make redeploy-<svc>` script | Deploy | ✓ | `Makefile:228` + `deploy/scripts/redeploy.sh` | — |

**Missing dependencies with no fallback:** None.
**Missing dependencies with fallback:** None.

## Validation Architecture

> Phase Nyquist Validation is enabled in `.planning/config.json` (`workflow.nyquist_validation: true`). This section defines the test framework, requirement-to-test map, sampling rate, and Wave 0 gaps for VALIDATION.md instantiation.

### Test Framework

| Property | Value |
|----------|-------|
| Backend framework | `go test` + `testify v1.8.4` (existing in `services/player/go.mod`) |
| Frontend unit framework | None currently; recommend Vitest 1.x for composable tests (already a peer of Vite which is the project bundler) |
| Frontend E2E framework | Playwright 1.58.0 (existing — `frontend/web/package.json`) |
| Backend test config | `services/player/go.mod` — no separate config file |
| Frontend unit config | NEW — `frontend/web/vitest.config.ts` (Wave 0 gap if not present) |
| Quick run command — backend | `cd services/player && go test ./internal/handler/... -run Override` |
| Quick run command — frontend unit | `cd frontend/web && bunx vitest run src/composables/useOverrideTracker.test.ts` |
| Quick run command — frontend E2E | `cd frontend/web && bunx playwright test combo-override` |
| Full suite — backend | `cd services/player && go test ./...` |
| Full suite — frontend | `cd frontend/web && bunx vitest run && bunx playwright test` |
| Phase gate | `make health` returns all-green; PromQL probe shows `combo_override_total` AND `combo_resolve_total` present and non-zero |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| M-01 | Composable detects user-initiated language change within 30s window, emits POST | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "language dimension"` | ❌ Wave 0 |
| M-01 | Composable detects user-initiated team change | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "team dimension"` | ❌ Wave 0 |
| M-01 | Composable detects user-initiated episode change | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "episode dimension"` | ❌ Wave 0 |
| M-01 | Composable detects user-initiated player change (Anime.vue level) | unit | `bunx vitest run src/views/Anime.test.ts -t "player override"` | ❌ Wave 0 |
| M-01 | Composable IGNORES auto-advance (false-positive prevention, Pitfall 1) | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "ignores auto-advance"` | ❌ Wave 0 |
| M-01 | Composable DEBOUNCES double-clicks within 250ms to one event | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "debounces"` | ❌ Wave 0 |
| M-01 | Composable IGNORES second change to same dimension after first emitted | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "first per dimension only"` | ❌ Wave 0 |
| M-01 | Composable IGNORES changes after 30s window | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "30s window expires"` | ❌ Wave 0 |
| M-01 | Window starts when resolvedCombo APPLIED, not on mount (Pitfall 2) | unit | `bunx vitest run src/composables/useOverrideTracker.test.ts -t "window starts on apply"` | ❌ Wave 0 |
| M-01 | Backend handler increments `ComboOverrideTotal` with correct labels | unit | `cd services/player && go test ./internal/handler -run TestOverride_IncrementsCounter` | ❌ Wave 0 |
| M-01 | Backend handler emits `combo_override` log line with structured fields | unit | `cd services/player && go test ./internal/handler -run TestOverride_LogsStructured` | ❌ Wave 0 |
| M-01 | Backend handler validates `dimension` against whitelist (Pitfall 3) | unit | `cd services/player && go test ./internal/handler -run TestOverride_RejectsInvalidDimension` | ❌ Wave 0 |
| M-01 | Backend handler accepts JWT-authenticated requests | unit | `cd services/player && go test ./internal/handler -run TestOverride_AcceptsJWT` | ❌ Wave 0 |
| M-01 | Backend handler accepts X-Anon-ID requests | unit | `cd services/player && go test ./internal/handler -run TestOverride_AcceptsAnonID` | ❌ Wave 0 |
| M-01 | Backend handler rejects requests with neither JWT nor X-Anon-ID | unit | `cd services/player && go test ./internal/handler -run TestOverride_RejectsBothMissing` | ❌ Wave 0 |
| M-01 | OptionalAuthMiddleware decodes JWT when present, no-op when absent | unit | `cd services/player && go test ./internal/transport -run TestOptionalAuth` | ❌ Wave 0 |
| M-01 | Resolver also emits `ComboResolveTotal` with same label set | unit | `cd services/player && go test ./internal/service -run TestResolve_IncrementsComboCounter` | ❌ Wave 0 |
| M-01 | E2E: open anime page, change language within 30s, verify counter incremented | integration | `bunx playwright test combo-override.spec.ts -t "auth user language change"` | ❌ Wave 0 |
| M-01 | E2E: anonymous user override flow end-to-end | integration | `bunx playwright test combo-override.spec.ts -t "anon user team change"` | ❌ Wave 0 |
| M-02 | Grafana dashboard panel JSON parses (no syntax errors) | unit | `cd docker/grafana/dashboards && jq . preference-resolution.json > /dev/null` | ❌ Wave 0 |
| M-02 | PromQL `rate(combo_override_total[5m])` returns non-empty after E2E test triggers events | manual-semi | `curl -s 'https://admin.animeenigma.ru/prometheus/api/v1/query?query=combo_override_total' \| jq '.data.result \| length > 0'` | manual |
| M-02 | Grafana panel renders with "Auto-Pick Override Rate" title | manual | Open `https://admin.animeenigma.ru/grafana/d/preference-resolution-v1` | manual |
| M-02 | Baseline snapshot ≥ 24h captured and recorded in PROJECT.md | manual | `cat .planning/PROJECT.md \| grep -A 10 "Baseline override rate"` | manual (Phase gate before Phase 6) |

### Sampling Rate

- **Per task commit:** Run the unit suite for the touched layer.
  - Composable changes: `bunx vitest run src/composables/useOverrideTracker.test.ts`
  - Backend handler changes: `cd services/player && go test ./internal/handler/...`
  - Metrics changes: `cd libs/metrics && go test ./...`
- **Per wave merge:** Full unit suite + lint.
  - Backend: `cd services/player && go test ./...`
  - Frontend: `cd frontend/web && bunx vitest run && bunx eslint src/ && bunx tsc --noEmit`
- **Phase gate:** Full suite green + Playwright E2E green + manual production verification:
  1. `make redeploy-player` → health green
  2. Curl smoke tests (auth + anon)
  3. PromQL probe returns non-zero for both counters
  4. Grafana panel renders
  5. 24h soak: confirm baseline appears in PROJECT.md

### Wave 0 Gaps

These must be created in Wave 0 before any implementation work:

- [ ] `frontend/web/vitest.config.ts` — Vitest config; install via `bun add -d vitest @vue/test-utils happy-dom`
- [ ] `frontend/web/src/composables/useOverrideTracker.test.ts` — covers M-01 composable detection logic
- [ ] `frontend/web/src/views/Anime.test.ts` — covers M-01 player-dimension tracking
- [ ] `frontend/web/tests/e2e/combo-override.spec.ts` — covers M-01 E2E flow (auth + anon)
- [ ] `services/player/internal/handler/override_test.go` — covers M-01 backend handler (counter + log + validation)
- [ ] `services/player/internal/transport/optional_auth_test.go` — covers M-01 middleware
- [ ] (Optional) `services/player/internal/service/preference_test.go` — extend `resolver_test.go` to assert `ComboResolveTotal` increments

**If no gaps:** N/A — Wave 0 has 6-7 gap files to create. None of the test infrastructure for these specific units exists; existing `go test` and Playwright work, but no Vitest config exists yet for unit-testing composables.

**Manual checks (cannot be fully automated, document in VALIDATION.md):**
- Production PromQL probe (requires production traffic to be non-zero)
- Grafana visual rendering (requires browser)
- 24h baseline capture (requires real time to pass)

## Sources

### Primary (HIGH confidence)
- `services/player/internal/transport/router.go` — verified routing structure, AuthMiddleware shape
- `services/player/internal/handler/preference.go` — verified handler template
- `services/player/internal/service/preference.go` — verified increment site for new counter
- `services/player/internal/service/resolver.go` — verified ResolvedCombo shape (Tier, TierNumber)
- `services/player/internal/domain/preference.go` — verified WatchCombo / ResolvedCombo types
- `services/gateway/internal/transport/router.go` — verified `/api/users/*` JWT-protected (Critical Finding 1)
- `libs/metrics/watch.go` — verified existing CounterVec patterns
- `libs/metrics/metrics.go` — verified Collector pattern (untouched by this phase)
- `libs/authz/jwt.go` — verified JWT manager + Claims shape for OptionalAuth
- `libs/httputil/middleware.go` + `libs/httputil/response.go` — verified Bind/Error/OK helpers
- `libs/logger/logger.go` — verified zap-based Infow shape
- `frontend/web/src/composables/useWatchPreferences.ts` — verified composable convention; identified anon short-circuit (Critical Finding 3)
- `frontend/web/src/composables/useImageProxy.ts` + `useAuth.ts` — verified composable patterns
- `frontend/web/src/api/client.ts` — verified axios interceptor pattern for X-Anon-ID propagation
- `frontend/web/src/types/preference.ts` — verified WatchCombo/ResolvedCombo type contract
- `frontend/web/src/components/player/{Kodik,AnimeLib,HiAnime,Consumet}Player.vue` — verified picker-handler line refs and currentCombo computed locations
- `frontend/web/src/views/Anime.vue` — verified host view, player switch site (videoProvider)
- `docker/grafana/dashboards/preference-resolution.json` — verified existing dashboard structure (Pattern: Resolution Tier panel id 2)
- `docker/grafana/provisioning/dashboards/dashboards.yml` — verified file-provider auto-load mechanism
- `docker/loki/loki-config.yml` — verified retention is 168h (Critical Finding 2)
- `docker/promtail/config.yml` — verified scrape labels (container, service, project)
- `docker/docker-compose.yml:189-235` — verified Loki, Promtail, Grafana wiring + volume mounts
- `Makefile:228` — verified `redeploy-<service>` target

### Secondary (MEDIUM confidence)
- MDN Web Docs — `crypto.randomUUID()` browser support [CITED: https://developer.mozilla.org/en-US/docs/Web/API/Crypto/randomUUID] (training-knowledge cross-checked, current as of Apr 2026)
- Prometheus best practices — cardinality limits and `metric_relabel_configs` [CITED: https://prometheus.io/docs/practices/instrumentation/#do-not-overuse-labels] (training)

### Tertiary (LOW confidence)
- (None — every claim in this document is anchored to a verified file or to a well-documented widely-deployed browser API.)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already in repo, versions verified
- Architecture: HIGH — every integration point verified by reading the file
- Pitfalls: HIGH — all derived from reading code paths, not external sources
- Critical findings 1-3: HIGH — each anchored to a specific file:line and resolution recommended
- Validation Architecture: MEDIUM — Vitest is recommended but does not currently exist in repo (Wave 0 gap); planner must validate the gap is acceptable

**Research date:** 2026-04-27
**Valid until:** 2026-05-27 (30 days — stable area, but defer-resolution reminder for any browser API surprises). The Loki retention finding (Critical 2) is fragile to docker-compose edits; re-verify before Phase 6.
