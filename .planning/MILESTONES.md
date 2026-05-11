# Milestones

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
