# PlatformStats → "Trump-style" Joke Card — Design

**Date:** 2026-05-25
**Workstream:** hero-spotlight
**Status:** Approved (brainstorm) — pending spec review
**Scope:** Rewrite the existing `platform_stats` spotlight card in place.

---

## 1. Summary

The `platform_stats` spotlight card currently ships a single real metric
(`anime_added_7d`) into a layout designed for a hero stat + a 2×2
supporting grid — so the grid renders empty and the card looks dead on a
small self-hosted site.

This redesign repurposes the same layout into a **self-aware, bombastic
"Trump-style" platform status card**:

- **Hero line:** `Работает: ДА | Аптайм: ОЧЕНЬ МНОГО 99.4% | catalog — UXΔ +5 · CDI 0.00 * 99 · MVQ Dragon 99%/99%` + a rotating tagline.
- **2×2 micro-grid:** 4 randomly-chosen, non-zero real metrics from
  Prometheus, each over a randomly-chosen window (day / week / all-time).

The card discriminator stays `platform_stats`; only its payload and
rendering change.

### Project metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — turns a near-empty dead card into a lively,
  always-populated one; pure delight, no task-flow regression.
- **CDI = 0.04 * 13** — touches the resolver, types, one new tiny Prom
  client, one compose env var, the SFC, and locale cleanup (medium spread);
  introduces a new-but-compatible pattern (Prometheus read from catalog);
  medium multi-module effort.
- **MVQ = Sprite 88%/90%** — small, delightful, whimsical polish that
  leans hard into the project's own voice (UXΔ/CDI/MVQ as a punchline).

---

## 2. Decisions (locked during brainstorm)

| # | Decision | Choice |
|---|----------|--------|
| D1 | Joke pool source | **Hand-authored static pool**, `go:embed`-ed into the catalog binary. No LLM, no scheduler. |
| D2 | Tile data source | **Prometheus query API** (`http://prometheus:9090/prometheus/api/v1/query`). |
| D3 | Hero composition | **Grounded health** (`Работает`/`Аптайм` from real Prometheus `up`) + **canned UXΔ/CDI/MVQ** from the pool, attached to a **daily-random real service name**. |
| D4 | Replace vs new | **Replace `platform_stats` in place** (keep the type discriminator). |
| D5 | Localization | **No i18n / no locale switching.** Pool composed at **80/10/10 RU/EN/JP**, picked uniformly; **everyone sees the same** daily pick verbatim. Chrome is fixed Russian. |

---

## 3. Determinism & "updated daily"

Reuses the mechanics already proven by the `latest_news` card:

- **Daily cache key** `spotlight:stats:<DateKeyUTC>` — the assembled payload
  is computed once per UTC day and served from cache for the rest of the
  day. Live Prometheus numbers are therefore *snapshotted* into the daily
  cache (= "updated daily", not live-ticking).
- **Date-seeded RNG** — `rand.New(rand.NewSource(seed))` where `seed` is an
  FNV-1a hash of the `DateKeyUTC` string. Today's random picks (tagline,
  quip, service, which 4 tiles, which window per tile) are therefore:
  - **stable** for the whole day even across a cache miss / multiple
    catalog instances,
  - **different** the next day,
  - **reproducible** in tests (inject a fixed seed).
- **Everyone sees the same** — `userID` is ignored and the daily pick is
  global; combined with no locale switching, every viewer sees byte-for-byte
  identical card content on a given day.

---

## 4. Data model

### 4.1 Backend payload (`services/catalog/internal/service/spotlight/types.go`)

The existing `PlatformStatsData` / `StatsMetric` structs are **replaced**:

```go
// PlatformStatsData is the payload for Card{Type: "platform_stats"}.
type PlatformStatsData struct {
    Hero  StatsHero   `json:"hero"`
    Tiles []StatsTile `json:"tiles"` // 0..4, daily-randomized, non-zero only
}

// StatsHero is the bombastic top line. Working/Uptime are REAL (Prometheus);
// the rest is canned joke content (single-language strings) from the pool.
type StatsHero struct {
    WorkingOK     bool     `json:"working_ok"`               // real: all targets up?
    UptimePercent *float64 `json:"uptime_percent,omitempty"` // real: avg_over_time(up[7d]); nil if Prom unreachable
    UptimeQuip    string   `json:"uptime_quip"`              // pool: e.g. "ОЧЕНЬ МНОГО"
    Service       string   `json:"service"`                  // daily-random real service name
    UXDelta       string   `json:"ux_delta"`                 // pool/canned: "+5 (Tremendous)"
    CDI           string   `json:"cdi"`                      // pool/canned: "0.00 * 99"
    MVQ           string   `json:"mvq"`                      // pool/canned: "Dragon 99%/99%"
    Tagline       string   `json:"tagline"`                  // pool: the rotating one-liner
}

// StatsTile is one micro-grid cell — a single Prometheus metric over one window.
type StatsTile struct {
    Label  string `json:"label"`  // from allowlist entry (single-language)
    Value  int64  `json:"value"`  // non-zero (filtered)
    Window string `json:"window"` // "day" | "week" | "all"
    Format string `json:"format"` // "int" | "bytes" | "seconds"
}
```

`Tiles` MUST be initialized as `[]StatsTile{}` (never nil) so it marshals as
`[]` not `null` (frontend treats `null` as a parse failure — same rule as
the other Phase-3 cards).

### 4.2 Frontend type (`frontend/web/src/types/spotlight.ts`)

The `PlatformStatsData` variant of the `SpotlightCard` discriminated union
is rewritten to mirror §4.1 exactly (`hero: StatsHero`, `tiles: StatsTile[]`).
The old `StatsMetric` interface is removed.

---

## 5. Embedded config (hand-authored, `go:embed`)

Two JSON files live next to the resolver under
`services/catalog/internal/service/spotlight/cards/` and are embedded via
`//go:embed`:

### 5.1 `platform_stats_jokes.json` — hero joke pool

```json
{
  "taglines": [
    "Лучшая платформа для аниме. Никто не стримит лучше нас. Поверьте.",
    "Best anime platform. Nobody streams like us. Believe me.",
    "最高のアニメサイト。誰にも負けない。本当だよ。"
  ],
  "uptime_quips": ["ОЧЕНЬ МНОГО", "ОГРОМНЫЙ", "ЛУЧШИЙ В МИРЕ"],
  "vibes": [
    { "ux_delta": "+5 (Tremendous)", "cdi": "0.00 * 99", "mvq": "Dragon 99%/99%" },
    { "ux_delta": "+5 (Bigly)",      "cdi": "0.01 * 88", "mvq": "Phoenix 95%/95%" }
  ]
}
```

- **Composition target:** `taglines` and `uptime_quips` authored at
  **~80% RU / 10% EN / 10% JP**. Uniform random pick → exposure tracks the
  ratio. `vibes` are language-neutral (numbers + creature names from the
  CONVENTIONS taxonomy: Phoenix / Griffin / Kraken / Sprite / Basilisk / Dragon).
- **Starting size:** ~24 taglines, ~8 quips, ~8 vibe sets (final copy is the
  implementer's; this file is the single place to tune humor later).
- `tagline`, `uptime_quip`, and `vibe` are drawn **independently** with the
  daily RNG.

### 5.2 `platform_stats_prom.json` — tile query allowlist

```json
[
  {
    "id": "http_requests",
    "label": "Запросов обработано",
    "metric": "http_requests_total",
    "format": "int",
    "windows": ["day", "week", "all"]
  },
  {
    "id": "response_bytes",
    "label": "Отдано данных",
    "metric": "http_response_size_bytes_sum",
    "format": "bytes",
    "windows": ["day", "week", "all"]
  }
]
```

- `label`s authored mostly in Russian (audience-weighted), single-language.
- **Window → PromQL** (built by the client, never from user input):
  - `day`  → `increase(<metric>[1d])`
  - `week` → `increase(<metric>[7d])`
  - `all`  → `<metric>` (raw cumulative counter; "all-time" ≈ since service
    start, bounded by Prometheus retention — documented as intentional).
- `format` drives frontend display (`int` → `toLocaleString`, `bytes` →
  humanized, `seconds` → `0.3s`).
- **Starting size:** ~10 entries spanning request counts, response bytes,
  request durations, and `up`-derived counts across services.

---

## 6. Components & data flow

### 6.1 Prometheus client — `services/catalog/internal/parser/prometheus/`

New, tiny, single-purpose:

```go
type Client struct { baseURL string; http *http.Client }

// Query runs an instant PromQL query and returns the first scalar/vector
// sample value. Returns (0, err) on transport / decode / empty-result.
func (c *Client) Query(ctx context.Context, promql string) (float64, error)

// Health returns (allUp bool, uptimePct float64, err error):
//   allUp      = (count(up == 0) == 0)
//   uptimePct  = avg_over_time(up[7d]) * 100, clamped 0..100
func (c *Client) Health(ctx context.Context) (bool, float64, error)
```

- Base URL from `PROMETHEUS_SERVICE_URL` env (e.g.
  `http://prometheus:9090/prometheus`); endpoint
  `GET {base}/api/v1/query?query=<promql>`. **Note the `/prometheus`
  route-prefix** (compose runs Prometheus with `--web.route-prefix=/prometheus`).
- Short timeout (~800ms) to respect the aggregator's per-card deadline.
- An interface (`promQuerier`) is defined in the cards package so tests inject
  a handwritten fake (no testify/mock — project rule).

### 6.2 Resolver — `cards/platform_stats.go` (rewritten)

`Resolve(ctx, _ *string)`:

1. **Cache GET** `spotlight:stats:<DateKeyUTC>` → return on hit.
2. **Seed RNG** from the date (FNV-1a of `DateKeyUTC`).
3. **Hero:**
   - `prom.Health()` → `WorkingOK` + `UptimePercent`. On error: `WorkingOK=false`,
     `UptimePercent=nil` (chrome renders `ТЕХНИЧЕСКИ ДА`, no number) — non-fatal.
   - Pick `tagline`, `uptime_quip`, `vibe` from the embedded pool via RNG.
   - Pick `service` from a fixed list of real backend services
     (auth, catalog, streaming, player, rooms, scheduler, themes,
     notifications, gateway) via RNG.
4. **Tiles:** shuffle the allowlist via RNG; for each entry pick a random
   valid window via RNG, build PromQL, `prom.Query()`. Keep only **`value > 0`**.
   Stop at 4 kept (or allowlist exhausted). Per-query errors are logged WARN
   and skipped (non-fatal).
5. **Eligibility:** the hero is pool-backed and **always** available → the
   card is **always eligible** (never returns `(nil, nil)`), even if
   Prometheus is fully down (then: `WorkingOK=false`, no uptime number,
   `Tiles: []`). This is the intended, funniest failure mode.
6. **Cache SET** the assembled payload with `cardTTL`; return the card.

### 6.3 DI — `cmd/catalog-api/main.go`

`NewPlatformStatsResolver(...)` gains a `*prometheus.Client` (constructed
from `PROMETHEUS_SERVICE_URL`) and the date-seeded RNG plumbing. Position in
the `spotlightResolvers` slice is unchanged.

### 6.4 Compose — `docker/docker-compose.yml`

Add to the **catalog** service block (mirror the gateway block):

```yaml
PROMETHEUS_SERVICE_URL: http://prometheus:9090/prometheus
```

(catalog already shares the Docker network with `prometheus`; add a
`depends_on`/start-order note only if needed — the resolver degrades
gracefully if Prometheus isn't up yet.)

### 6.5 Frontend — `PlatformStatsCard.vue` (rewritten)

- **Single-root `<article>`**, no top-level `v-if` (Transition-safety rule).
- **Hero (left):** `Работает: {ДА | ТЕХНИЧЕСКИ ДА}` from `working_ok`;
  `Аптайм: {uptime_quip} {uptime_percent}%` (omit the number when nil);
  vibe row `{service} — UXΔ {ux_delta} · CDI {cdi} · MVQ {mvq}`; the
  `tagline` rendered prominently.
- **Micro-grid (right, 2×2):** `v-for` over `tiles` — each cell shows the
  Russian window badge (`ЗА ДЕНЬ`/`ЗА НЕДЕЛЮ`/`ЗА ВСЁ ВРЕМЯ`), the formatted
  value (`format`-aware), and the `label`. 0 tiles → grid renders nothing
  (hero only).
- **No `t()`, no `localeStr`** — strings rendered verbatim. Chrome constants
  (`title`, `Работает`, `Аптайм`, badges, `ДА`/`ТЕХНИЧЕСКИ ДА`) are fixed
  Russian literals in the SFC.
- **UI-SPEC contract preserved:** only `font-medium`/`font-semibold`,
  `p-4 md:p-6 lg:p-8`, `min-h-[400px] md:min-h-[340px] lg:min-h-[320px]`,
  Tailwind utility-only.
- The old `Sparkline` + `DeltaChip` usage is **removed** from this card (they
  were tied to the single-counter hero). The components stay in the tree for
  potential reuse by other cards.

---

## 7. Localization cleanup

- Remove the `spotlight.platformStats.*` namespace from **both**
  `frontend/web/src/locales/en.json` and `ru.json`.
- Update `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts` so the
  parity expectations no longer include `platformStats` keys.

---

## 8. Error handling & edge cases

| Situation | Behaviour |
|-----------|-----------|
| Prometheus unreachable | `WorkingOK=false`, `UptimePercent=nil`, `Tiles=[]`. Card still renders (hero from pool). Logged WARN. |
| Some tile queries fail / return 0 | Those tiles skipped; fewer than 4 tiles is fine. |
| Fewer than 4 non-zero metrics exist | Grid shows however many were found (0..3). |
| Embedded JSON malformed at build | Caught by an `init()`/load-time `json.Unmarshal` with a test asserting both files parse + are non-empty. |
| Cache get/set error | Logged WARN, compute proceeds / response still returned (existing pattern). |

---

## 9. Testing

### Backend (`platform_stats_test.go`, rewritten)

Handwritten fake `promQuerier` + fixed RNG seed:

1. **Happy path** — fake returns healthy + non-zero values → card has
   `WorkingOK=true`, an uptime %, a service name, all vibe fields, and exactly
   4 non-zero tiles.
2. **Daily stability** — same seed twice → identical picks (tagline / quip /
   service / tile set / windows).
3. **Non-zero filter** — fake returns 0 for some metrics → those tiles absent.
4. **Prometheus down** — fake errors on every call → card still eligible,
   `WorkingOK=false`, `UptimePercent=nil`, `Tiles=[]`.
5. **Pool integrity** — embedded JSON parses, `taglines`/`uptime_quips`/`vibes`
   and allowlist are non-empty.
6. **`types_test.go`** — round-trip marshal/unmarshal of `PlatformStatsData`;
   `Tiles` marshals as `[]` when empty.

### Frontend (`PlatformStatsCard.spec.ts`, rewritten — ≥5 assertions)

1. Renders `ДА` when `working_ok`, `ТЕХНИЧЕСКИ ДА` when not.
2. Renders uptime quip + percent; omits percent when `uptime_percent` absent.
3. Renders the vibe row (service + UXΔ/CDI/MVQ) and the tagline verbatim.
4. Renders N tiles with the correct Russian window badge + formatted value.
5. 0 tiles → hero renders, grid empty.

### Parity

`spotlight-keys.spec.ts` passes after the `platformStats` keys are removed.

### Verify command

```bash
cd services/catalog && go test ./internal/service/spotlight/... ./internal/parser/prometheus/... -count=1 -race
cd frontend/web && bunx vitest run src/components/home/spotlight/cards/PlatformStatsCard.spec.ts src/locales/__tests__/spotlight-keys.spec.ts && bunx tsc --noEmit
```

---

## 10. Out of scope / YAGNI

- LLM-generated jokes, a scheduler refresh job, per-user or per-locale
  variation, Grafana dashboard/API auth integration, historical metrics
  beyond Prometheus retention, sparkline/delta on this card.

---

## 11. Files touched

**New**
- `services/catalog/internal/parser/prometheus/client.go` (+ `client_test.go`)
- `services/catalog/internal/service/spotlight/cards/platform_stats_jokes.json`
- `services/catalog/internal/service/spotlight/cards/platform_stats_prom.json`

**Rewritten**
- `services/catalog/internal/service/spotlight/cards/platform_stats.go` (+ `_test.go`)
- `services/catalog/internal/service/spotlight/types.go` (`PlatformStatsData`, `StatsHero`, `StatsTile`; drop `StatsMetric`)
- `services/catalog/internal/service/spotlight/types_test.go`
- `frontend/web/src/types/spotlight.ts` (`PlatformStatsData` variant)
- `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue` (+ `.spec.ts`)

**Edited**
- `services/catalog/cmd/catalog-api/main.go` (DI: Prom client + RNG)
- `docker/docker-compose.yml` (catalog `PROMETHEUS_SERVICE_URL`)
- `frontend/web/src/locales/en.json`, `ru.json` (remove `platformStats.*`)
- `frontend/web/src/locales/__tests__/spotlight-keys.spec.ts`
