# Scraper Health — Reference

How the EN scraper microservice (`services/scraper/`) evaluates its constructed
provider chain (gogoanime → animepahe → allanime-okru → miruro → nineanime),
and how that knowledge is surfaced and acted on.

Three layers:

1. **Liveness probe** — one background goroutine per registered provider that
   exercises the full 5-stage pipeline against real anime every ~15 min.
2. **Health cache + orchestrator gate** — an in-memory, fail-open cache the
   failover loop consults to skip providers the probe confirmed broken.
3. **Surfacing** — Prometheus gauges, the Grafana *Playback Health* dashboard,
   two alert rules, and three HTTP endpoints.

---

## 1. The liveness probe

Code: `services/scraper/internal/health/probe.go` (`ProbeRunner`).
Spawned in `cmd/scraper-api/main.go` (~line 570): after all providers are
`Register()`-ed and **before** `ListenAndServe`, one `go runner.Start(probeCtx)`
per EN provider. Probes run in the same process as user traffic on purpose —
they share the cookie jar and HTTP-client identity, so a probe success means
the *user-serving* path works too. On SIGTERM, `probeCancel()` fires before
server shutdown.

### Cadence

| Knob | Value | Where |
|---|---|---|
| Base interval | 15 min | `probeBaseInterval` |
| Jitter | ±20 % (clamped ≥ interval/2) | `probeJitterPct`, `nextSleep` |
| Initial delay | random 0 … interval/4 | `computeInitialDelay` |
| Initial-delay override (tests/CI) | `SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS` | env |

### The 5 stages

Stage names are a **versioned contract** — they appear verbatim as Prometheus
label values and in alert rules (`internal/health/stage.go`):

| # | Stage | What runs | Failure condition |
|---|---|---|---|
| 1 | `search` | `provider.FindID(ref)` | error |
| 2 | `episodes` | `provider.ListEpisodes` | error or 0 items |
| 3 | `servers` | `provider.ListServers` | error or 0 items |
| 4 | `stream` | `provider.GetStream` on up to the **first 2** servers (mirrors the orchestrator's walk past embed-page servers that return `ErrExtractFailed`) | error or no sources |
| 5 | `stream_segment` | bounded GET that follows the HLS chain to a **real media segment** (see below) | non-2xx, empty body, still-a-playlist after 2 hops |

On the first failing stage the tick **short-circuits** — later stages are not
exercised (wasted load on a broken upstream, and their failures would be
derivative). Skipped stages are written to the cache as `Up=false` with
`LastErr: "skipped: upstream stage X failed this tick"` (`markRemainingStale`),
but their *windows* are untouched — uncheckable ≠ failed.

### Stage 5 details (`fetchSegment`)

The naive version of this stage (GET `Sources[0].URL`, accept any 2xx) was
wrong twice, and both fixes are load-bearing:

- **ISS-021 — playlists lie.** For HLS, `Sources[0].URL` is the master/media
  *playlist*, which stays 200 even when the `.ts` segments 502. `fetchSegment`
  now follows the playlist (master → variant → segment, max
  `maxSegmentFollowDepth = 2` hops) and only counts success when a non-playlist
  body with actual bytes downloads. Proxy-rewritten URIs inside playlists are
  unwrapped back to the upstream URL before the next hop.
- **AUTO-125 — probe must take the user's path.** Most provider CDNs reject a
  direct GET from the scraper (signed URLs, Referer gates, geo-fences), which
  made `stream_segment` flap DOWN while real users streamed fine. Probe fetches
  that go through the local streaming service's proxy
  (`http://streaming:8082/api/v1/hls-proxy?url=…&referer=…`) must replay the
  stream's provenance signature (`exp`/`sig`) — the proxy's trust gate is
  `first-party OR signed` since the static allowlist was retired (2026-07-14).

Guard rails on this stage:

- `segmentTimeout = 10 s` per fetch; playlist bodies are read up to 64 KiB
  (`maxPlaylistReadBytes`).
- **Bounded drain** (`maxDrainBytes = 64 KiB`): the cleanup defer discards at
  most 64 KiB of unread body before `Close`. Before 2026-06-10 the drain was
  unbounded — for progressive-MP4 sources (allanime `fast4speed`, nineanime)
  every tick downloaded ~50 MB of episode through the HLS proxy until the 10 s
  timeout cut it, pinning streaming's hls-proxy P95 at 13 s+. Regression test:
  `TestProbe_FetchSegmentDoesNotDrainLargeBody`.
- **SSRF (REVIEW BLK-01)**: re-validated on *every* hop — http(s) schemes only,
  non-empty host, loopback/RFC-1918/link-local/docker-service-name hosts
  rejected (`isPrivateOrLoopback`), redirects never followed
  (`http.ErrUseLastResponse`; a 3xx counts as stage failure), depth cap defends
  against self-referential playlists.

### Golden pool

`internal/health/golden.go` — a hand-curated, 5-entry list of evergreen anime
(Naruto, One Piece, Attack on Titan, …) with MAL IDs verified against jikan.moe
(2026-05-12). Each tick picks **one** entry at random. Entries carry romaji
`AltTitles` because some providers index under romaji; without them the probe both fails and poisons the
shared per-MAL-ID slug cache. **Maintenance trap:** a wrong MAL ID produces
permanent `search`-stage false-negatives.

### Panic safety

Two recover layers: `runOneTickSafely` absorbs per-tick provider panics (loop
keeps ticking); `Start`'s outer recover catches loop-body panics and **exits
without respawning** (REVIEW BLK-03 — respawn would hot-loop a deterministic
panic). A dead probe is detected via the missing heartbeat metric, not a
restart.

---

## 2. DOWN/UP semantics, cache, and the orchestrator gate

### Sliding window (`internal/health/window.go`)

Per (provider, stage): a stage flips DOWN only after **3 failures within
15 min** (`failureThreshold` / `failureWindow`); a **single success** resets it
UP. Asymmetric on purpose — failures accumulate, successes are decisive.

### In-memory cache (`internal/health/cache.go`)

The probe writes a `ProviderHealth{Stages, LastUpdated}` snapshot per tick;
the orchestrator reads it on **every request**. `IsHealthy(provider)` is
**fail-open** — it returns `false` (skip the provider) only when *all* of:

1. an entry exists,
2. it is fresh (≤ `cacheStaleTTL = 30 min`, covering two nominal probe intervals),
3. it contains a `stream_segment` key (i.e. the tick reached stage 5),
4. that stage is `Up == false`.

Everything else — unknown provider, stale entry, short-circuited tick — fails
open so **a probe outage can never blank the service**. Deliberate divergence
(REVIEW WR-03): a provider whose *search* stage is broken keeps receiving
traffic (users may still succeed via failover) — paging the operator is the
alert rules' job, not the gate's.

### Orchestrator gate (`internal/service/orchestrator.go`, `runFailover`)

Before dispatching to each provider in the failover chain, the orchestrator
checks `cache.IsHealthy(name)`. A skip increments
`parser_fallback_total{from,to}` and records an `ErrProviderDown`-wrapped error
(so an all-skipped chain doesn't masquerade as NotFound).

Slow providers are bounded by the orchestrator's per-provider deadline
(`SCRAPER_PROVIDER_TIMEOUT`, or the longer browser-engine override). A timeout
is classified as a retryable provider failure while the parent request remains
alive. The catalog `stream_providers` policy/health state is the durable source
of truth and is hot-reloaded by the scraper.

---

## 3. Surfacing: metrics, endpoints, dashboard, alerts

### Prometheus metrics (`libs/metrics/provider.go`)

| Metric | Labels | Meaning |
|---|---|---|
| `provider_health_up` | `provider`, `stage` | 0/1 per (provider, stage), post-window (3-in-15-min) |
| `provider_probe_last_tick_timestamp` | `provider` | Unix ts heartbeat; `time() - …` > ~20 min ⇒ dead probe goroutine |

At boot, `main.go` seeds all five stage gauges for every provider (EN **and**
18+) via `bootHealthSeedValue` so they appear in Grafana immediately. Only EN
providers get a probe goroutine — the golden pool is mainstream anime, so
**18anime health stays at its boot seed**. The heartbeat seeds to 0.

**Kodik is probed elsewhere:** `services/catalog/internal/service/health_checker.go`
(`PlayerHealthChecker`, 5-min ticker, Naruto search probe) emits
`provider_health_up{provider="kodik", stage="liveness"}` from the catalog
service, so the RU iframe player shares the same dashboard rows despite not
being part of the EN failover chain.

### HTTP endpoints

| Route | Reached via | Returns |
|---|---|---|
| `GET /health` (scraper-local, port 8088) | docker healthcheck | plain process liveness — *not* provider health |
| `GET /scraper/health` | gateway → catalog `GET /api/anime/{uuid}/scraper/health` (the FE uses the `_` placeholder id) → catalog's scraper client | public per-provider snapshot: per-stage up/down, `enabled`/`up`/`reason`/`description` from the operator ProvidersConfig, plus a `playable` map |
| `GET /scraper/health/admin` | gateway `GET /api/admin/scraper/health` (JWT + admin role at the gateway) | the public snapshot **plus** the probe cache's enriched view: per-stage `LastOK` timestamps and `LastErr` excerpts (truncated to 256 chars, `MaxLastErrChars`) |

`GET /scraper/health` semantics worth knowing (ISS-021 fix): each provider's
own `HealthCheck()` self-report covers only API liveness — providers never
fetch segments — so the handler **overlays the probe's real byte-oracle** onto
`stream_segment`. Entries older than `playabilityFreshTTL = 30 min` (2 probe
ticks) are treated as "no recent playability probe": `stream_segment` is shown
red and the provider is **omitted** from `playable` (absent = unknown, never a
fake green). The top-level `up` bool considers only the four non-segment
stages, so a freshly booted provider isn't reported down before its first tick.
The 18+ chain has a parallel route family (`/anime18/health`,
`/anime18/health/admin`) served by a separate orchestrator that is never part
of the EN failover chain.

### Grafana + alerts (`docker/grafana/`)

- Dashboard: `dashboards/playback-health.json` — overall stage-health ratio,
  per-provider management table (joins `provider_enabled` +
  `provider_health_up` + `provider_info`; includes Kodik and 18anime), per-stage
  timelines, and probe-heartbeat age (`time() - provider_probe_last_tick_timestamp`).
- Alerts (`provisioning/alerting/rules.yml`):
  - **Kodik Player Unavailable** — `provider_health_up{provider="kodik"} == 0`
    for 30 min.
  - **Scraper Provider Stream-Segment Down** —
    `max by (provider) (provider_health_up{stage=~"stream|stream_segment"}) == 0`
    for **4 h** (a single provider flapping is routine; the failover chain
    absorbs it — only a sustained outage pages).

### Tracing/egress attribution

Probe traffic runs off-request, so `main.go` seeds the probe context with
baggage `scheduled_job:scraper-health-probe` — its egress effects attribute to
that name in the Activity Register instead of `goroutine/unknown`.

---

## Operator cheat-sheet

```bash
# Live provider snapshot (public shape)
curl -s http://localhost:8088/scraper/health | python3 -m json.tool

# Enriched admin view (LastOK / LastErr per stage) — via gateway with admin JWT
curl -s https://animeenigma.ru/api/admin/scraper/health -H "Authorization: Bearer …"

# Which providers are gauge-down right now?
curl -s 'http://localhost:9090/prometheus/api/v1/query' \
  --data-urlencode 'query=provider_health_up == 0'

# Dead probe goroutines (heartbeat older than 20 min)
curl -s 'http://localhost:9090/prometheus/api/v1/query' \
  --data-urlencode 'query=time() - provider_probe_last_tick_timestamp > 1200'

# Take a misbehaving provider out of the chain (catalog DB — single source of
# truth, hot-reloaded within ~60s, no restart):
#   UPDATE stream_providers SET policy='manual' WHERE name='animepahe';
```

Known issues touching this system: ISS-017 (probe noise vs real downs,
romaji titles), ISS-021 (playlist validation), ISS-022 (the now-bounded
per-provider timeout), and the 2026-06-10
bounded-drain fix (probe downloaded whole episodes; see
`docs/issues/README.md`).
