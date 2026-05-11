# Project Research Summary

**Project:** v3.0 Universal Anime Scraper
**Domain:** Self-hosted Go scraping subsystem for English anime stream resolution (Shikimori/MAL ID → HLS m3u8 + VTT subs), replacing dead `aniwatch` / broken `consumet` paths in a small (~10 active users) self-hosted deployment where the dev server **is** production.
**Researched:** 2026-05-11
**Confidence:** MEDIUM-HIGH (HIGH on the Go HTTP+goquery layer, ID-mapping, and architecture seam; MEDIUM-LOW on AnimeKai stream decryption because every public extractor in 2026 still routes through the same `enc-dec.app` service whose contract change broke our Consumet pipeline)

## Executive Summary

v3.0 replaces two dead EN provider paths (`aniwatch` + `consumet-api` containers) with an in-house Go scraping subsystem **extended into `services/catalog/` rather than spawned as a new service**. Three reinforcing findings drive this: (a) the cutover already requires touching catalog (delete `parser/hianime/`, `parser/consumet/`, rewire `catalog.go:44-45`), so a separate service doubles deploy-coordination during the most dangerous moment of the milestone; (b) catalog already owns every other provider parser and the Shikimori upsert path; (c) cutover discipline (Phase E deletes dead code only after ≥ 7 days clean prod traffic) is much easier with a single service redeploy. The contract the new subsystem exposes is byte-identical in shape to today's `domain.HiAnimeStream` DTO, so the existing `HiAnimePlayer.vue` and `ConsumetPlayer.vue` change ~10 lines each and the rest of the frontend (SubtitleOverlay, HLS proxy, Jimaku JP-sub composable) is untouched.

The stack additions are deliberately minimal and Go-native: `goquery@v1.10.3` (Go-1.21-compatible) for HTML traversal, `golang.org/x/time/rate` for per-host token-bucket limiting, `hashicorp/go-retryablehttp` to replace the hand-rolled retry loops in the current parsers, and `dop251/goja` for the one specific job of evaluating AnimePahe's Kwik packer-style JS. **Colly is rejected** (last release March 2024, 14+ months stale); **chromedp/Playwright/uTLS/cloudscraper are all rejected** — the 2026-05-09 live triage shows plain `curl` against animekai.to / animepahe / anitaku.io returns real HTML 200s, so we are explicitly **not** facing a Cloudflare-JS-challenge problem today. The existing `docker/megacloud-extractor/` Node sidecar is **kept** for embed decryption (the regex/key rotation pattern is exactly where a JS-permissive parser earns its keep) and called over HTTP from the Go layer.

The single biggest risk this milestone carries is that AnimeKai's stream-URL extraction currently routes through `https://enc-dec.app` in every public reference implementation (Aniyomi, walterwhite-69, Sheets-Astrum-BOT), and that is **the exact remote service whose body-shape change broke our Consumet pipeline**. Three structural mitigations are non-negotiable and must land in Phase A, not Phase D: (1) a per-stage `provider_health_up{provider, stage}` gauge so dead-upstream-looking-up never repeats (ISS-007/009); (2) golden-file fixtures + sentinel-error contract so silent selector drift surfaces as a hard error instead of `[]Episode{}, nil` (Pitfall 5/8); (3) a hard architectural rule that the EN-source endpoint returns `{sources[], tracks[], …}` or HTTP 404 — **never** a Kodik iframe URL across-tier (Pitfall 1 / ISS-008 / `feedback_animelib_no_kodik_fallback.md`). Recommended provider order: **ship AnimePahe first**, then AnimeKai behind an explicit "may break" feature flag, then Anitaku as a v3.1+ cold spare. See "Provider Order Reconciliation" below — this is the one place the four research files disagree, and the tie-breaker is our own production incident history.

## Key Findings

### Recommended Stack

The new subsystem stays inside the existing Go monorepo with a minimal additions list. Pure-Go HTTP + DOM traversal handles search/info/episodes for all three target providers. JS evaluation is needed in exactly one place per provider (Kwik for AnimePahe, MegaCloud-style decryption for AnimeKai), and we have two well-scoped tools for those: `dop251/goja` for the pure-CPU Kwik unpacker, and the existing `docker/megacloud-extractor/` Node sidecar (called over HTTP) for MegaCloud-style embed decryption.

**Core technologies (additions only — Go 1.22, Chi, GORM, Postgres, Redis, `libs/{logger,metrics,cache,errors,videoutils,idmapping}`, and `megacloud-extractor` are GIVENS):**
- **`github.com/PuerkitoBio/goquery@v1.10.3`** — jQuery-style HTML DOM traversal. Pinned to v1.10.3 (last Go-1.21-compatible tag; v1.12.0 requires Go 1.25).
- **`golang.org/x/time/rate`** — Per-host token-bucket limiter (1 RPS animekai/animepahe, 2 RPS anitaku, 0.5 RPS kwik.cx). Prevents Pitfall 6 (per-host bans from parallel fan-out).
- **`github.com/hashicorp/go-retryablehttp@v0.7.7+`** — Replaces hand-rolled retry loops in `hianime/client.go:258` and `consumet/client.go:223`.
- **`github.com/dop251/goja`** — Pure-Go ES5/ES6-partial JS runtime. Used in exactly one place: AnimePahe's Kwik packer unpacking. No DOM, no network.
- **`github.com/sebdah/goldie/v2`** — Golden-file snapshot testing for parser determinism (Pitfall 5/12).
- **`net/http/cookiejar` + `golang.org/x/net/publicsuffix`** — Per-provider scoped cookie jar (DDoS-Guard cookies for AnimePahe).

**Explicitly rejected with reasons:** `gocolly/colly` (14+ months stale), `chromedp`/`go-rod`/Playwright (image bloat + Pitfall 11 no-headless-browser rule), `refraction-networking/utls`/`bogdanfinn/tls-client` (no JA3 blocks observed), `cloudscraper_go`/`FlareSolverr` (ineffective against modern Cloudflare per Scrapfly 2026), `@consumet/extensions` (the dependency that just died).

### Expected Features

**Must have (table stakes, all P1 — ship in v3.0):**
- TS-01 Shikimori-ID → provider-ID via `malsync.moe` (verified live), 24h cache, fuzzy fallback
- TS-02 HLS m3u8 + Referer/Origin headers through existing `libs/videoutils/proxy.go`
- TS-03 Episode list with sub/dub split + `is_filler` flag
- TS-04 EN VTT subtitle track from AnimeKai (AnimePahe degrades gracefully)
- TS-05 Multi-provider **sequential** failover (never parallel — Pitfall 6)
- TS-06 Per-provider Prometheus health metric (prevents ISS-007 silent-death repeat)
- TS-07 Cutover removes dead `aniwatch` + `consumet` containers
- TS-08 Frontend players repointed with ~10-line diff each
- TS-09 Hard 10s timeout on every upstream call
- TS-10 TTL'd URL cache: 30 min streams, 6h episodes, 15m search

**Should have (P1 architectural payoff):**
- DIFF-01 `Provider` interface from day one (mirrors v2.0 `SignalModule` payoff)
- DIFF-02 Liveness probe (15-min cron with golden-anime pool, marks unhealthy on 3 consecutive failures)
- DIFF-05 Subtitle-track normalization (ISO 639-1 language code)
- DIFF-07 ReportButton emits `provider:<name>` field

**Defer (P2 / v3.1):**
- DIFF-04 Fuzzy title fallback (only if > 5% malsync miss rate)
- DIFF-06 `/api/admin/scraper/diag/:shikimoriId` debug endpoint
- Anitaku/Gogoanime as third provider (domain volatility 5+ rotations in 18 months; coverage overlap high)

**Anti-features explicitly rejected:**
- Parallel fan-out across providers (Pitfall 6 / ANTI-02)
- Iframe-only providers (kills SubtitleOverlay — ANTI-01)
- Cross-tier fallback EN→Kodik iframe (Pitfall 1 / ISS-008 — already shipped twice)
- Custom AES key from training data (ANTI-08 — aniwatch's failure mode)
- Pre-populating provider catalogs (CLAUDE.md policy)
- Auto-pick "first with results" ignoring per-anime preference (breaks `feedback_watch_preferences.md`)

### Architecture Approach

Single new tree at `services/catalog/internal/parser/scraper/` with the interface + orchestrator + per-provider clients. Failover lives in the **orchestrator at the service layer**, never in per-provider clients (Anti-pattern 1 — the Consumet `FallbackProviders` mistake). Handlers stay thin HTTP-shape-only.

**Major components:**
1. **`Provider` interface** (`provider.go`) — `Name`, `FindID`, `ListEpisodes`, `ListServers`, `GetStream`, `HealthCheck`. Sentinel errors: `ErrNotFound`, `ErrProviderDown`, `ErrExtractFailed`. `Stream` DTO byte-identical to today's `HiAnimeStream`.
2. **`Orchestrator`** (`orchestrator.go`) — Sequential failover, per-host rate limiter, 60s in-memory health cache, 30m Redis stream-URL cache. Marks unhealthy on `ErrProviderDown`; skips for next 60s.
3. **Per-provider clients** — `animepahe/client.go` (goja for Kwik), `animekai/client.go` (calls megacloud-extractor sidecar over HTTP). Ignorant of each other.
4. **`MegacloudClient`** — Thin Go HTTP wrapper around `http://megacloud-extractor:3200/extract?url=...`. No new Go decryption.
5. **New HTTP routes** under `/api/anime/{id}/scraper/{episodes,servers,stream,health}` + `/api/anime/scraper/search`. Zero gateway changes (slots into existing `/api/anime/*`).
6. **Reused libs unchanged:** `libs/idmapping`, `libs/cache`, `libs/metrics`, `libs/videoutils/proxy.go` (only `HLSProxyAllowedDomains` array changes).

### Critical Pitfalls

1. **Silent cross-tier provider fallback (Pitfall 1 / ISS-008 / `feedback_animelib_no_kodik_fallback.md`)** — Shipped this bug **twice** already. Prevention is architectural: EN endpoint's DTO struct **has no `iframe_url` field**. No EN source → HTTP 404 with `{reason, tried[]}`.
2. **Dead upstream that "looks UP" (Pitfall 2 / ISS-007 / ISS-009)** — aniwatch's `/health` was 200 for 9 days while `hianime.to` was dead. Prevention: per-stage health gauge `provider_health_up{provider, stage}` for 5 stages (search/episodes/servers/stream/stream_segment); alert on `stream_segment == 0 for 15m`.
3. **Caching short-lived signed stream URLs longer than expiry (Pitfall 3 / ISS-001)** — Kwik/Megacloud URLs sign with 15-30 min (sometimes 60s) TTLs. Prevention: cache the *resolution work*, not the URL. `parseExpiry(url) → time.Time` per provider; cap at `min(parsedExpiry - 30s, 5min)`.
4. **Decryption fragility / inheriting external-service deps (Pitfall 4)** — Every public AnimeKai extractor depends on `enc-dec.app` — the service that broke Consumet. Prevention: extend `megacloud-extractor` with in-house token generator (~50 LOC); delete `patch-aniwatch.sh` (Anti-pattern 4: monkey-patching vendored npm).
5. **Parallel fan-out → per-host bans (Pitfall 6)** — 3 providers × 10 users × 3 mirrors = 90 concurrent connections; soft-banned in minutes. Prevention: sequential orchestration only; per-host `rate.Limiter` + `semaphore.Weighted` (weight=2); exponential backoff 1s/2s/4s/8s; 5-min circuit-break.

Honorable mention: **Pitfall 11 — anti-bot scope creep** encoded as architectural rule. No Puppeteer, no Playwright, no headless Chrome, no residential proxies, no TLS-fingerprint spoofing. Tripping these means "pick a different provider," not "escalate tooling."

## Provider Order Reconciliation

The four research files disagree on which provider to ship FIRST. This is the only material disagreement, and the tie-breaker is our own incident history.

**The disagreement:**
- **STACK.md** → AnimePahe first; AnimeKai's `enc-dec.app` dependency is the exact pattern that just broke Consumet.
- **FEATURES.md** → AnimeKai first (clean malsync mapping + inline WebVTT subs); in-house token generator is "~50 LOC".
- **ARCHITECTURE.md** → AnimeKai-first in Phase B/D, but the assumption is bolted on; architecture works either way.
- **PITFALLS.md** → Doesn't pick, but Pitfall 4 warns against inherited external-service deps and Pitfall 10 says "Phase 4 ships exactly one provider."

**Recommendation: ship AnimePahe FIRST.** AnimeKai in Phase D behind feature flag. Anitaku in v3.1+. Reasoning:

1. **STATE.md + the Consumet incident are the strongest evidence we have.** The `enc-dec.app` contract change is *the* triage event that started this milestone. Building v3.0 around the same external service risks the same single-point-of-failure.
2. **AnimePahe's risks are local and bounded.** Pure-CPU Kwik unpacking via goja. No external decryption service. DDoS-Guard handled by cookie jar (verified by Aniyomi using no headless browser). Worst case "Kwik changes obfuscator" is local re-test/fix.
3. **AnimeKai's TS-04 subtitle advantage is real but recoverable.** AnimePahe degrades to no separate VTT track — video is still EN audio. JP subs go through Jimaku independently (ARM AniList ID resolution doesn't care about provider).
4. **Pitfall 10 is binding.** "Phase 4 ships exactly one provider; provider 2 only added after a documented user-coverage gap." Shipping AnimeKai first inverts the risk budget.
5. **The in-house `enc-dec.app` replacement is real R&D, not 50 LOC.** STACK.md is right: "belongs in its own Phase, not 'just add goja'." Bundling it into the first shippable provider doubles Phase B's scope at the highest-risk moment.

**Concrete phase order:** A (scaffolding) → B (AnimePahe + ConsumetPlayer cutover) → C (liveness probe + per-stage health metrics) → D (AnimeKai + in-house token generator + HiAnimePlayer cutover) → E (cutover: delete dead containers + parser dirs + patch-aniwatch.sh, gated on ≥ 7 days clean traffic). Anitaku deferred to v3.1.

This reorders ARCHITECTURE.md §9's Phase B/D mapping (AnimeKai was Phase B there) but preserves the cutover invariants exactly: at every commit boundary, ≥ 1 EN player works.

## Implications for Roadmap

### Phase A — Foundation: Contract, scaffolding, golden-file harness
**Rationale:** PITFALLS.md is explicit — 6 of 12 pitfalls (1, 4, 5, 6, 8, 11) have prevention that is architectural and cannot be retrofitted cheaply. Get the seam right before any provider ships. No production behavior change.
**Delivers:** `Provider` interface + DTOs + sentinel errors; `Orchestrator` skeleton with zero providers; `MegacloudClient` Go HTTP wrapper; `make capture-goldens` + `testdata/` convention; `verifyPageShape` sentinel contract; per-host `rate.Limiter` + `semaphore.Weighted` primitives; new HTTP routes registered (returning 404 until Phase B); CI lint check on `go.mod` enforcing no chromedp/rod/utls/tls-client/cloudscraper deps.
**Addresses features:** DIFF-01 (Provider interface architectural payoff).
**Avoids pitfalls:** 1 (DTO has no `iframe_url` field — type-level enforcement), 4 (Go-owned decryption documented), 5+8+12 (golden-file harness exists before any parser), 6 (sequential-only orchestrator), 10 (no providers this phase), 11 (no-headless-browser CI rule).

### Phase B — First provider: AnimePahe + endpoint live + ConsumetPlayer cutover
**Rationale:** Structurally-safest provider (local Kwik only, no external decryption). Ships table stakes TS-01/02/03/05/06/09/10 end-to-end. Consumet tab cutover first because it's the more-broken one today.
**Delivers:** `parser/scraper/animepahe/client.go` + goja Kwik unpacker + goldens; orchestrator registers AnimePahe; new HTTP routes serve real responses; frontend `scraperApi` added; `ConsumetPlayer.vue` swaps to `scraperApi` with `prefer: 'animepahe'`. AnimePahe CDNs `kwik.cx`, `owocdn.top`, `uwucdn.top` are already in `HLSProxyAllowedDomains`.
**Addresses features:** TS-01, TS-02, TS-03, TS-05 (orchestrator in place), TS-06, TS-08 (one player), TS-09, TS-10, DIFF-07.
**Acceptance:** Consumet tab serves playable HLS for previously-failing test anime; `parser_requests_total{provider="animepahe"}` ticks up; ID mapping < 500ms p95.
**Avoids pitfalls:** 3 (TTL ≤ 30 min, Kwik expiry parsed), 6 (per-host limiter active), 9 (dark-cutover — old consumet container still running).

### Phase C — Per-provider observability: liveness probe + per-stage health metrics
**Rationale:** ISS-007 and ISS-009 are the most expensive incidents in this codebase's history. Lands between providers, not after both, so the second provider can't hide the first's silent degradation.
**Delivers:** `LivenessProbe` background goroutine (15 min ± 20% jitter, rotating 5-10 golden-anime pool); `provider_health_up{provider, stage}` gauges for 5 stages (search/episodes/servers/stream/stream_segment); orchestrator skips unhealthy in last 60s; Grafana alert on `stream_segment == 0 for 15m`; `GET /api/admin/scraper/health` endpoint.
**Addresses features:** DIFF-02, TS-06 (full).
**Avoids pitfalls:** 2 (full pipeline test, per-stage gauges), 5 (`parser_zero_match_total{provider, selector}`).

### Phase D — Second provider: AnimeKai with in-house token generator + HiAnimePlayer cutover
**Rationale:** Brings AnimeKai online after Phase B/C have proven AnimePahe + observability in prod. In-house token generator (~50 LOC extension of `megacloud-extractor`) is FEATURES.md's correct mitigation for inherited `enc-dec.app` fragility. Behind feature flag for first week.
**Delivers:** `parser/scraper/animekai/client.go` + goldens; `megacloud-extractor/server.js` extended with `/animekai-token` endpoint (no `enc-dec.app` call); orchestrator registers AnimeKai as second provider; `HiAnimePlayer.vue` swaps to `scraperApi` with `prefer: 'animekai'`; AnimeKai CDN domains (`megacloud.tv`, `netmagcdn.com` — verify during impl) appended to `HLSProxyAllowedDomains`.
**Addresses features:** TS-04 (EN VTT subs), TS-05 (real failover testable), TS-08 (second player).
**Acceptance:** Both EN tabs playable; forcing AnimeKai down still produces stream via AnimePahe fallback; `parser_fallback_total{from="animekai"}` increments.
**Avoids pitfalls:** 4 (no `enc-dec.app` dep — owned locally), 9 (feature flag on; old aniwatch still running one more phase).

### Phase E — Cutover: delete dead code
**Rationale:** Pitfall 9 (cutover bugs from premature deletion) is the only HIGH-recovery-cost pitfall. Mandatory ≥ 7 days clean prod traffic on new scraper.
**Delivers:** Delete `parser/hianime/` + `parser/consumet/`; delete 7 handler funcs; delete 6 old routes; delete `AniwatchAPIURL` + `ConsumetAPIURL` config; delete `aniwatch` + `consumet` blocks from docker-compose.yml; delete `docker/megacloud-extractor/patch-aniwatch.sh` (Anti-pattern 4 dies); delete `hianimeApi`/`consumetApi` from frontend `client.ts`; Redis cache bust for `search:*`/`stream:*`/`episodes:*`; verify `docker ps` shows neither dead container.
**Addresses features:** TS-07.
**Acceptance:** Catalog image size drops; `grep -E "aniwatch|consumet-api" docker/docker-compose.yml` returns nothing.
**Avoids pitfalls:** 4 (patch-aniwatch.sh deleted), 9 (≥ 7 days + ≤ 5% per-provider error rate gate).

### Phase Ordering Rationale

- **A → B → C → D → E** mirrors ARCHITECTURE.md §5's cutover discipline: at every commit boundary, ≥ 1 EN player works.
- **Foundation phase A mandatory** — 6 of 12 pitfalls have architectural-only prevention.
- **Observability phase C between providers**, not after both, so provider 2 can't hide provider 1's silent degradation (ISS-009 shape).
- **Single-PR phase E** is cheapest way to honor "7 days clean" guardrail without dragging dead containers through multiple deploys.
- **Anitaku deferred to v3.1** per Pitfall 10 — coverage overlap with AnimeKai+AnimePahe is high; domain volatility (5+ rotations in 18 months) makes maintenance expensive.

### Research Flags

Phases likely needing deeper research (invoke `/gsd-research-phase`):
- **Phase D (AnimeKai + in-house token generator)** — *the* technically-risky deliverable. Reverse-engineer the token alphabet substitution + AES-CBC variant from Aniyomi/walterwhite-69 sources; verify against real animekai.to embed; decide Node-side (extend `server.js`) vs Go-side (`crypto/aes` + `crypto/md5`) implementation. STACK.md flags this LOW confidence. PITFALLS.md flags it as the structural risk.
- **Phase C (per-stage health metrics)** — gauge family + label cardinality (5 stages × N providers × M golden anime ≈ 100 series) needs design before code. Verify Prometheus storage budget; align with `libs/metrics/parser.go` patterns.

Phases with standard patterns (skip deeper research):
- **Phase A** — pure architectural scaffolding; STACK.md + ARCHITECTURE.md detailed enough. Mirrors v2.0 `SignalModule` pattern.
- **Phase B (AnimePahe)** — FEATURES.md provider table + STACK.md kwik guidance + KevCui/animepahe-dl reference are sufficient. Goja unpacker ~15 LOC equivalent.
- **Phase E (cutover)** — ARCHITECTURE.md §9 enumerates every file with line numbers. Mechanical; the 7-day soak is the hard part.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All version dates verified within 2 months of today; Colly rejection grounded in concrete release cadence; goquery v1.10.3 vs v1.12.0 anchored to our actual Go 1.22. |
| Features | MEDIUM-HIGH | HIGH on ID-mapping (live-probed malsync.moe) and player contract (read from current code). MEDIUM on AnimeKai/MegaUp long-term reliability (every 2026 ref impl still uses `enc-dec.app`). LOW on AniZone (Cloudflare 403'd both probes; no public ref impl) — **dropped from v3.0**. |
| Architecture | HIGH | Every claim anchored to verbatim file/line in current tree. "Extend catalog vs new service" decided on operational grounds defensible in PR review. |
| Pitfalls | HIGH | 9 of 12 pitfalls anchored to documented incidents (ISS-001/002/005/006/007/008/009/010) + feedback memories. Remaining 3 MEDIUM (general piracy-mirror patterns) explicitly flagged. |

**Overall confidence:** MEDIUM-HIGH. Architecture and stack solid; highest residual risk is AnimeKai stream extraction, bounded by Phase D feature flag + Provider Order Reconciliation (AnimePahe ships first, milestone has working EN path before AnimeKai's risk is introduced).

### Gaps to Address

- **AnimeKai token encryption algorithm RE** — every 2026 extractor routes through `enc-dec.app`. Real R&D, Phase D primary deliverable. **Mitigation:** Phase B ships AnimePahe first; not on critical path.
- **Exact AnimePahe rate-limit threshold** (LOW confidence) — needs empirical measurement in Phase B. **Mitigation:** start at 1 RPS conservative; tune up only after observation.
- **Whether `megacloud-extractor` handles AnimeKai's MegaUp variant** (LOW confidence) — existing flow targets HiAnime-flavor MegaCloud. **Mitigation:** Phase D spike against real animekai.to embed early; budget +1 fallback regex (existing `server.js:55-69` has 4 variants).
- **AnimeKai CDN allowed-domains** — exact hostnames need discovery during Phase D impl. **Mitigation:** `HLSProxyAllowedDomains` is single-array append.
- **AniList `streamingEpisodes` as canonical episode mapping** (Pitfall 7 LOW) — affects subtitle drift for filler-heavy series. **Mitigation:** Phase D surfaces drift (`subtitle_drift_warning` event when video/sub duration differs > 90s) rather than auto-correct; manual `episode_offset_map` for known-problem series.

## Sources

### Primary (HIGH confidence)
- **GitHub / pkg.go.dev verification:** `PuerkitoBio/goquery` (v1.12.0 / v1.10.3 dates), `gocolly/colly` (v2.2.0 March 2024 — rejection basis), `refraction-networking/utls` (v1.8.2), `bogdanfinn/tls-client` (v1.14.0), `hashicorp/go-retryablehttp`.
- **Direct repo inspection of competing scrapers:** `Kohi-den/.../animekai/AnimeKai.kt` (confirms `enc-dec.app` dep), `Kohi-den/.../animepahe/AnimePahe.kt` (confirms Kwik local-only + cookie-jar DDoS-Guard), `walterwhite-69/AnimeKAI-API` (same `enc-dec.app` pattern in Python — structural risk), `KevCui/animepahe-dl` (Kwik m3u8 technique).
- **Local code (read in pass):** `docker/megacloud-extractor/server.js` (lines 36-205 AES-256-CBC + cinemaxhq keys); `services/catalog/internal/parser/{kodik,hianime,consumet}/client.go`; `services/catalog/internal/transport/router.go` (50-98); `services/catalog/internal/handler/catalog.go` (685-842); `services/catalog/internal/service/catalog.go` (37-135, 1640-1820); `libs/videoutils/proxy.go` (230-255); `libs/cache/ttl.go`; `libs/metrics/parser.go`; `libs/idmapping/client.go`; `frontend/web/src/components/player/{HiAnime,Consumet}Player.vue`; `frontend/web/src/api/client.ts`.
- **AnimeEnigma production incidents (`docs/issues/README.md`):** ISS-001 (Cloudflare 403 as HLS), ISS-002 (missing CDN allow-list), ISS-005 (sequential search — *intra-provider* parallelization only), ISS-006 (mobile Safari HLS), ISS-007 (HiAnime 9-day silent death), ISS-008 (AnimeLib Kodik silent swap), ISS-009 (Go client + health check different paths), ISS-010 (Shikimori .one → .io strips POST body).
- **Project memories:** `feedback_animelib_no_kodik_fallback.md` (2026-05-09 "AniLib must mean AniLib"), `feedback_verify_streams.md`, `project_deployment.md`, `MEMORY.md` (Shikimori = MAL IDs, ARM endpoint).
- **Live probe 2026-05-11:** `GET api.malsync.moe/mal/anime/30276` → confirms TS-01 viability.
- **STATE.md 2026-05-09 triage:** verified-alive providers (animekai.to, animepahe, anitaku.io, anizone.to) vs verified-dead (hianime.*, aniwatchtv.to, kaido.to, aniwave.to, animekai.bz); aniwatch-api repo deleted; `riimuru/consumet-api` 5-months-stale calling `enc-dec.app` with wrong body shape.

### Secondary (MEDIUM confidence)
- HiAnime ecosystem shutdown March 2026 + USTR "notorious market" + ~900 repos purged (Wondershare 2026 roundup, Protocloud).
- Consumet hosted endpoints shut down (`consumet/api.consumet.org#725`); npm package providers age-out (`consumet/consumet.ts#613` `anitaku.bz → anitaku.io`).
- Kwik.cx packer-eval pattern confirmed across multiple downloader projects (anime-dl PR #316).
- AnimePahe/Kwik signed-URL TTL "15-30 min typical" — needs per-provider verification in Phase B.

### Tertiary (LOW confidence — flagged for Phase-D verification)
- Exact AnimeKai token encryption algorithm.
- Whether `megacloud-extractor` handles AnimeKai's MegaUp variant out of the box.
- AniList `streamingEpisodes` as canonical episode mapping (Pitfall 7).
- `yuzono/aniyomi-extensions#416` (page DMCA-451; signal-only from search snippets).
- AniZone (`anizone.to`) — Cloudflare 403; no public ref impl in 2026. **Dropped from v3.0.**

---
*Research completed: 2026-05-11*
*Ready for roadmap: yes — 5 phases (A scaffolding, B AnimePahe + Consumet-tab cutover, C health metrics, D AnimeKai + HiAnime-tab cutover, E dead-code deletion). Anitaku deferred to v3.1. AniZone dropped.*
