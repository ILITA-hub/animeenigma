# Phase 18: 9anime — Context

**Gathered:** 2026-05-12
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous --auto)

<domain>
## Phase Boundary

A second alive EN provider is in rotation so a single provider failure does not blank the English tab for users.

**Concretely, this phase delivers:**

1. A **new scraper provider package** at `services/scraper/internal/providers/9anime/` implementing the `domain.Provider` interface for the 9anime source. Mirrors the AnimePahe package layout (`client.go`, `dto.go`, `malsync.go`, `cache.go`, plus tests). Uses the **canonical Phase 17 stage keys** (`StageSearch`, `StageEpisodes`, `StageServers`, `StageStream`, `StageStreamSegment`).
2. **Malsync-based ID resolution** with fuzzy fallback — identical to the AnimePahe contract (24h cache, fuzzy uses the same Levenshtein-distance helper). Cache key: `malsync:9anime:<mal_id>`.
3. **WordPress/Madara HTML scraping** for `ListEpisodes` using the `bsx`/`bixbox`/`bs`/`bt` class family (per requirements). Cached 6 hours, surface sub/dub split where present.
4. **`ListServers` parses embed URLs** from 9anime's per-episode page. Each distinct embed host (mp4upload, streamsb, streamtape, megacloud variants — **exact set discovered during impl**) is registered as a named `EmbedExtractor` in the existing registry so future providers reuse them.
5. **`GetStream`** resolves an embed URL via `ListServers`, then dispatches to the matching `EmbedExtractor`. **No embed extraction logic lives inside the 9anime client itself** — only HTML scraping + URL extraction. The Phase 15 EmbedExtractor registry pattern owns the per-host extraction.
6. **HLS proxy allowlist extension** — append 9anime's CDN hostnames (the resolved hosts behind mp4upload/streamsb/streamtape plus 9anime's static asset hosts) to `libs/videoutils/proxy.go::HLSProxyAllowedDomains`. Verified by a successful stream proxy in production.
7. **Orchestrator registration** — register the new provider in `cmd/scraper-api/main.go` so the existing sequential failover (AnimePahe → 9anime) just picks it up. Probe runner (Phase 17) probes the new provider automatically because it iterates `RegisteredProviders()`.
8. **Frontend dropdown enrichment** — the existing "Source: AnimePahe" dropdown in `EnglishPlayer.vue` (Phase 16) gains a second option "9anime"; user override persists via the existing `useWatchPreferences.preferredScraperProvider` store (Phase 16).
9. **`parser_fallback_total{from="animepahe",to="9anime"}` increments** when the orchestrator skips AnimePahe (health gauge 0 OR explicit error) and serves 9anime. This metric was wired in Phase 17 — this phase just makes it fire for a real pair.

**Out of scope (deferred to later phases):**
- AnimeKai (Phase 19) — flag-gated R&D path with its own in-house token generator.
- Cutover / deletion of HiAnime + Consumet code paths (Phase 20).
- Per-episode quality switching beyond what each embed extractor already supports.
- Tracing (no Jaeger / OTLP in this phase).

**Requirements covered:**
- SCRAPER-9ANI-01 (malsync + fuzzy fallback ID resolution)
- SCRAPER-9ANI-02 (WordPress/Madara markup scrape + sub/dub split + 6h cache)
- SCRAPER-9ANI-03 (embed extractors registered per host)
- SCRAPER-9ANI-04 (GetStream dispatches via extractor registry)
- SCRAPER-9ANI-05 (HLS proxy allowlist extension)
- SCRAPER-9ANI-06 (failover AnimePahe → 9anime end-to-end + fallback metric)

</domain>

<decisions>
## Implementation Decisions

### D1 — 9anime mirror viability is research-gated (CRITICAL OPEN ITEM)

The phase requirements name **"9anime"** but `STATE.md` (Phase 17 carryover from v3.0 triage 2026-05-09) records `aniwave.to` and `kaido.to` as **VERIFIED DEAD**. The original `9anime.to` rebrand chain went 9anime → aniwave → kaido and the canonical successor mirrors are reportedly down. The research subagent's **first task** is to identify which 9anime mirror (if any) is actually serving today: candidates include `9anime.org.lv`, `9animetv.to`, `9anime.gs`, `9anime.pe`, `9anime.id`, `9anime.movie`, plus the official Telegram/Discord mirror announcements. Each candidate is tested with a `HEAD /` plus a sample anime page parse. **If no 9anime mirror is alive**, the planner pivots Phase 18 to the next-best alive EN provider — likely **Anitaku/Gogoanime** (`anitaku.io` — verified alive per Phase 17 STATE) — and the requirement IDs `SCRAPER-9ANI-01..06` are mapped 1:1 to a new `services/scraper/internal/providers/gogoanime/` package with the same contract (the ROADMAP allows phase rename if the underlying provider must change; this is a known v3.0 risk).

**Why this is OK:** Phase 18's *goal* is "a second alive EN provider in rotation", not specifically the 9anime brand. ROADMAP success criteria reference embed hosts (mp4upload/streamsb/etc.) which Gogoanime also uses — the interface is portable.

**Trade-off accepted:** if 9anime IS alive on some mirror, we keep the name. If not, the planner replans to Gogoanime/Anitaku with identical scope. Final brand decision is the planner's after research.

### D2 — Provider package layout mirrors AnimePahe exactly

The new package at `services/scraper/internal/providers/9anime/` (or `gogoanime/` per D1) has the same file layout as `services/scraper/internal/providers/animepahe/`:
- `client.go` — Provider interface impl + HTTP client wiring + each stage method
- `dto.go` — response shapes for malsync + page HTML parsing helpers
- `malsync.go` — 24h cached MAL → slug resolution + fuzzy fallback
- `cache.go` — Redis cache wrappers (search 15m, episodes 6h, stream ≤ min(parsed expiry − 30s, 5min))
- `ddosguard.go` — IF the chosen mirror sits behind DDoS-Guard; OMIT if not. AnimePahe needed it; 9anime mirrors may not.
- per-file `_test.go` with offline goldens

**Why:** the analog provider establishes the contract; adding a new provider should not require re-inventing infrastructure.

### D3 — Embed extractor reuse, not duplication

The Phase 15 `EmbedExtractor` registry (`services/scraper/internal/embeds/`) already has Kwik (Phase 16). This phase **adds new extractors for whatever embed hosts the chosen provider actually uses**:
- **Likely new entries:** `mp4upload`, `streamsb`, `streamtape`, `megacloud` (if not already added by AnimePahe path).
- Each extractor is a standalone file in `services/scraper/internal/embeds/<name>/` with its own `client.go` + tests + goldens.
- The 9anime/Gogoanime client only **discovers** the embed URLs and calls `embeds.Get("mp4upload").Extract(ctx, url)` — no extraction logic in the provider package.

**Why:** new providers using the same embed hosts (likely Phase 19 AnimeKai or future) reuse the extractors without copy-paste. This is the architectural seam Phase 15 explicitly created.

### D4 — Stage definitions reuse Phase 17 canonical constants

The new provider's `HealthCheck` exposes 4 stages: `StageSearch`, `StageEpisodes`, `StageServers`, `StageStream` (the 5th, `StageStreamSegment`, is owned by the probe runner). Use the constants from `services/scraper/internal/health/stage.go`. Do NOT introduce new stage names — the Grafana dashboard and alert rule from Phase 17 depend on the 5-stage contract.

### D5 — Orchestrator failover order is config-driven (not hardcoded)

The provider registration order in `cmd/scraper-api/main.go` declares the failover sequence:
```go
orchestrator.Register(animepahe.New(...))
orchestrator.Register(_9anime.New(...))  // or gogoanime per D1
```
The existing orchestrator iterates in registration order, skipping any with `IsHealthy(provider, StageSearch) == false` per Phase 17. No new ranking logic needed — declaration order IS the order.

**User override remains via `preferredScraperProvider`** (Phase 16) — if user pinned 9anime, orchestrator tries 9anime first regardless of declaration order.

### D6 — Frontend dropdown surface — minimal change

The `EnglishPlayer.vue` source dropdown already exists (Phase 16). This phase:
- Adds a second `<option>` for the new provider in the dropdown.
- Adds a locale key for the provider's display name in `frontend/web/src/locales/{ru,en,ja}.json`.
- No new component, no layout change — the UI hint is just "two items in the dropdown instead of one".

### D7 — HLS proxy allowlist — append, don't replace

`libs/videoutils/proxy.go::HLSProxyAllowedDomains` is a string slice. Append the new hostnames; do NOT touch existing entries (AnimePahe / Kwik / Jimaku / Kodik are still in use). Hostnames discovered during impl (mp4upload's actual CDN host, streamsb's CDN host, etc.) are appended in their resolved form. **Regression-lock test:** the existing Phase 16 test asserting `pacha.kwik.cx` is in the allowlist must still pass.

### D8 — Tests offline, with goldens — same as AnimePahe

Probe + parser tests use `services/scraper/testdata/9anime/` (or `gogoanime/`) golden HTML/JSON files captured via `make capture-goldens-9anime` (new Makefile target mirroring `make capture-goldens-animepahe`). **No live network in CI.** The live probe in production via Phase 17 catches upstream death.

### D9 — Cache TTLs match the Phase 16 contract verbatim

- malsync resolution: 24h
- search: 15m
- episodes: 6h
- stream URL: `min(parsed embed expiry − 30s, 5min)` (whichever shorter)

Redis cache key prefixes: `malsync:9anime:*`, `episodes:9anime:*`, `stream:9anime:*` (or `gogoanime` per D1).

### D10 — DDoS-Guard cookie helper reuse if needed

The Phase 16 `services/scraper/internal/providers/animepahe/ddosguard.go` is a generic helper shape-wise. If the chosen 9anime mirror sits behind DDoS-Guard, **promote the helper to** `services/scraper/internal/ddosguard/` (a shared package) and import from both providers. If 9anime does NOT use DDoS-Guard, do NOT introduce the dependency.

</decisions>

<code_context>
## Existing Code Insights

- **`services/scraper/internal/providers/animepahe/{client,dto,malsync,cache,ddosguard}.go`** — analog template. Copy the layout; replace upstream calls + HTML parsers.
- **`services/scraper/internal/domain/provider.go`** — `domain.Provider` interface (Search, ListEpisodes, ListServers, GetStream, HealthCheck) and `domain.Health` with per-stage status (extended in Phase 17).
- **`services/scraper/internal/service/orchestrator.go`** — sequential failover loop. Iterates `RegisteredProviders()` in registration order. No code change needed if the new provider satisfies the interface.
- **`services/scraper/internal/embeds/`** — registry + Kwik extractor (Phase 16). New extractors for mp4upload/streamsb/streamtape live as siblings (`services/scraper/internal/embeds/<name>/`).
- **`services/scraper/internal/health/{probe,cache,stage,golden,window,testutil_provider}.go`** — Phase 17 observability. New providers are picked up automatically because the probe loops `RegisteredProviders()`. The golden pool's 5–10 anime are MAL IDs; verify each resolves on 9anime as well.
- **`services/scraper/cmd/scraper-api/main.go`** — registration point. New provider registers between AnimePahe (line ~existing) and the probe-runner goroutines.
- **`libs/videoutils/proxy.go::HLSProxyAllowedDomains`** — append-only string slice.
- **`libs/cache/`** — Redis wrappers; cache key naming uses `<resource>:<provider>:<id>` (e.g., `episodes:animepahe:<slug>`).
- **`frontend/web/src/components/player/EnglishPlayer.vue`** — Phase 16. Source dropdown already extensible.
- **`frontend/web/src/stores/watchPreferences.ts`** + `useWatchPreferences.preferredScraperProvider` — Phase 16. New provider value is just a new string in the same store.
- **`frontend/web/src/locales/{ru,en,ja}.json`** — new locale key for the provider display name in all 3 locales.
- **`docker/.env`** — new env var `SCRAPER_9ANIME_BASE_URL` (or `SCRAPER_GOGOANIME_BASE_URL` per D1) plus optional `SCRAPER_9ANIME_TIMEOUT_SECONDS` if defaults need overriding for the chosen mirror.

</code_context>

<specifics>
## Specific Ideas

### S1 — Plan structure (planner's discretion — suggested)

A reasonable 4-plan split (mirrors Phase 16's 6-plan split but scoped smaller because most infra exists):

- **18-01**: Provider mirror viability research + capture goldens. Wave 1. Researcher confirms which 9anime mirror is alive (or pivots to Gogoanime). Outputs: `services/scraper/testdata/<provider>/{search,episodes,page}.html` goldens + provider hostname constant.
- **18-02**: Provider package — `client.go`, `dto.go`, `malsync.go`, `cache.go`, tests. Wave 2 (depends on 18-01 goldens). Implements Search/ListEpisodes/ListServers/GetStream + HealthCheck. Each method offline-tested against goldens.
- **18-03**: Embed extractor additions — `services/scraper/internal/embeds/<host>/` for each new host discovered by 18-02 (mp4upload, streamsb, streamtape, megacloud variants). Wave 3 (depends on 18-02's ListServers identifying hosts).
- **18-04**: Wiring + frontend dropdown + locale + Hostname allowlist + after-update. Wave 4. Registers provider in main.go, extends HLSProxyAllowedDomains, adds dropdown option + locale strings, runs `make redeploy-scraper && make redeploy-web && make health` + changelog entry.

### S2 — Test approach

- **Offline goldens** for all parser tests (`testdata/<provider>/*.html`).
- **Live probe** from Phase 17 catches upstream death in production — no live tests in CI.
- **Failover integration test** drives a fake AnimePahe with `IsHealthy=false` plus a live-ish 9anime backed by goldens, asserts `parser_fallback_total{from="animepahe",to="9anime"}` increments.
- **Reuse `services/scraper/internal/health/testutil_provider.go`** (FakeProvider from Phase 17) where applicable.

### S3 — Failover metric verification

After deploy, force AnimePahe's health gauge to 0 (manually `curl http://localhost:8088/scraper/health/admin` to confirm gauge state — or use the env override `SCRAPER_PROBE_INITIAL_DELAY_OVERRIDE_SECONDS=5` + temporarily blackhole AnimePahe DNS). Issue a real stream request; confirm playback comes from 9anime AND `curl http://localhost:8088/metrics | grep parser_fallback_total` shows `from="animepahe",to="9anime"` non-zero.

### S4 — Provider name disambiguation

If D1 forces a pivot to Gogoanime, the planner **renames the package directory and the phase title in ROADMAP.md** (use `gsd-sdk` query if available, otherwise manual edit). Requirements IDs `SCRAPER-9ANI-01..06` keep their literal name but the implementing provider is gogoanime — REQUIREMENTS.md gets a one-line annotation: "SCRAPER-9ANI-* IDs implemented by Gogoanime provider; 9anime mirror unreachable as of 2026-05-12".

### S5 — Embed extractor SSRF guard reuse

The Phase 17 BLK-01 fix added a CheckRedirect + scheme/host allowlist to the probe's `fetchSegment`. New embed extractors that make HTTP calls (mp4upload, streamsb, streamtape) must use the **same hardened HTTP client pattern** — `BaseHTTPClient` plus the SSRF guard from `libs/videoutils/safeget.go` (or wherever the Phase 17 fix landed). Do NOT introduce a new unguarded `http.DefaultClient` call.

</specifics>

<deferred>
## Deferred Ideas

- **Per-user provider preference UI in settings panel** — out of scope. The Phase 16 `useWatchPreferences` store is enough; a Settings panel for it is a future cosmetic phase.
- **AnimeKai (third provider)** — Phase 19. Gated by feature flag; separate token-generator R&D.
- **Cutover** (delete HiAnime/Consumet) — Phase 20. Gated on ≥ 7 days clean prod traffic on EnglishPlayer.
- **Per-episode quality dropdown beyond what extractors auto-select** — out of scope. Existing extractors return highest-available quality; that's enough for v3.0.
- **Server-side analytics on which provider users prefer** — out of scope. Telemetry on the orchestrator's fallback metric is enough for v3.0; per-user analytics is v3.1+.
- **Multi-mirror failover within 9anime itself** (try 9animetv.to → 9anime.gs → ...) — out of scope. One mirror is enough; if it dies, Phase 19's AnimeKai backfills.

</deferred>
