# Milestones

## v3.1 Scraper Self-Healing — REOPENED 2026-05-19

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
