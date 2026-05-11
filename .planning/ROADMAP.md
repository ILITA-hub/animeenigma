# Roadmap: AnimeEnigma

## Milestones

- ✅ **v1.0 Smart Watch Picker Overhaul** — Phases 1-8 (shipped 2026-05-03) — see `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v2.0 Recommendations Engine** — Phases 9-14 (shipped 2026-05-07) — see `.planning/milestones/v2.0-ROADMAP.md`
- 🟢 **v3.0 Universal Anime Scraper** — Phases 15-20 (planning) — see below

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
- [ ] **Phase 16: AnimePahe + New EnglishPlayer** — First live provider (Kwik via goja), new unified `EnglishPlayer.vue` replacing both HiAnime + Consumet tabs end-to-end
- [ ] **Phase 17: Observability** — Per-provider/per-stage health gauges, 15-min liveness probe with golden anime pool, orchestrator skips unhealthy, Grafana alert, admin health endpoint
- [ ] **Phase 18: 9anime** — Second provider (WordPress/Madara markup), failover AnimePahe → 9anime verified end-to-end, new embed extractors registered
- [ ] **Phase 19: AnimeKai (gated)** — Third provider behind `SCRAPER_ANIMEKAI_ENABLED` feature flag; in-house token generator in megacloud-extractor sidecar (no `enc-dec.app`); flag default-off carryover acceptable if R&D doesn't converge
- [ ] **Phase 20: Cutover** — Delete HiAnime + Consumet code paths, containers, env vars, frontend exports; gated on ≥ 7 days clean prod traffic on EnglishPlayer

### Next Milestone (TBD)

After v3.0 ships, run `/gsd-new-milestone` to start the next cycle.

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
**Plans**: TBD

### Phase 18: 9anime
**Goal**: A second alive EN provider is in rotation so a single provider failure does not blank the English tab for users.
**Depends on**: Phase 17
**Requirements**: SCRAPER-9ANI-01, SCRAPER-9ANI-02, SCRAPER-9ANI-03, SCRAPER-9ANI-04, SCRAPER-9ANI-05, SCRAPER-9ANI-06
**Success Criteria** (what must be TRUE):
  1. The "Source:" dropdown inside the English player offers AnimePahe and 9anime; user can manually switch and the choice persists per anime.
  2. With AnimePahe's health gauge forced to 0, the orchestrator transparently serves a playable HLS stream from 9anime and `parser_fallback_total{from="animepahe",to="9anime"}` increments.
  3. The embed extractors that 9anime exposes (mp4upload, streamsb, streamtape, megacloud variants — exact set discovered during impl) are each registered as named `EmbedExtractor` entries in the registry, reusable by future providers.
  4. 9anime CDN hostnames (mp4upload / streamsb / streamtape resolved hosts plus 9anime's static asset hosts) are added to `libs/videoutils/proxy.go::HLSProxyAllowedDomains` and verified by a successful stream proxy in production.
  5. 9anime episode lists surface sub/dub split where present and are cached 6 hours; ID resolution uses malsync with fuzzy fallback identical to AnimePahe's contract.
**Plans**: TBD
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
**Plans**: TBD

### Phase 20: Cutover
**Goal**: Dead HiAnime + Consumet code paths are deleted in a single PR. The frontend has one English player surface, one backend route family, and one set of locale strings. Catalog image size drops; docker-compose ps shows neither dead container.
**Depends on**: Phase 19 (or Phase 18 if AnimeKai is shipped flag-default-off)
**Requirements**: SCRAPER-CUT-01, SCRAPER-CUT-02, SCRAPER-CUT-03, SCRAPER-CUT-04, SCRAPER-CUT-05, SCRAPER-CUT-06, SCRAPER-CUT-07
**Success Criteria** (what must be TRUE):
  1. **Hard guardrail (must be true before any deletion ships):** the new EnglishPlayer has served ≥ 7 days of clean production traffic — per-provider error rate ≤ 5%, zero Telegram alerts, zero user-reported player breakage in `docs/issues/` for that window.
  2. After the cutover PR merges, `ls services/catalog/internal/parser/{hianime,consumet}/` returns "No such file"; `grep -E "aniwatch|consumet-api" docker/docker-compose.yml` returns nothing; `docker compose ps` after redeploy shows neither container.
  3. The catalog service starts without `ANIWATCH_API_URL` or `CONSUMET_API_URL` env vars set; `docker/megacloud-extractor/patch-aniwatch.sh` is deleted and the `megacloud-extractor` container entrypoint is plain `node server.js`.
  4. The frontend has no remaining `HiAnimePlayer.vue`, `ConsumetPlayer.vue`, `hianimeApi`, `consumetApi`, or `?legacy=1` flag — verified by `grep -r "HiAnimePlayer\|ConsumetPlayer\|hianimeApi\|consumetApi\|legacy=1" frontend/web/src/` returning nothing.
  5. Redis cache keys from the dead namespaces (`search:hianime:*`, `search:consumet:*`, `stream:hianime:*`, `stream:consumet:*`, `episodes:hianime:*`, `episodes:consumet:*`) are deleted by the one-shot script committed alongside the PR; `ru.json` / `en.json` / `ja.json` contain only the unified "English" tab label, no HiAnime/Consumet strings.
**Plans**: TBD

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 1-8 | v1.0 | 18/18 | ✅ Complete | 2026-04-27 → 2026-05-03 |
| 9-14 | v2.0 | 8/8 | ✅ Complete | 2026-05-06 → 2026-05-07 |
| 15 | v3.0 | 4/4 | Complete    | 2026-05-11 |
| 16 | v3.0 | 0/6 | Planned     | — |
| 17 | v3.0 | 0/? | Not started | — |
| 18 | v3.0 | 0/? | Not started | — |
| 19 | v3.0 | 0/? | Not started | — |
| 20 | v3.0 | 0/? | Not started | — |
