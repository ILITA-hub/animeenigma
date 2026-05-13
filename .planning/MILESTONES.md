# Milestones

## v3.1 Scraper Self-Healing (Shipped: 2026-05-13)

**Phases completed:** 9 phases, 33 plans, 49 tasks

**Key accomplishments:**

- File existence:
- File existence:
- File existence:
- File existence:
- Connectivity probe proves AnimePahe is alive behind DDoS-Guard; BaseHTTPClient.Jar() accessor unblocks Plan 16-03; four upstream-shaped fixtures and a CI-enforced regression lock on the HLS allowlist now ship.
- Found during:
- First live `domain.Provider` for the v3.0 universal scraper — resolves AnimePahe anime IDs via malsync (24h cache) with Jaro-Winkler 0.85 fuzzy fallback, paginates episode listings with 6h cache, scrapes /play HTML for Kwik servers, delegates stream extraction to the Plan 16-02 KwikExtractor, and transparently handles DDoS-Guard cookies via the Plan 16-01 BaseHTTPClient.Jar() accessor.
- scraperApi client + 12 new locale keys + ReportButton/diagnostics scraperProvider+triedChain wiring + useWatchPreferences per-anime scraper preference — green floor for Plan 16-06 (EnglishPlayer.vue) with hiAnimeApi/consumetApi untouched.
- Wave 3 closing plan — converts the assembled Phase 16-02 (Kwik) + 16-03 (AnimePahe) pieces into a live end-to-end pipeline.
- Unified English-source player (`EnglishPlayer.vue`) live behind the scraperApi orchestrator with cyan accent + single-option Source dropdown; Anime.vue mounts it as the default English-language tab and gates HiAnime + Consumet legacy tabs behind `?legacy=1`; Playwright e2e covers tab visibility, legacy flag, and ReportButton meta.tried thread-through.
- Prometheus gauge family `provider_health_up{provider, stage}` + 60s fail-open in-memory cache + orchestrator runFailover skip-unhealthy branch wired through with nil-cache backcompat for Phase 16.
- One-liner:
- 1. [Rule 3 - Blocking] Updated `services/scraper/internal/transport/router_test.go`
- Prometheus scrape job for scraper:8088, scraper-health Grafana dashboard with 5 stage stat tiles + heartbeat + fallback panels, and a stream_segment-down Telegram alert — the deploy-side scaffolding for Phase 17 ahead of any Go code emitting the new gauges.
- Shared fuzzy/ package extracted + 8 anitaku.to goldens captured atomically + GogoanimeConfig env var + 20 RED-state test scaffolds, all on a clean break from the dead 9anime brand.
- Gogoanime/Anitaku scraper provider implementing domain.Provider end-to-end against the Plan 18-01 anitaku.to goldens — fuzzy /search.html ID resolution (primary path), sub/dub category merge with 6h cache, Cloudflare-Turnstile skip-list at ListServers, and registry-dispatched GetStream with &e=<delta> + &s=<unix> stream TTL.
- Three new EmbedExtractor implementations (vibeplayer regex-only, streamhg + earnvids Dean-Edwards packers) backed by a shared packedExtractor base in packed_common.go, with the goja-runtime helper lifted from kwik.go's method to a package-level function so kwik + packed both route through one watchdog implementation.
- Gogoanime/Anitaku wired end-to-end into the running scraper service + EnglishPlayer source dropdown: 3 extractors registered, provider failover-positioned after animepahe, 5 CDN hostnames appended to HLS allowlist, multi-option source dropdown activated with ARIA semantics + rollback-on-fail UX, production deploy verified healthy across all 5 probe stages.
- AnimeKai shipped as a wired-but-disabled scraper provider behind `SCRAPER_ANIMEKAI_ENABLED=false`: every Provider method returns wrapped `domain.ErrProviderDown`, the sidecar `POST /animekai-token` returns HTTP 501, and the orchestrator continues to serve from AnimePahe → Gogoanime without users seeing the third option. SCRAPER-KAI-01..04 + KAI-07 carried to v3.1 with a body-only fill-in surface.
- 4-gate hard guardrail Bash script that blocks Phase 20 deletion until EnglishPlayer has served >= 7 days of clean production traffic (earliest legitimate ship: 2026-05-19, since EnglishPlayer first shipped 2026-05-12 via commit 9e9d9a2).
- 1. [Rule 1 — Bug] UTF-8 BOM literal in source caused `vet: illegal byte order mark` compile error
- TestOrchestrator_AnimePaheToGogoanimeFailover
- Found during:
- streamhg/earnvids extractors now return BOTH hls2 (signed m3u8) AND hls3 (.txt) URLs as separate Stream.Sources entries, and gogoanime.coldPathGated.attemptOne iterates all Sources via the streamprobe gate before declaring a server failed.
- Closed the architectural loop on Phase 22's multi-URL fallback by adding the two hls3 CDN eTLD+1 hosts to HLSProxyAllowedDomains, locked them with a regression + behavior test pair, proved end-to-end survival via a handler-level integration smoke, and documented ISS-011 (VibePlayer ad-decoy poisoning) inline as the v3.1 motivating incident — status Mitigated pending WARP egress.
- SHIPPED
- SHIPPED
- SHIPPED

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
