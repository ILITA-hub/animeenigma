# Milestones

## v4.0 Activity Register (ClickHouse unified event plane) (Shipped: 2026-06-08)

**Phases completed:** 6 phases, 23 plans, 31 tasks

**Key accomplishments:**

- Stood up a live, host-bound ClickHouse 26.3.12.3 instance with native Prometheus self-metrics on :9363 and a clickhouse-backup 2.7.0 sidecar whose backup/restore procedure was dry-run-verified end-to-end — satisfying AR-STORE-01 as the storage foundation for the v4 Activity Register.
- 1. [Rule 3 - Blocking] Pinned older CH dep versions to avoid a workspace-wide go directive bump
- The live analytics clickstream now dual-writes to both Postgres and ClickHouse (`ANALYTICS_STORE_BACKEND=dualwrite`, PG source of truth + reversible), a new `aenigma-clickhouse` Grafana datasource is provisioned, and all 6 product-analytics panels render from ClickHouse `events_resolved` — proven by a live smoke event reaching both stores (AR-STORE-04).
- The BE effect-ingestion path that did not exist before: domain.Event now carries effect dimensions+measures, InsertBatch populates the (previously zeroed) ClickHouse effect columns, POST /internal/effects ingests effect batches into the existing batcher, and libs/tracing gained a recording RoundTripper + non-blocking producer + PII-safe baggage/ctx attribution helpers.
- The four previously-uninstrumented outbound clients (Kodik extractor, OpenSubtitles, idmapping ARM/AniList, and the scraper BaseHTTPClient across all 7 providers) now route through the recording transport — the leaf modules via injected transports that keep them zero-dependency at go 1.22, and the scraper path additionally carrying a private-ctx stream-provider tag the recorder reads for `target = provider + host`.
- HLS-proxy egress now aggregates to ONE effect row per (stream-session, host) instead of one row per ~6s segment: a per-manifest crypto/rand `?sess=` token (injected in `rewriteHLSURL`) correlates a watch's segment GETs into a bounded, idle-reaped in-memory tally that emits a single summed `tracing.Effect` on session end — and the proxy now counts both `bytes_in` (upstream `resp.Body` via a no-buffer `countReader`) and `bytes_out` (client sink).
- The PII boundary is now hardened and proven (user_id never rides outbound wire baggage — only origin/operation do, end-to-end), the egress Producer + SeedMiddleware are wired into catalog/scraper (streaming/analytics were already wired), and the full BE egress recorder is verified LIVE in ClickHouse: real per-client scraper egress rows, ONE aggregated HLS row per (session,host), and ClickHouse `count(user_id != '') = 0`.
- Task 1 (TDD) — `attribution.go` + tests:
- Task 1 (TDD) — `readgate.go` + tests:
- Task 1 (TDD) — `keyclass.go` + tests:
- Task 1 — analytics compute + publish (`cf856046`):
- Task 1 (`2cbae12a`) — GlobalSink getter + DB-effect callbacks + ReadGate + ThresholdRefresher across 7 GORM services:
- 1. [Rule 3 - Blocking] Missing context file `04-PATTERNS.md`
- 1. [Rule 1 — Verification hygiene] Reworded `rum.ts` comments to avoid literal byte-field tokens
- Task 1 — automated pre-gate: COMPLETE.
- One-liner:
- Authored the two table-driven Activity Register reports as pure declarative Grafana JSON over the already-provisioned aenigma-clickhouse datasource: a wide-event PIVOT dashboard whose $group_by template var regroups the table by ANY dimension live, and a from->choke-point->effects FLOW report rendering origin->operation->per-target requests + bytes.
- Task 1 — automated reload + dry-run: COMPLETE.
- 1. [Rule 3 - Blocking] spanmetrics connector duplicate-dimension config error
- 1. [Rule 3 - Blocking] Provisioned read-only Tempo+Loki datasources not pruned by block removal

---

## v3.1 Scraper Self-Healing — CLOSED 2026-06-04

**Status:** Shipped & closed. Original ship 2026-05-13 (Phases 21-23, tagged `v3.1`); reopened 2026-05-19 for Phases 24-28; all reopened phases shipped (SUMMARYs on disk). Closed 2026-06-04 to start root **v4 (Activity Register)**.

**Reopened phases delivered (24-28):**

- **Phase 24 — EN Reconnect:** restored `EnglishPlayer.vue` + EN tab + language/provider type unions + i18n; per-provider end-to-end verification gate.
- **Phase 25 — Audit Findings Resolution:** closed BLK-INT-01 (hls3 host auto-discovery) + W-INT-01..03.
- **Phase 26 — Provider Expansion:** AllAnime lifted into scraper providers; AnimeKai revival; 2026 source sweep.
- **Phase 27 — AnimePahe Revival:** stealth-Chromium sidecar (DDoS-Guard solve) restoring `animepahe.pw`.
- **Phase 28 — Provider Expansion R2:** AnimeFever + Miruro + 9anime added to the failover chain.

**Also shipped outside the numbered roadmap:** `18anime` adult-provider group ported into the scraper microservice as a separate 18+ provider group (merge `e2172c31`).

**Carryover deferred to backlog / future v3.x:** MinIO hot archival; VibePlayer recovery via WARP egress.

**Tag note:** the original `v3.1` git tag (2026-05-13 ship point) is left intact; the reopened delta is recorded here rather than re-tagged (per operator: "close it and go v4").

> Full reopening chronology preserved in the historical detail below + `.planning/milestones/v3.1-REOPENING.md`.

---

## v3.0 Universal Anime Scraper (Shipped: 2026-05-18) — backfilled record

**Phases 15-20.** Provider-failover scraper microservice (`services/scraper/`) replacing the removed HiAnime + Consumet: provider interface + orchestrator + EmbedExtractor registry + BaseHTTPClient (15); AnimePahe/Kwik + unified `EnglishPlayer.vue` (16); per-provider health observability + 15-min canary (17); Gogoanime/Anitaku second provider (18); gated AnimeKai (19); HiAnime/Consumet cutover (20 — over-rotated, repaired in v3.1 Phase 24). Was never formally archived/tagged at the time; recorded here for history completeness.

---

## v3.1 Scraper Self-Healing — reopening detail (2026-05-19, historical)

**Original ship:** 2026-05-13 — Phases 21, 22, 23 (16 SCRAPER-HEAL requirements, 9 plans across 3 phases). Audit verdict that day: `gaps_found (with strong implementation foundation)` — 1 production blocker (BLK-INT-01: rotated hls3 hosts not in HLS proxy allowlist → silent HTTP 200 / Content-Length 0) and 3 warnings (W-INT-01..03) all logged but not resolved.

**Reopened:** 2026-05-19 — three triggers:

1. **Regression (2026-05-18):** Phase 20 cutover (v3.0) over-rotated. The targeted HiAnime + Consumet deletions swept up EnglishPlayer.vue, the EN tab markup in Anime.vue, the multi-source dropdown infrastructure, and the i18n keys that v3.1 Phase 21's SCRAPER-HEAL-08 delivered. Production EN playback now blocked at the UI layer (no tab to click) — orthogonal to the existing BLK-INT-01 backend block.
2. **Original audit gaps still open:** BLK-INT-01 + W-INT-01..03 from the 2026-05-13 audit were never closed. The audit predicted the canary would catch BLK-INT-01 via Pattern 7, but the manual smoke test for that pipeline (Task 4 of Plan 23-03) was deferred and never run.
3. **New scope from operator:** add provider expansion — lift AllAnime catalog parser into a third in-rotation scraper provider, run a fresh 2026 EN-source survival sweep, and pick up the v3.0 Phase 19 AnimeKai escape-hatch carryover.

**New phases (24-26) scoped:**

| Phase | Name | Requirements | Purpose |
|---|---|---|---|
| 24 | EN Reconnect | SCRAPER-HEAL-17..20 | Restore EnglishPlayer.vue + EN tab + type unions + i18n keys; gate on "test each provider" end-to-end verification |
| 25 | Audit Findings Resolution | SCRAPER-HEAL-21..24 | Auto-discover hls3 hosts (BLK-INT-01), fix probe-race test (W-INT-01), sync maintenance prompt cacheStream symbol (W-INT-02), fix silent-200 in streaming handler (W-INT-03) |
| 26 | Provider Expansion | SCRAPER-HEAL-25..28 | Lift `services/catalog/internal/parser/allanime/` into `services/scraper/internal/providers/allanime/`, fresh 2026 candidate research, resurrect AnimeKai with in-house MegaUp token generator (carried from v3.0 Phase 19) |

**Phase 21-23 deliverables that DID ship and DID NOT regress** (unchanged, no rework needed):

- `libs/streamprobe/` package + 7-Reason classification + hardcoded ad-CDN blocklist
- gogoanime server priority + per-server fallback + Redis winning-server cache
- `parser_unplayable_total` + `parser_ad_decoy_total` metrics
- `meta.gated` response field in `/scraper/stream`
- Multi-URL extraction in streamhg + earnvids (hls2 + hls3)
- HLS proxy allowlist additions (managementadvisory.sbs, exoplanethunting.space — but see Phase 25 BLK-INT-01 for the rotation problem)
- Daily 03:00 canary cron + 5-label metric
- Grafana dashboard `scraper-provider-health.json` + 3 alert rules (ScraperPlayabilityRegression / ScraperAdDecoySurge / ScraperUnplayableSpike)
- Maintenance-prompt Patterns 6/7 + Scraper Playability Regression section

**Phase 21 deliverable that DID ship and DID regress** (Phase 24 restores):

- `frontend/web/src/components/player/EnglishPlayer.vue` three-phase loader (SCRAPER-HEAL-08) — file deleted 2026-05-18 in commit fe3b487 alongside HiAnime/Consumet. Recoverable from git history (`git show 8424e99:frontend/web/src/components/player/EnglishPlayer.vue` = 1973 lines, last good state).

**Reopening trace:** see `.planning/milestones/v3.1-REOPENING.md` for the full chronology + commit references + decision rationale.

**Original 2026-05-13 ship summary** (preserved for history): 16 SCRAPER-HEAL requirements implemented across 9 plans, production smoke passed on backend (meta.gated true on cold, absent on warm; parser_unplayable_total live increment caught a real streamhg failure → earnvids took over). Multi-URL extraction (hls2+hls3) wired end-to-end. Daily canary running with 5-label metric to Grafana → maintenance-webhook on :8087. See `v3.1-MILESTONE-AUDIT.md` for the audit narrative.

---

## v2.0 Recommendations Engine (Shipped: 2026-05-07)

**Phases completed:** 6 phases (9-14), 8 plans, 23/23 requirements satisfied.

**Key accomplishments:**

- Phase 9 (Foundation): Pluggable `SignalModule` interface, weighted-ensemble aggregator, per-pool min-max normalizer, and auto-migrated persistence tables (`rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence`).
- Phase 10 (Population Signals + Trending row): S3 (30-day watch-start count) and S4 (currently-airing / aired-within-90-days) cron-precomputed; anonymous "Trending now" row live on home page.
- Phase 11 (User Signals + "Up Next for you"): S1 score-cluster k-NN and S2 item-item metadata; 20-item personalized row for logged-in users with Redis 6-h top-N cache.
- Phase 12 (S5 TF-IDF Attribute Affinity): Six-dimensional time-weighted TF-IDF over tags / studios / genres / demographic / source / type / producers; integer-episode fallback for Kodik rows; AniList tags backfill pipeline.
- Phase 13 (S6 Combo-Watched-After Pin): "Because you finished X" pin appears within seconds of any score-≥7 completion; cascade local co-occurrence → Shikimori `/similar` → score-≥5 fallback; production p95 = 48ms full-stack.
- Phase 14 (Admin Debug + Eval Pipeline): `/admin/recs/:user_id` page with per-signal contribution table, S5 TF-IDF term breakdown, S6 pin_source, S11 filter audit; force-recompute endpoint (p95 ~10ms); `rec_click` + `rec_watched` events feeding `rec_signal_ctr` Prometheus metric and the new "Rec engine" Grafana dashboard.

**One-liner:** Recommendations are pluggable, transparent, and personalized — every signal's contribution is admin-auditable, every event is measurable, and v2.1 weight tuning has the data it needs.

**Archive:** `.planning/milestones/v2.0-ROADMAP.md`, `.planning/milestones/v2.0-REQUIREMENTS.md`, `.planning/milestones/v2.0-MILESTONE-AUDIT.md`.

---
