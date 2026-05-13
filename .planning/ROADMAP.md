# Roadmap: AnimeEnigma

## Milestones

- ✅ **v1.0 Smart Watch Picker Overhaul** — Phases 1-8 (shipped 2026-05-03) — see `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v2.0 Recommendations Engine** — Phases 9-14 (shipped 2026-05-07) — see `.planning/milestones/v2.0-ROADMAP.md`
- 🟢 **v3.0 Universal Anime Scraper** — Phases 15-20 (executing; Phase 20 cutover gated on 7-day soak) — see below
- 🟢 **v3.1 Scraper Self-Healing** — Phases 21-23 (planning) — see `.planning/milestones/v3.1-ROADMAP.md`

## Phases

**Phase Numbering:**
- Phase numbering is continuous across milestones (v1.0: Phases 1-8; v2.0: Phases 9-14; v3.0: Phases 15-20)
- Decimal phases (e.g., 16.1) reserved for urgent insertions

<details>
<summary>✅ v1.0 Smart Watch Picker Overhaul (Phases 1-8) — SHIPPED 2026-05-03</summary>

- [x] Phase 1: Instrumentation Baseline (7/7 plans) — completed 2026-04-27
- [x] Phase 2: Analytics Audit (1/1 plan) — completed 2026-04-28
- [x] Phase 3: Single Source of Truth for "Watched" (5/5 tasks) — completed 2026-04-28
- [x] Phase 4: Resume State Machine in All Four Players (1/1 plan) — completed 2026-05-03
- [x] Phase 5: Analytics Gap-Fill (1/1 plan) — completed 2026-05-03
- [x] Phase 6: Tier 2 Inference Rewrite (1/1 plan) — completed 2026-05-03
- [x] Phase 7: Advanced Settings, Anonymous UX, Cross-Device Freshness (1/1 plan) — completed 2026-05-03
- [x] Phase 8: Recommendations Readiness Documentation (1/1 plan) — completed 2026-05-03

</details>

<details>
<summary>✅ v2.0 Recommendations Engine (Phases 9-14) — SHIPPED 2026-05-07</summary>

- [x] Phase 9: Recs Foundation — Interface, Ensemble, Normalizer, Schema (1/1 plan) — completed 2026-05-06
- [x] Phase 10: Population Signals, Filter, Trending Row (1/1 plan) — completed 2026-05-06
- [x] Phase 11: User Signals & "Up Next for you" Row (1/1 plan) — completed 2026-05-06
- [x] Phase 12: TF-IDF Attribute Affinity (S5) (3/3 plans across 3 waves: catalog → maintenance → player) — completed 2026-05-06
- [x] Phase 13: Combo-Watched-After Pin (S6) (1/1 plan) — completed 2026-05-06
- [x] Phase 14: Admin Debug Page & Eval Pipeline (1/1 plan) — completed 2026-05-07

</details>

### v3.0 Universal Anime Scraper (Phases 15-20)

- [x] **Phase 15: Foundation** — Provider interface, orchestrator skeleton, EmbedExtractor registry, BaseHTTPClient, megacloud-extractor Go wrapper, golden-file harness, 503-stub HTTP endpoints (completed 2026-05-11)
- [x] **Phase 16: AnimePahe + New EnglishPlayer** — First live provider (Kwik via goja), new unified `EnglishPlayer.vue` replacing both HiAnime + Consumet tabs end-to-end (completed 2026-05-12)
- [x] **Phase 17: Observability** — Per-provider/per-stage health gauges, 15-min liveness probe with golden anime pool, orchestrator skips unhealthy, Grafana alert, admin health endpoint (completed 2026-05-12)
- [x] **Phase 18: 9anime → Anitaku/Gogoanime** — Second provider (pivoted per 2026-05-12 research — see .planning/phases/18-9anime/18-RESEARCH.md), failover AnimePahe → Gogoanime wired + verified at integration-test + production-health-probe layer (live browser smoke deferred to HUMAN-UAT.md), 3 new embed extractors registered (vibeplayer, streamhg, earnvids)
- [x] **Phase 19: AnimeKai (gated)** — Third provider behind `SCRAPER_ANIMEKAI_ENABLED` feature flag; in-house token generator in megacloud-extractor sidecar (no `enc-dec.app`); flag default-off carryover acceptable if R&D doesn't converge (completed 2026-05-12)
- [ ] **Phase 20: Cutover** — Delete HiAnime + Consumet code paths, containers, env vars, frontend exports; gated on ≥ 7 days clean prod traffic on EnglishPlayer

### v3.1 Scraper Self-Healing (Phases 21-23) — Planning

- [x] **Phase 21: Playability Foundation** — `libs/streamprobe/` package (Probe + hardcoded ad-CDN blocklist + Redis-lift TODO), gogoanime server-priority + per-server fallback, Redis winning-server cache, `parser_unplayable_total` + `parser_ad_decoy_total` metrics, scraper `meta.gated` response field, `EnglishPlayer.vue` three-phase loader (EN + RU). Restores production playback by routing around ad-poisoned VibePlayer transparently. (completed 2026-05-13 — VERIFICATION human_needed pending browser smoke; backend smoke passed live)
- [x] **Phase 22: Provider Robustness** — Multi-URL extraction (`hls2` signed `.m3u8` + `hls3` unsigned `.txt`) in streamhg/earnvids embeds, HLS proxy allowlist additions (`managementadvisory.sbs` + `exoplanethunting.space`), ISS-011 inline incident entry. Adds per-server URL-family fallback. See `.planning/phases/22-provider-robustness/22-CONTEXT.md`. (completed 2026-05-13)
- [ ] **Phase 23: Self-Maintenance Loop** — Daily 03:00 canary cron (Frieren + One Piece + 3 dynamic from watch_history), `playability_canary_runs_total` metric, Grafana dashboard, three alert rules → existing `services/maintenance` webhook. Maintenance prompt Patterns 6/7 + Scraper Playability Regression section already in place (shipped 2026-05-13 alongside the spec). See `.planning/phases/23-self-maintenance-loop/23-CONTEXT.md`.

### Next Milestone (TBD)

After v3.1 ships, run `/gsd-new-milestone` to start the next cycle. Reserved future phases:
- Phase 24: VibePlayer Recovery via WARP egress (separate spec when there is appetite to revive VibePlayer as a working server)
- Phase 25: MinIO Hot Archival (separate v3.2 spec; rip + serve popular titles from MinIO)

## Phase Details

### Phase 15: Foundation
**Goal**: All structural seams in place so subsequent phases plug in providers without re-architecting. No user-visible behavior change. Adds a new `services/scraper/` microservice (orchestrator + interfaces + harness) plus a thin client inside catalog that talks to it.
**Depends on**: Nothing (first v3.0 phase)
**Requirements**: SCRAPER-FOUND-01, SCRAPER-FOUND-02, SCRAPER-FOUND-03, SCRAPER-FOUND-04, SCRAPER-FOUND-05, SCRAPER-FOUND-06, SCRAPER-FOUND-07, SCRAPER-FOUND-08, SCRAPER-FOUND-09, SCRAPER-FOUND-10, SCRAPER-NF-01, SCRAPER-NF-03
**Success Criteria** (what must be TRUE):
  1. `docker compose ps` shows a healthy `animeenigma-scraper` container on `:8088` (changed from planned `:8087` in Plan 15-01 — host port conflict with the `services/maintenance` binary that runs natively on 8087 outside docker-compose); `make redeploy-scraper` and `make logs-scraper` both work; the service starts with no providers registered and serves `GET /scraper/health` returning a JSON snapshot.
  2. `GET /api/anime/{animeId}/scraper/episodes|servers|stream|health` on the catalog API surface returns HTTP 503 `not-yet-implemented`. Catalog resolves the UUID → MAL ID and forwards to scraper via `services/catalog/internal/parser/scraper/client.go`; scraper returns 503 from `services/scraper/internal/handler/`. Both layers are wired.
  3. The Stream DTO compiled into `services/scraper/internal/domain/` has no `iframe_url` field at the Go type level — silent cross-tier fallback to Kodik is structurally impossible. Compile-time test asserts the field's absence.
  4. `make capture-goldens` recipe runs in `services/scraper/` and produces deterministic `testdata/<provider>/*.html` fixtures; parser unit tests run offline against goldens (no network).
  5. CI fails any `services/scraper/go.mod` PR that adds `chromedp`, `go-rod`, `chromedp-rod`, `utls`, `tls-client`, `cloudscraper_go`, or `flaresolverr` — verified by a deliberate red PR.
  6. Every upstream HTTP call routed through `BaseHTTPClient` (in scraper) has a hard 10-second timeout and uses `hashicorp/go-retryablehttp` exponential backoff (1s → 2s → 4s → 8s) — no hand-rolled retry loops permitted.
**Plans**: TBD

### Phase 16: AnimePahe + New EnglishPlayer
**Goal**: A user opens an anime in the new "English" tab and watches it end-to-end via AnimePahe. The old HiAnime and Consumet player tabs continue to exist (in a debug-only path) so users have a soak-period fallback.
**Depends on**: Phase 15
**Requirements**: SCRAPER-PAHE-01, SCRAPER-PAHE-02, SCRAPER-PAHE-03, SCRAPER-PAHE-04, SCRAPER-PAHE-05, SCRAPER-UI-01, SCRAPER-UI-02, SCRAPER-UI-03, SCRAPER-UI-04, SCRAPER-NF-02, SCRAPER-NF-05
**Success Criteria** (what must be TRUE):
  1. A logged-in user on the anime detail page sees a single "English" tab (replacing the two old "HiAnime" + "Consumet" tabs); old tabs are reachable only via `?legacy=1` dev flag.
  2. Clicking the English tab on an anime with malsync coverage plays an HLS stream sourced from AnimePahe through `EnglishPlayer.vue` — Video.js + HLS.js + `SubtitleOverlay.vue` for Jimaku JP subs all functional.
  3. The "Source: AnimePahe" dropdown inside the player UI is visible (single-option until Phase 18); user override selection persists per anime via the existing watch-preference store.
  4. Cache TTLs observed in Redis match the freshness contract: 24h malsync, 6h episodes, 15min search, ≤ `min(parsed kwik expiry − 30s, 5min)` for stream URLs.
  5. ReportButton bug reports submitted from the English tab include `provider:animepahe` plus the orchestrator's `tried:` chain — verified via a test report inspected in the player_reports volume.
**Plans**: 6 plans across 4 waves (Wave 1: 16-01 + 16-02 + 16-04 parallel; Wave 2: 16-03; Wave 3: 16-05; Wave 4: 16-06)
- [ ] 16-01-PLAN.md — Wave 1: AnimePahe connectivity probe + BaseHTTPClient.Jar() accessor + HLS allowlist regression lock + capture goldens
- [ ] 16-02-PLAN.md — Wave 1: Kwik EmbedExtractor (dop251/goja in-process, fresh runtime per call, SSRF + timeout guards)
- [ ] 16-03-PLAN.md — Wave 2: AnimePahe Provider (malsync 24h + fuzzy fallback, episodes 6h, ListServers HTML scrape via goquery, stream TTL ≤ min(expires−30s, 5min), DDoS-Guard cookie helper)
- [ ] 16-04-PLAN.md — Wave 1: Frontend infra — scraperApi client + new locale keys (3 locales) + ReportButton + diagnostics scraperProvider/triedChain props + useWatchPreferences.preferredScraperProvider
- [ ] 16-05-PLAN.md — Wave 3: Scraper boot wiring (Kwik + AnimePahe registered, Redis cache, ANIMEPAHE_BASE_URL env, meta.tried response field, 503-stubs → live orchestrator)
- [ ] 16-06-PLAN.md — Wave 4: EnglishPlayer.vue (fork of HiAnimePlayer with scraperApi + cyan accent) + Anime.vue tab integration + legacy=1 gating + Playwright e2e
**UI hint**: yes

### Phase 17: Observability
**Goal**: A dead provider stops being silently dead — health visibility per provider per stage exists before a second provider is added so it can't hide the first's degradation.
**Depends on**: Phase 16
**Requirements**: SCRAPER-OBS-01, SCRAPER-OBS-02, SCRAPER-OBS-03, SCRAPER-OBS-04, SCRAPER-OBS-05, SCRAPER-NF-04
**Success Criteria** (what must be TRUE):
  1. Prometheus exposes `provider_health_up{provider, stage}` with 5 stages (search, episodes, servers, stream, stream_segment); the liveness probe runs every 15 min ± 20% jitter against a rotating 5-10 anime golden pool.
  2. A stage flips to 0 after 3 consecutive failures within 15 min — verified by intentionally breaking the AnimePahe stage in a controlled test.
  3. When `provider_health_up{stage="stream_segment"} == 0 for 15m`, a Grafana alert fires to the existing Telegram admin chat (`TELEGRAM_ADMIN_CHAT_ID`) — verified end-to-end with a test alert.
  4. The orchestrator skips any provider whose in-memory 60-second health cache reads 0; skipped providers re-enter rotation on the next probe pass that flips them back to 1.
  5. `GET /api/admin/scraper/health` returns the per-provider/per-stage snapshot plus last-success timestamps; `parser_requests_total`, `parser_request_duration_seconds`, `parser_fallback_total{from,to}`, and `parser_zero_match_total{provider,selector}` all emit with `{provider}` labels.
**Plans**: 4 plans across 3 waves (Wave 1: 17-01 + 17-04 parallel; Wave 2: 17-02; Wave 3: 17-03 — 17-03 bumped to Wave 3 because it shares main.go edits with 17-02)
- [x] 17-01-PLAN.md — Wave 1: domain/cache foundation — provider_health_up gauge family + InMemoryHealthCache + stage constants + orchestrator skip-unhealthy wiring + parser_zero_match_total counter
- [x] 17-04-PLAN.md — Wave 1: Prometheus scrape job (the missing P-04 blocker) + Grafana scraper-health dashboard + provider-health-stream-segment-down Telegram alert + changelog entry
- [x] 17-02-PLAN.md — Wave 2: ProbeRunner (15-min ± 20% jitter, 5-stage pipeline, 3-of-15-min sliding window, defer-recover) + golden pool + main.go wiring + AnimePahe stage-key rename + first ParserZeroMatchTotal emit
- [x] 17-03-PLAN.md — Wave 3: GET /api/admin/scraper/health admin endpoint (scraper handler + transport route) + gateway proxy config/handler/router (specific-before-general /admin/scraper/* before /admin/*)

### Phase 18: 9anime → Anitaku/Gogoanime (pivoted)
**Goal**: A second alive EN provider is in rotation so a single provider failure does not blank the English tab for users.
**Depends on**: Phase 17
**Requirements**: SCRAPER-9ANI-01, SCRAPER-9ANI-02, SCRAPER-9ANI-03, SCRAPER-9ANI-04, SCRAPER-9ANI-05, SCRAPER-9ANI-06
**Success Criteria** (what must be TRUE):
  1. The "Source:" dropdown inside the English player offers AnimePahe and 9anime; user can manually switch and the choice persists per anime.
  2. With AnimePahe's health gauge forced to 0, the orchestrator transparently serves a playable HLS stream from 9anime and `parser_fallback_total{from="animepahe",to="9anime"}` increments.
  3. The embed extractors that 9anime exposes (mp4upload, streamsb, streamtape, megacloud variants — exact set discovered during impl) are each registered as named `EmbedExtractor` entries in the registry, reusable by future providers.
  4. 9anime CDN hostnames (mp4upload / streamsb / streamtape resolved hosts plus 9anime's static asset hosts) are added to `libs/videoutils/proxy.go::HLSProxyAllowedDomains` and verified by a successful stream proxy in production.
  5. 9anime episode lists surface sub/dub split where present and are cached 6 hours; ID resolution uses malsync with fuzzy fallback identical to AnimePahe's contract.
**Plans**: 4 plans across 3 waves (Wave 1: 18-01; Wave 2: 18-02 + 18-03 parallel; Wave 3: 18-04)
- [x] 18-01-PLAN.md — Wave 1: Wave-0 scaffolding (shared fuzzy/ pkg, goldens, RED test files, config + Makefile target, REQUIREMENTS/ROADMAP annotations)
- [x] 18-02-PLAN.md — Wave 2: Gogoanime provider package (client/dto/malsync/cache) — fuzzy-first FindID, sub/dub merge ListEpisodes, anime_muti_link ListServers, GetStream via registry
- [x] 18-03-PLAN.md — Wave 2: Three embed extractors (vibeplayer regex-only; streamhg + earnvids share Dean-Edwards packedExtractor base)
- [x] 18-04-PLAN.md — Wave 3: Orchestrator + extractor registration in main.go, HLS proxy allowlist append (5 hostnames), EnglishPlayer.vue multi-option dropdown activation + capitalizeProvider gogoanime branch, /animeenigma-after-update + failover-metric verification (live browser failover smoke deferred to HUMAN-UAT.md; compensating integration test PASS)
**UI hint**: yes

### Phase 19: AnimeKai (gated)
**Goal**: A third provider ships behind a feature flag so we can validate the in-house token generator against live `animekai.to` without putting users on a path that depends on `enc-dec.app`. Acceptable outcome: flag default-off and `SCRAPER-KAI-01..04` carried to v3.1 if R&D doesn't converge.
**Depends on**: Phase 18
**Requirements**: SCRAPER-KAI-01, SCRAPER-KAI-02, SCRAPER-KAI-03, SCRAPER-KAI-04, SCRAPER-KAI-05, SCRAPER-KAI-06, SCRAPER-KAI-07
**Success Criteria** (what must be TRUE):
  1. `SCRAPER_ANIMEKAI_ENABLED` env-var feature flag exists, reads at orchestrator startup, defaults to off in production, and is toggleable via `docker compose restart catalog` without a rebuild.
  2. With the flag on, AnimeKai's MegaUp embed token + AES key are generated entirely inside `docker/megacloud-extractor/` via a new `/animekai-token` endpoint — `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` returns nothing.
  3. With the flag on, the failover chain AnimePahe → 9anime → AnimeKai is verified end-to-end: forcing the first two providers' health gauges to 0 still produces a playable stream from AnimeKai (recorded by `parser_fallback_total`).
  4. With the flag off in production for ≥ 7 days, no AnimeKai traffic reaches the upstream — confirmed by `parser_requests_total{provider="animekai"}` staying flat-zero during that window.
  5. If the in-house token generator does not converge (extractor returns errors against live `animekai.to`), the phase exits with flag default-off and the four AnimeKai impl requirements (`SCRAPER-KAI-01..04`) explicitly documented as v3.1 carryover — Phase 20 cutover is not blocked.
**Plans**: 1 plan in 1 wave (ESCAPE-HATCH path per 19-RESEARCH.md §Convergence Probability Assessment — AnimeKai officially announced shutdown 2026-05-10)
- [x] 19-01-PLAN.md — Wave 1: Provider package scaffold (stub returning ErrProviderDown) + AnimeKaiConfig + conditional main.go registration + sidecar /animekai-token HTTP 501 stub + REQUIREMENTS.md v3.1 carryover annotation

### Phase 20: Cutover — PAUSED 2026-05-13

**Status note:** Paused after Plan 20-01 (pre-flight guardrail) because the EnglishPlayer is NOT actually serving clean production traffic — PoC 2026-05-13 proved VibePlayer (the default first server) returns ad-decoy m3u8s for the entire variant playlist. The 7-day soak gate is structurally unsatisfiable while EnglishPlayer can't deliver real video. v3.1 (Phases 21-23) restores playback and re-arms the soak clock; Phase 20 resumes from Plan 20-02 once v3.1 Phase 21 ships and the soak clock runs cleanly for 7 days.

**Goal**: Dead HiAnime + Consumet code paths are deleted in a single PR. The frontend has one English player surface, one backend route family, and one set of locale strings. Catalog image size drops; docker-compose ps shows neither dead container.
**Depends on**: Phase 19 + v3.1 Phase 21 (functional playback) + 7-day clean soak post-Phase-21
**Requirements**: SCRAPER-CUT-01, SCRAPER-CUT-02, SCRAPER-CUT-03, SCRAPER-CUT-04, SCRAPER-CUT-05, SCRAPER-CUT-06, SCRAPER-CUT-07
**Success Criteria** (what must be TRUE):
  1. **Hard guardrail (must be true before any deletion ships):** the new EnglishPlayer has served ≥ 7 days of clean production traffic — per-provider error rate ≤ 5%, zero Telegram alerts, zero user-reported player breakage in `docs/issues/` for that window. Soak clock starts the day v3.1 Phase 21 reaches production.
  2. After the cutover PR merges, `ls services/catalog/internal/parser/{hianime,consumet}/` returns "No such file"; `grep -E "aniwatch|consumet-api" docker/docker-compose.yml` returns nothing; `docker compose ps` after redeploy shows neither container.
  3. The catalog service starts without `ANIWATCH_API_URL` or `CONSUMET_API_URL` env vars set; `docker/megacloud-extractor/patch-aniwatch.sh` is deleted and the `megacloud-extractor` container entrypoint is plain `node server.js`.
  4. The frontend has no remaining `HiAnimePlayer.vue`, `ConsumetPlayer.vue`, `hianimeApi`, `consumetApi`, or `?legacy=1` flag — verified by `grep -r "HiAnimePlayer\|ConsumetPlayer\|hianimeApi\|consumetApi\|legacy=1" frontend/web/src/` returning nothing.
  5. Redis cache keys from the dead namespaces (`search:hianime:*`, `search:consumet:*`, `stream:hianime:*`, `stream:consumet:*`, `episodes:hianime:*`, `episodes:consumet:*`) are deleted by the one-shot script committed alongside the PR; `ru.json` / `en.json` / `ja.json` contain only the unified "English" tab label, no HiAnime/Consumet strings.
**Plans**: 5 plans across 4 waves (Wave 0: 20-01 pre-flight guardrail; Wave 1: 20-02 + 20-03 parallel; Wave 2: 20-04; Wave 3: 20-05)
- [x] 20-01-PLAN.md — Wave 0: pre-flight guardrail script (4 gates: date ≥ 2026-05-19, per-provider error_rate ≤ 5%, zero ProviderHealthStreamSegmentDown alerts in 7d, no new docs/issues player-breakage entries in 7d); exits non-zero if any gate fails
- [ ] 20-02-PLAN.md — Wave 1: backend catalog deletion — parser/hianime + parser/consumet directories, 8 handler funcs, 8 routes, HiAnimeConfig + ConsumetConfig, HiAnime + Consumet health probes, service-layer wiring, main.go args
- [ ] 20-03-PLAN.md — Wave 1: docker deletion — aniwatch + consumet service blocks in docker-compose.yml, ANIWATCH_API_URL + CONSUMET_API_URL env entries, patch-aniwatch.sh file, Makefile redeploy-% catalog cache-purge hook
- [ ] 20-04-PLAN.md — Wave 2: frontend deletion — HiAnimePlayer.vue + ConsumetPlayer.vue components, hiAnimeApi + consumetApi exports, ?legacy=1 plumbing in Anime.vue, narrow player/PlayerName type unions, localStorage migration to 'english', drop tabDebugSuffix from 3 locale files
- [ ] 20-05-PLAN.md — Wave 3: Redis purge script (SCAN+UNLINK) + run against prod + smoke tests + changelog.json entry + Telegram notification + /animeenigma-after-update final invocation

### Phase 21: Playability Foundation
**Goal**: Production English playback works again. A request that would have hit an ad-poisoned server transparently rolls forward to the next server in priority order and plays real video — verified by a playability gate that catches the poison before the URL reaches the user. Latency cost is masked by a three-phase loader. v3.1's foundation phase; unblocks the v3.0 Phase 20 soak clock by making the EnglishPlayer functional.
**Depends on**: v3.0 Phase 17 (metrics + health gauge infrastructure already shipped)
**Requirements**: SCRAPER-HEAL-01, SCRAPER-HEAL-02, SCRAPER-HEAL-03, SCRAPER-HEAL-04, SCRAPER-HEAL-05, SCRAPER-HEAL-06, SCRAPER-HEAL-07, SCRAPER-HEAL-08
**Success Criteria** (what must be TRUE):
  1. **Hard fix-the-prod gate:** Production EnglishPlayer plays real video end-to-end for Frieren ep 1 (sub) — fetched master m3u8 contains zero `*.ibyteimg.com` / `p16-ad-sg.*` segments; first segment HEAD returns 200; user-visible playback confirmed manually (browser smoke or HUMAN-UAT).
  2. New package `libs/streamprobe/` is registered in `go.work`, used by `services/scraper/internal/providers/gogoanime/`, and unit-test-covered for all 7 `Reason` enum values via synthetic m3u8 fixtures (incl. `ad_decoy`, `cdn_unreachable`, `signed_url_expired`, `zero_match`, `status_403`, `empty_response`, `playable`).
  3. `gogoanime.ListServers` sorts results per env `SCRAPER_SERVER_PRIORITY` (default `streamhg,earnvids,vibeplayer`); typo'd entries fail-fast at scraper startup with a clear error message naming the unknown server.
  4. `gogoanime.GetStream` iterates servers in priority order, runs the playability gate on each, returns first success; total in-call budget ≤ 8 s across servers. Winning server cached at Redis key `scraper:winning_server:<provider>:<anime>:<ep>` for 5 minutes; warm-path skips the gate on cache hit.
  5. Scraper `/metrics` exposes `parser_unplayable_total{provider, server, reason}` and `parser_ad_decoy_total{provider, server}` with non-zero values exercised by test (curl the endpoint after a gated fetch).
  6. `GET /scraper/stream` JSON response includes `meta.gated: true` whenever the gate ran on this call (absent / false on cache hit); a frontend integration test asserts the FE reads this field correctly.
  7. `frontend/web/src/components/player/EnglishPlayer.vue` renders three sequential loader phases (EN + RU copy) driven by `loadingServers` / `loadingStream` / `validatingStream` refs — verified by Vitest component test exercising each phase + locale.
**Plans**: 4 plans across 2 waves (Wave 1: 21-01 + 21-02 parallel; Wave 2: 21-03 + 21-04 parallel — no file overlap between 21-03 backend and 21-04 frontend)
- [x] 21-01-PLAN.md — Wave 1: libs/streamprobe package — Probe(ctx, masterURL, headers) Result + 7-Reason enum + hardcoded ad-CDN host-suffix blocklist with Redis-lift TODO + SSRF guard
- [x] 21-02-PLAN.md — Wave 1: parser_unplayable_total + parser_ad_decoy_total counters in libs/metrics + writeSuccess(..., gated bool) handler signature emitting meta.gated when true
- [x] 21-03-PLAN.md — Wave 2: SCRAPER_SERVER_PRIORITY config + ValidatePriorityList fail-fast + gogoanime.GetStreamWithGate (parallel top-2 probe, sequential 3+, ≤8s budget) + winning-server Redis cache + boot wiring + maintenance-prompt reason-enum sync test + prod smoke
- [x] 21-04-PLAN.md — Wave 2: EnglishPlayer.vue validatingStream ref + three-phase loader template (EN + RU inline copy per D6) + fetchStream meta.gated wiring + Vitest spec (9 cases) + changelog + after-update
**Status**: SHIPPED 2026-05-13 — VERIFICATION.md `human_needed`, 5/7 must-haves fully met. Backend production smoke passed (meta.gated true on cold, absent on warm, parser_unplayable_total live increment caught a real streamhg failure → earnvids took over). Manual browser smoke for the three-phase loader is the only outstanding user gate; flaky `TestGetStreamWithGate_AdDecoy_Skipped` race logged as W-21-01 follow-up (implementation correct, only test is broken).

### Phase 22: Provider Robustness
**Goal**: When a single CDN behind a server fails (signed-URL expired, 403, geo-block), the orchestrator transparently tries that server's secondary URL family before giving up on the server. Catches the failure mode "the regex still works but the URL doesn't" — distinct from Phase 21's "the server is dead".
**Depends on**: Phase 21 (gate + per-server fallback already iterates `[]Sources`)
**Requirements**: SCRAPER-HEAL-09, SCRAPER-HEAL-10, SCRAPER-HEAL-11
**Success Criteria** (what must be TRUE):
  1. `services/scraper/internal/embeds/streamhg.go` and `earnvids.go` return BOTH the `hls2` (signed `.m3u8`) AND `hls3` (unsigned `.txt`) URLs as separate `Stream.Sources` entries, verified by unit test against golden packed-JS fixtures captured 2026-05-13.
  2. `libs/videoutils/proxy.go` `HLSProxyAllowedDomains` contains `managementadvisory.sbs` and `exoplanethunting.space` (the hls3 CDN hosts); integration test fetches a synthetic hls3 m3u8 through the HLS proxy and confirms 200 OK passthrough.
  3. End-to-end synthetic: when `hls2` returns 403 (simulated via test fixture), gogoanime `GetStream` falls through to `hls3` via Phase 21's per-server iteration and returns a playable URL.
  4. `docs/issues/README.md` contains an inline `ISS-011: VibePlayer Ad-Decoy Poisoning` entry documenting the PoC 2026-05-13 findings — status `Mitigated` (not Resolved) since root cause (IP-level poisoning) persists; entry sits in Active Issues until WARP recovery (future phase) flips it.
**Plans**: 2 plans in 1 wave (parallel — file scopes do not overlap)
- [x] 22-01-PLAN.md — Wave 1: Multi-URL extraction in streamhg/earnvids (hls2 + hls3) + cold-path Sources iteration in gogoanime.coldPathGated (SCRAPER-HEAL-09)
- [x] 22-02-hls-proxy-allowlist-and-iss011-PLAN.md — Wave 1: HLS proxy allowlist (managementadvisory.sbs + exoplanethunting.space) + handler-level integration smoke + ISS-011 inline doc entry + /animeenigma-after-update (SCRAPER-HEAL-10, SCRAPER-HEAL-11)
**Status**: SHIPPED 2026-05-13 — VERIFICATION.md `passed`, 14/14 must-haves met. Production /scraper/stream returns sources_count: 2 (hls2+hls3) per server; cold-path iteration over Sources verified live (Phase 21 follow-up incidentally landed here). Live observation: hls3 host has already rotated to `strategicplanning.sbs` — exactly the failure mode Phase 23 canary is designed to catch via Pattern 7.

### Phase 23: Self-Maintenance Loop
**Goal**: A regression at any upstream site is detected within 24 hours by a daily canary that exercises real production code paths, surfaces a labeled alert into the existing `services/maintenance` bot, and gets dispatched per `.claude/maintenance-prompt.md` Patterns 6/7 — without a human needing to notice.
**Depends on**: Phase 21 (gate exists) + Phase 22 (multi-URL extraction live so the canary exercises full surface)
**Requirements**: SCRAPER-HEAL-12, SCRAPER-HEAL-13, SCRAPER-HEAL-14, SCRAPER-HEAL-15, SCRAPER-HEAL-16
**Success Criteria** (what must be TRUE):
  1. `services/scheduler/internal/jobs/scraper_playability_canary.go` runs daily at 03:00 local (±5 min jitter), composes anime list as `[Frieren, One Piece, 3 distinct anime_ids from watch_history < 24h]` with fallback to `anime_list ORDER BY updated_at DESC` when history empty, and writes per-run logs to the `player_reports` Docker volume.
  2. Scheduler `/metrics` exposes `playability_canary_runs_total{provider, server, result, reason, anime_slot}` with `anime_slot ∈ {anchor_frieren, anchor_one_piece, recent_1, recent_2, recent_3}` — verified by reading metrics after one canary run.
  3. New Grafana dashboard `infra/grafana/dashboards/scraper-provider-health.json` shows stacked pass/fail per provider/server (24h), reason breakdown, last-canary timestamp, and top failing tuples.
  4. Three Prometheus alert rules in `infra/grafana/alerts/scraper.yaml` route to existing `services/maintenance` `/api/grafana-webhook`: `ScraperPlayabilityRegression` (any canary fail in 25h, warning), `ScraperAdDecoySurge` (rate > 0 sustained 5m, warning), `ScraperUnplayableSpike` (rate / getstream-rate > 0.05 sustained 5m, critical). All labels include `provider`, `server`, `reason`.
  5. Synthetic Pattern 6 alert injected into `/api/grafana-webhook` produces a maintenance bot response that (a) names Pattern 6 in `known_pattern`, (b) tiers as `button_fix` for the server-priority reorder fix path, (c) names the correct files; synthetic Pattern 7 alert similarly dispatches to selector/regex/allowlist fix paths.
  6. `.claude/maintenance-prompt.md` Patterns 6/7 + Scraper Playability Regression section verified still present and parseable (SCRAPER-HEAL-16 — pre-shipped 2026-05-13).
**Plans**: 3 plans across 3 waves (serialized because all three plans append to docker/docker-compose.yml — different service blocks but same file. Wave 1: 23-01 canary cron + metric; Wave 2: 23-02 Grafana dashboard + provisioning; Wave 3: 23-03 alert rules + synthetic Pattern 6/7 webhook verification + maintenance-prompt symbol stability + /animeenigma-after-update)
- [x] 23-01-canary-cron-PLAN.md — Wave 1: services/scheduler/internal/jobs/scraper_playability_canary.go (cron 0 3 * * * + ±5min jitter, anchors Frieren + One Piece + 3 dynamic from watch_history with anime_list fallback) + playability_canary_runs_total{provider, server, result, reason, anime_slot} counter in libs/metrics + per-run JSON log to player_reports volume + scheduler boot wiring + manual-trigger handler (SCRAPER-HEAL-12, SCRAPER-HEAL-13)
- [x] 23-02-grafana-dashboard-PLAN.md — Wave 1: infra/grafana/dashboards/scraper-provider-health.json (4 panels: pass/fail per provider/server 24h, reason breakdown, last canary run, top failing tuples) + provisioning wiring + docker-compose mount (SCRAPER-HEAL-14)
- [ ] 23-03-alerts-and-maintenance-verify-PLAN.md — Wave 2: infra/grafana/alerts/scraper.yaml (ScraperPlayabilityRegression warning, ScraperAdDecoySurge warning, ScraperUnplayableSpike critical — all with provider/server/reason labels routing to existing /api/grafana-webhook) + synthetic Pattern 6/7 webhook tests + maintenance-prompt symbol-stability tests (cacheStream + computeStreamTTL + all 7 Reason values) + /animeenigma-after-update final step (SCRAPER-HEAL-15, SCRAPER-HEAL-16)

### Next Milestone (TBD)

After v3.1 ships, run `/gsd-new-milestone` to start the next cycle. Reserved future phases:
- Phase 24: VibePlayer Recovery via WARP egress (revives VibePlayer as a working server by routing scraper egress through Cloudflare WARP; separate spec when there is appetite)
- Phase 25: MinIO Hot Archival (rip popular HLS streams to MinIO; serve from there to decouple from upstream availability; separate v3.2 spec)

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-8 | v1.0 | 18/18 | ✅ Complete | 2026-04-27 → 2026-05-03 |
| 9-14 | v2.0 | 8/8 | ✅ Complete | 2026-05-06 → 2026-05-07 |
| 15 | v3.0 | 4/4 | Complete    | 2026-05-11 |
| 16 | v3.0 | 0/6 | Planned     | — |
| 17 | v3.0 | 4/4 | Complete    | 2026-05-12 |
| 18 | v3.0 | 4/4 | Complete    | 2026-05-12 |
| 19 | v3.0 | 1/1 | Complete    | 2026-05-12 |
| 20 | v3.0 | 1/5 | In Progress|  |
| 23 | v3.1 | 2/3 | In Progress | 2026-05-13 (23-01, 23-02 shipped) |
