# Scraper Framework Reference

Canonical map of how the EN scraper (`services/scraper/`) works end-to-end:
failover orchestrator, route family, typed errors, providers+embeds, Camoufox
stealth sidecar, DB roster, and the analytics playability probe.

---

## 1. Overview — OurEnglish Failover Orchestrator

The `services/scraper/` microservice (port 8088) owns all EN subtitle/stream
resolution. Its core is the `Orchestrator`
(`services/scraper/internal/service/orchestrator.go`), which holds a registered
list of providers and runs sequential failover across them.

**Default failover chain** (registration order = failover order,
`cmd/scraper-api/main.go` lines 330–455):

```
gogoanime → animepahe → allanime-okru → miruro → nineanime
```
(`allanime-okru` folds the former standalone `allanime` + `okru` providers into
one AllAnime-GraphQL-discovery + ok.ru-stream-resolution provider, 2026-07-06 —
the dead `api.allanime.day` clock-sync path was dropped in the merge. Historical
`allanime` and `animefever` rows remain disabled DB tombstones, but neither is
part of the compile-time fallback roster. AnimeKai has no runtime package or
configuration.)

**Registration mechanics** (`orchestrator.go:76–94`):
- `Register(p)` — adds a provider to the auto-failover chain.
- `RegisterDegraded(p)` — adds a provider that is reachable via an explicit
  `prefer` pin but is **excluded from automatic failover**. Degraded providers are stored in
  `Orchestrator.degraded` map and skipped by `orderedProviders` unless the
  caller explicitly requested them via `prefer`.

**In-memory health cache** (`services/scraper/internal/health/`): the orchestrator
optionally consults an `InMemoryHealthCache`. If a provider's gauge reads DOWN,
`runFailover` skips it, emits `parser_fallback_total{from,to}`, and continues to
the next provider. The cache is fail-open (missing/stale entries return healthy).

**Per-provider timeout** (`SCRAPER_PROVIDER_TIMEOUT`, default 8s, with
`SCRAPER_BROWSER_PROVIDER_TIMEOUT` default 35s for DB `engine=browser`): each
provider call runs under a sub-context deadline; a
budget timeout while the parent context is still alive is reclassified as a
retryable `"provider_timeout"` fallback rather than a terminal error (ISS-022).

---

## 2. Route Family — `prefer` vs `exclusive`

All four scraper routes are exposed via the catalog gateway at:

```
GET /api/anime/{uuid}/scraper/episodes?prefer=<name>&exclusive=<bool>
GET /api/anime/{uuid}/scraper/servers?episode=<id>&prefer=<name>&exclusive=<bool>
GET /api/anime/{uuid}/scraper/stream?episode=<id>&server=<id>&category=sub|dub&prefer=<name>&exclusive=<bool>
GET /api/anime/{uuid}/scraper/health
```

### Catalog passthrough

The gateway routes `/api/anime/*` to `catalog:8081`. The catalog handler
(`services/catalog/internal/handler/scraper.go`) reads `prefer` and
`exclusive=true` from the query string and forwards both transparently to the
scraper client (`services/catalog/internal/parser/scraper/client.go`), which
appends `?exclusive=true` when set. The catalog service layer
(`services/catalog/internal/service/scraper.go`) resolves the anime UUID →
MAL/Shikimori ID before calling the scraper; the stream body is signed for the
HLS proxy before being returned (`streamsign.SignScraperStreamBody`).

### `prefer` (soft)

Moves the named provider to position 0 in the failover order; the rest of the
chain remains. Unknown or blank `prefer` values are silently ignored
(regex-gated to `^[a-z0-9_-]{1,64}$` in `handler/scraper.go:142`).

### `exclusive=true` (no-failover)

`orderedProviders(prefer, exclusive=true)` returns a slice containing **only**
the preferred provider (`orchestrator.go:151–155`). Any other provider in the
chain is never tried. If `prefer` is unset or does not match any registered
provider, the returned slice is empty and the handler emits 503 `NO_PROVIDERS`.

This is the "honest availability" probe path: `exclusive=true` is the only
correct way to test whether a **specific** provider has a given anime without
masking failures behind another provider's success.

### `meta.tried` and `meta.provider`

Every response (success and error) includes `meta.tried` (the ordered provider
list the orchestrator would have used). `meta.provider` (the failover winner)
is emitted **only on `/scraper/episodes` success** — `/scraper/servers` and
`/scraper/stream` call `writeSuccess` without the provider argument, so they
never emit it. The frontend uses `meta.provider` from the episodes call to pin
subsequent `servers`/`stream` calls to the same provider. Episode/server IDs
are opaque and provider-specific; handing a foreign ID to the wrong provider
produces silently empty results (`FindIDNamed` docstring, `orchestrator.go:410`).

---

## 3. Typed Errors → HTTP

Defined in `services/scraper/internal/domain/errors.go`:

| Domain error | Meaning | HTTP | Code |
|---|---|---|---|
| `ErrNotFound` | Provider answered; anime/episode not there | 404 | `NOT_FOUND` |
| `ErrProviderDown` | Provider unreachable (timeout, 5xx, anti-bot) | 502 | `PROVIDER_DOWN` |
| `ErrExtractFailed` | Provider responded; parse/decrypt failed | 502 | `EXTRACT_FAILED` |
| `context.Canceled` / `DeadlineExceeded` | Caller cancelled | 499 | `INTERNAL` |
| (other) | Unknown | 500 | `INTERNAL` |

Mapped in `handler/scraper.go:writeOrchestratorError` (lines 627–640).

**Failover semantics** (`orchestrator.go:failoverDecision`, lines 174–188):
- `ErrNotFound`, `ErrProviderDown`, `ErrExtractFailed` → retryable (advance to
  next provider).
- `context.Canceled`, `context.DeadlineExceeded` (from **parent** context) →
  terminal, stop immediately.
- Unknown errors → treated as `ErrProviderDown` (defensive).

`summarizeFailover` collapses per-provider errors: any non-`ErrNotFound` error
wins; if every provider returned `ErrNotFound`, return `ErrNotFound`
(`orchestrator.go:202–218`).

---

## 4. Providers + Embeds

### Provider directories

```
services/scraper/internal/providers/
├── gogoanime/       # PRIMARY: Anitaku/Gogoanime (EN sub/dub, gatedProvider)
├── animepahe/       # 2nd: AnimePahe via Camoufox resolver sidecar (Kwik embed)
├── allanimeokru/    # 3rd: AllAnime (OK.ru) — AllAnime GraphQL discovery + ok.ru
│                    # stream resolution (id "allanime-okru"). Folded 2026-07-06
│                    # from the former allanime + okru providers; clock path dropped.
├── miruro/          # Miruro, pure-Go secure-pipe transform (Phase 28)
├── nineanime/       # 9anime.me.uk — last-resort MP4 (Phase 28)
└── eighteenanime/   # 18+ group, separate orchestrator (adultOrch)
```

### Embed extractors

```
services/scraper/internal/embeds/
├── kwik.go            # AnimePahe embed (kwik.cx / kwik.si)
├── megacloud.go       # MegaCloud-compatible embed extraction
├── megaplay.go        # Gogoanime / 9anime megaplay.buzz HLS player
├── vibeplayer.go      # Gogoanime VibePlayer embed
├── streamhg.go        # Gogoanime StreamHG embed
├── earnvids.go        # Gogoanime EarnVids embed
└── packed_common.go   # Shared Dean-Edwards packer helpers
```

Registry order matters: `domain.Registry.Find` returns the **first** match.
Registration order is defined in `cmd/scraper-api/main.go`; keep more-specific
extractors ahead of generic MegaCloud/Megaplay matches.

### Provider pipeline

Each provider implements `domain.Provider` interface, which defines the 4-step
pipeline:

1. **`FindID(ctx, AnimeRef)`** — resolves MAL/Shikimori ID → provider-internal
   anime ID. Uses malsync lookup, then fuzzy title search fallback.
2. **`ListEpisodes(ctx, providerID)`** — returns `[]domain.Episode` (with
   `HasSub`, `HasDub` flags; providers that don't distinguish default to
   `HasSub=true` in `ListEpisodesNamed`, `orchestrator.go:457`).
3. **`ListServers(ctx, providerID, episodeID)`** — returns available embed
   servers for the episode.
4. **`GetStream(ctx, ...)` / `GetStreamGated(ctx, ...)`** — extracts the
   playable HLS/MP4 `*domain.Stream` (sources + headers + subtitles).

**Gated provider** (`orchestrator.go:488–569`): gogoanime implements the
optional `gatedProvider` interface (`GetStreamWithGate`). On the cold path
(empty `server` param), it runs `ListServers` + iterates the configured
`SCRAPER_SERVER_PRIORITY` list to select the best embed, then probes it
for playability before returning. The `gated` bool in `meta.gated` tells
the frontend whether the three-phase loader's Phase 3 ran (SCRAPER-HEAL-04/07).

---

## 5. Stealth Sidecar (Camoufox)

**Location:** `services/stealth-scraper/` (Python/FastAPI + Camoufox Firefox).

**Why:** Certain CDNs (e.g. gogoanime → megaplay player on `mewstream.buzz` /
`cinewave2.site`) require a real browser TLS/HTTP2/JS fingerprint that a Go
`net/http` curl-class client cannot reproduce. Swapping exit IPs does not help;
the constraint is client identity (JA3 + HTTP/2 fingerprint + JS engine).

**How it's triggered:** The `engine` column in `stream_providers` (catalog DB)
controls whether a provider uses the Go scraper or the sidecar. Both
**gogoanime** and **nineanime** support `engine = "browser"`. When set, the Go
scraper calls `sidecar.New(cfg.StealthScraperURL)` to delegate stream extraction
(`main.go:305–311` for gogoanime, `main.go:446–449` for nineanime):

```go
gogoUseBrowser := func() bool {
    return cfg.Providers.EngineOf("gogoanime") == config.EngineBrowser
}
nineUseBrowser := func() bool {
    return cfg.Providers.EngineOf("nineanime") == config.EngineBrowser
}
```

**HTTP contract:**
- `POST /resolve` — resolve a stream (retains a browser session).
- `GET /hls?sid=&url=` — MANDATORY stream proxy; fetches playlist/segments
  via the retained browser context (clearance cookies bound to exit IP+UA).
- `DELETE /session/{sid}` — release browser session.

The sidecar drives a warm pool of persistent Camoufox browser profiles
(`app/engine.py`), intercepts the `getSources` + `.m3u8` network requests
the player JS fires, then rewrites playlists so all segment URIs route back
through `/hls`. Full architecture and lessons: [[project_stealth_scraper_camoufox]].

**Note:** direct IP proved clean on 2026-06-20 (no residential proxy needed for
current CDN hosts). Residential proxy support remains available for future CDN
rotations via `STEALTH_PROXIES`.

---

## 6. DB Roster — `stream_providers`

**Model:** `services/catalog/internal/domain/scraper_provider.go`
**Physical table:** `stream_providers` (renamed from `scraper_providers`
2026-06-17; see `TableName()` in that file).

The table is the **single source of truth** for every stream provider (EN chain,
18+ group, first-party players). The Go-embedded seed populates it on a fresh
DB; the scraper fetches it at boot and on a refresh interval.

### Key columns

| Column | Type | Notes |
|---|---|---|
| `name` | string PK | Canonical ID, e.g. `gogoanime` |
| `policy` | `auto\|manual\|disabled` | Auto/manual participation; disabled is the admin hard lock |
| `health` | `up\|degraded\|recovering\|down` | Probe-managed health lifecycle |
| `status` | `enabled\|degraded\|disabled` | Compatibility wire value derived from policy + health |
| `group` | `en\|adult\|ru\|firstparty` | Intrinsic provider family |
| `scraper_operated` | bool | True only for providers constructed by the scraper service |
| `engine` | `http\|browser` | `browser` → Camoufox sidecar path |
| `base_url` | string | Provider mirror origin; empty = built-in default |
| `preference_weight` | int | Ranking input for `/capabilities` |
| `supports_sub`, `supports_dub` | bool | Capability traits |

**Derived wire `status` semantics** (`domain.ScraperProvider.WireStatus`):
- `enabled` — in the auto-failover chain.
- `degraded` — registered for explicit `prefer` pin only; excluded from
  auto-failover; sorted last in the player with a "degraded" pill.
- `disabled` — not registered at all.

### Internal serving

`GET /internal/scraper/providers` (Docker-network-only; handler:
`services/catalog/internal/handler/internal_scraper_providers.go`) returns all
`stream_providers` rows as `{"providers":[...]}`.

The scraper loads this at boot (`config.LoadProvidersRemote`,
`main.go:228–236`) and refreshes periodically
(`config.StartProvidersRefresher`, `main.go:531`). The `registerByStatus`
helper (`main.go:238–255`) dispatches to `Register` / `RegisterDegraded` /
skip accordingly.

### Per-anime capabilities

`GET /api/anime/{uuid}/capabilities` (handler:
`services/catalog/internal/handler/capabilities.go`) assembles a ranked
capability report from the `stream_providers` roster plus live per-title
discovery for the surviving catalog and scraper adapters. Disabled rows and
rows without runtime wiring are omitted. Consumed by the aePlayer source picker.

---

## 7. Playability Probe

**Package:** `services/analytics/internal/probe/`
**Endpoint:** `POST /internal/probe/run` (Docker-network-only; triggered daily
by the scheduler, or on-demand by an operator).

Handler: `services/analytics/internal/handler/probe.go`. 5-minute timeout per
full provider sweep; returns 204 on success.

### How the probe runs (`probe/engine.go`)

On each `RunOnce` call:

1. **Anime set** (`probe/animeset.go`): builds 4 slots from catalog:
   - `anchor` — a fixed configured anime UUID.
   - `featured` — from `GET /api/home/spotlight` (the "featured"-type card).
   - `spotlight_random` — random pick from spotlight anime-bearing cards.
   - `random` — another random pick.

2. **For each provider**, resolves each slot with `exclusive=true` (no
   failover) via the catalog passthrough:
   - `GET /api/anime/{uuid}/scraper/episodes?prefer=<p>&exclusive=true`
   - `GET /api/anime/{uuid}/scraper/servers?episode=...&prefer=<p>&exclusive=true`
   - `GET /api/anime/{uuid}/scraper/stream?episode=...&server=...&prefer=<p>&exclusive=true`

   If the probe gets a 404 (`ErrProbeNotFound`, `resolver.go:73`), the slot
   is **skipped** and the provider gets one **re-roll** from the popular pool
   (`GET /api/anime/popular?page_size=100`, `popularpool.go:30`). Re-rolls are
   never failures; if the re-roll also 404s or fails, those verdicts stand.

3. **Validation** (`probe/validator.go`): for each resolved stream:
   - Fetches the `master.m3u8` through the **HLS proxy** (with catalog-signed
     `exp`/`sig` forwarded).
   - Follows up to 2 levels of HLS playlist nesting (master → variant →
     segment).
   - On reaching a media segment, shells out to **ffprobe** (`probe/ffprobe.go`)
     to confirm a decodable video stream.
   - Returns a `Verdict{Reason}` — `playable` or a failure reason
     (`cdn_unreachable`, `status_403`, `empty_response`, `decode_failed`,
     `invalid_video`).

4. **Scoring** (`probe/scorer.go:Rollup`):
   A slot **passes** if any of its verdicts is `playable`. The per-provider
   verdict is:
   - **`up`** — > 50% of slots passed.
   - **`degraded`** — > 0% but ≤ 50% passed.
   - **`down`** — 0% passed.
   The dominant non-playable reason (highest count; lexicographic tie-break)
   labels Degraded/Down verdicts.

5. **Sinks** (`probe/reporter.go`, metric names from `libs/metrics/probe.go`):
   - Prometheus gauge `probe_provider_up{provider}` (1.0 / 0.5 / 0.0).
   - Prometheus counter `probe_runs_total{provider,slot,server,result,reason}`.
   - Prometheus gauge `probe_last_run_timestamp` (unix seconds of last run).
   - Prometheus gauge `probe_provider_status{provider,status,reason}` (value
     always 1; info-style rollup label series; reset each run to avoid stale
     label cardinality).
   - ClickHouse table `analytics.probe_runs` (90-day TTL MergeTree):
     `(run_ts, provider, anime_uuid, anime_name, slot, server, stage, reason, playable)`.

The `InMemoryHealthCache` infrastructure (`services/scraper/internal/health/`)
exists and is wired into the orchestrator's skip-unhealthy check
(`orchestrator.go:317`, `orchestrator.go:536`): when the cache marks a provider
DOWN, `runFailover` skips it and emits `parser_fallback_total`. However, the
**writer is not enabled in production** — `InMemoryHealthCache.Update()` is
defined but never called outside tests. The cache is permanently empty, so the
health check always fails open (every provider is treated as healthy). The
analytics playback probe is a **separate process** that writes verdicts to
ClickHouse and Prometheus only; it has no path into the scraper process's
in-memory struct and does not affect skip decisions.

Full live-run details: [[project_unified_probe_live_first_run]].
