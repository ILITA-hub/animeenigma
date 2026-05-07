# Phase 12: TF-IDF Attribute Affinity (S5) - Context

**Gathered:** 2026-05-06
**Status:** Ready for planning
**Mode:** Auto-generated with locked decisions from design spec §3.1 + Phase 11 hand-off (autonomous mode)

<domain>
## Phase Boundary

Land the heaviest single signal — S5 TF-IDF time-weighted attribute affinity across the seven attribute dimensions named in design spec §3.1 (tags 0.30, studios 0.20, genres 0.15, demographic 0.10, source 0.10, type 0.10, producers 0.05) with the Kodik episode-count fallback. **Inventory and (where needed) backfill missing `animes` columns identified by the Phase 11 schema audit** — only `genres` exists today; the other six dimensions need new schema + Shikimori parser changes + a one-time backfill before S5 can score across them. After this phase ships, the existing logged-in "Up Next for you" row (Phase 11) is unchanged in shape but visibly different in ordering — top-3 anime for `ui_audit_bot` shift relative to Phase 11 baseline because S5 now contributes 0.20 of the final score.

In scope:
- **Schema additions on `animes` table:**
  - `kind` (string, indexed) — TV / Movie / OVA / ONA / Special / Music. Already queried by Shikimori GraphQL via `kind` field; just not stored. **Maps to S5 attribute "type"** (spec §3.1 weight 0.10).
  - `rating` (string) — G / PG / PG-13 / R / R+ / Rx. **Maps to S5 attribute "demographic"** (spec §3.1 weight 0.10) — Shikimori `rating` is the closest proxy for the demographic dimension; it's per-anime and aligns with audience targeting.
  - `material_source` (string) — manga / novel / light_novel / original / visual_novel / game / etc. From Shikimori `source` field. **Maps to S5 attribute "source"** (spec §3.1 weight 0.10). Column name avoids collision with existing `SourceType` (which is for video sources, not adaptation source).
- **New m2m tables:**
  - `anime_studios` join table + `studios` table (id, name). Shikimori provides `studios { id name }`. **Maps to BOTH S5 "studios" AND "producers"** (Shikimori does not separate them; the spec's separate weights for studios 0.20 + producers 0.05 collapse to a single 0.25 weight on the studios attribute. **Decision §A2** documents this collapse — see Decisions.)
  - `anime_tags` join table + `tags` table (id, name). **Tags fetched from AniList** (we already have AniList IDs via ARM mapping per `libs/idmapping/`) — Shikimori does NOT expose tags. AniList's `media.tags { name rank }` provides ~30-100 tags per anime with rank scores. **Maps to S5 "tags"** (spec §3.1 weight 0.30).
- **Shikimori parser updates** (`services/catalog/internal/parser/shikimori/client.go`):
  - Add `studios { id name }`, `kind`, `rating`, `source` to the GraphQL query.
  - Hydrate the new fields onto the `domain.Anime` struct on every fetch (search + by-ID).
- **AniList parser** (NEW, `services/catalog/internal/parser/anilist/client.go`):
  - Thin GraphQL client that hits AniList's public API (no auth required for read-only queries).
  - Single method: `FetchTags(ctx, anilistID) ([]Tag, error)` returning `[{Name, Rank}]` for an anime by AniList ID.
  - Rate-limited like the Shikimori client (AniList limits to 90 req/min unauthenticated).
- **Backfill script:** `services/maintenance/cmd/backfill-attributes/main.go` (or similar) — one-time job that walks every `animes` row, queries Shikimori for studios/kind/rating/source if absent, and queries AniList (via ARM-resolved AniList ID) for tags. Idempotent — skips rows already populated. Logs progress. Bounded by the rate limits.
- **S5 SignalModule:** `services/player/internal/service/recs/signals/s5_attribute.go` implementing the spec §3.1 TF-IDF formula:
  - `unit(u, anime) = max(duration_watched / 60, 1) minutes` if `player ∈ {hianime, consumet, animelib}` else `episode_count(u, anime)` (Kodik fallback per design §3.1).
  - `tf(u, attr) = Σ unit(u, anime) for anime in user.history if attr ∈ anime.attrs / total_units(u)`
  - `idf(attr) = log(total_users / (1 + users_with_any_history_in[attr]))`
  - `affinity(u, attr) = tf(u, attr) × idf(attr)`
  - `S5_raw(u, candidate) = 0.30 × Σ affinity over candidate.tags + 0.25 × Σ affinity over candidate.studios + 0.15 × Σ affinity over candidate.genres + 0.10 × affinity(rating) + 0.10 × affinity(material_source) + 0.10 × affinity(kind)` (note collapsed studios+producers weight per Decision §A2).
- **Persistence:** `S5_raw` per candidate is computed at request-time from the user's pre-aggregated affinity vector stored in `rec_user_signals.s5_affinity` (JSONB; key = `{dim}:{attr_id}`, value = `affinity_score`). The user-cron `Orchestrator.RunForUser` writes this vector after every per-user precompute.
- **Ensemble integration:** Register S5 in the handler's logged-in ensemble at weight 0.20 (spec §13). The ensemble registry edit is a one-line addition next to the existing S1/S2/S3/S4 entries.
- **Anonymous flow unchanged.** S5 is a per-user signal; anonymous calls do NOT compute S5.

Out of scope (later phases):
- S6 combo-watched-after pin → Phase 13 (synchronous seed update on `MarkEpisodeWatched`)
- Admin debug page + force-recompute endpoint → Phase 14
- Per-signal CTR Prometheus metrics + `rec_click` / `rec_watched` events → Phase 14
- Tuning weights based on production CTR data → v2.1 (after Phase 14 telemetry exists)
- Tags from MAL → not needed (AniList covers the same surface and is rate-limit-friendlier)
- Per-user adult-content filter → v2.1+ (out of scope until per-user setting exists)
- Producers as a separate dimension → v3.0 if Shikimori (or another source) ever exposes it cleanly; for now the studios dimension absorbs both.

</domain>

<decisions>
## Implementation Decisions

### Schema (Decision §A1) — Six new dimensions on `animes`

- **Single-value columns** (added to `animes` table directly via GORM struct tags + AutoMigrate):
  - `Kind string` — TV / Movie / OVA / ONA / Special / Music. Indexed for filtering.
  - `Rating string` — Shikimori rating enum (G / PG / PG-13 / R / R+ / Rx). Used as the S5 "demographic" attribute.
  - `MaterialSource string` — Shikimori source enum (manga / novel / light_novel / original / visual_novel / game / web_manga / etc.). Column named `material_source` to avoid collision with the existing `VideoSource.SourceType` field.
- **Multi-value m2m relationships** (added as new tables via GORM AutoMigrate):
  - `Studio { ID string PK; Name string; CreatedAt }` + `anime_studios` join (anime_id, studio_id, PK composite). Mirrors the existing `Genre` + `anime_genres` pattern in `services/catalog/internal/domain/anime.go`.
  - `Tag { ID string PK; Name string; Source string }` + `anime_tags` join (anime_id, tag_id, rank int — preserves AniList's per-anime tag rank). `Source` field is `'anilist'` for now, leaves room for `'mal'` or `'shikimori-keyword'` later.
- **Non-decisions:**
  - No separate `Producer` table or m2m. Studios absorbs producers (Decision §A2).
  - No `Demographic` table. Shikimori `rating` is the proxy (Decision §A3).

### Studios + Producers Collapse (Decision §A2)

- Spec §3.1 lists studios 0.20 and producers 0.05 as separate dimensions but Shikimori does NOT separate them — `studios { id name }` returns the production company list with no role distinction.
- **Resolution:** S5 ships with one studios dimension at weight 0.25 (= 0.20 + 0.05). Producers are not separately tracked.
- **Why this is acceptable:** spec §3.1 weights are themselves "v1, will tune via Phase 14 admin breakdown CTR data". The 0.05 producer weight was always small; folding it into studios changes ranking by < 5% in the worst case.
- **Ensemble math:** the seven dimensions in spec §3.1 reduce to six in code (collapsed), but the **sum still equals 1.0** (0.30 + 0.25 + 0.15 + 0.10 + 0.10 + 0.10 = 1.00). S5 final still feeds the ensemble at 0.20 — the per-attribute redistribution is internal to S5.
- **Documented in code:** the S5 module's per-attribute weight constants block carries a `// Decision §A2` comment explaining the collapse.

### Demographic Source (Decision §A3)

- Spec §3.1 names "demographic" as a dimension (shounen / seinen / shoujo / etc.). Shikimori does NOT have a demographic field, but `rating` (G / PG / PG-13 / R / R+ / Rx) is a usable proxy — it captures audience targeting at a coarser granularity.
- **Resolution:** S5's "demographic" dimension reads from `animes.rating`. The S5 module exposes this as the `demographic` attribute key in the affinity vector for admin debug page consistency, even though the underlying column is named `rating`.
- **Future:** if `demographic` becomes a real Shikimori field (or if we add a derivation rule from genres → demographic, e.g., shounen-tagged genres → "shounen"), Phase 14+ can swap the data source without rewriting S5.

### Tags from AniList (Decision §A4)

- Shikimori does not expose tags. AniList does (`media.tags { name rank category isAdult isGeneralSpoiler }`).
- **Resolution:** add a thin `services/catalog/internal/parser/anilist/client.go` GraphQL client (no auth required for public read). Single method `FetchTags(ctx, anilistID) ([]Tag, error)`. Rate-limited at 90 req/min (AniList's default unauth limit). Cached via `libs/cache` for 24h per anime.
- **Tag rank handling:** AniList's `rank` field (0-100) measures how strongly a tag applies to an anime. **Decision: S5 ignores rank for the v1 implementation** — every tag the anime has counts equally for TF-IDF. Rank-weighted TF-IDF is a v2.1 refinement once we have CTR data to tune against.
- **Adult/spoiler flags:** `isAdult` tags inherit from the existing `animes.hidden` flag (out of scope for tags filter); `isGeneralSpoiler` tags are kept (S5 doesn't show tags to users, only uses them for similarity).
- **Backfill:** the new backfill job uses the existing `libs/idmapping` ARM client to resolve Shikimori → AniList IDs, then calls `anilist.FetchTags`. Anime without an AniList mapping (via ARM) get an empty tags set and S5 simply contributes zero on the tags dimension for those rows — consistent with the "missing attribute → zero contribution" cold-start contract from spec §3.3.

### S5 Implementation Layout

- **File:** `services/player/internal/service/recs/signals/s5_attribute.go` + `s5_attribute_test.go`.
- **Package contract:** implements `recs.SignalModule`. ID returns `recs.SignalID("s5")`. `Precompute(ctx, userID)` re-aggregates the user's affinity vector from their `watch_history` × `animes` joins and writes to `rec_user_signals.s5_affinity` JSONB. `Score(ctx, userID, candidates)` reads the affinity vector once, then computes per-candidate raw S5 score by summing affinity for each attribute the candidate has.
- **TF-IDF caching:** the IDF term `log(total_users / (1 + users_with_any_history_in[attr]))` is global (population-scope, not per-user). Compute once per cron tick across all attributes. Stored alongside the affinity vector — or in a small `rec_idf_cache` Redis hash with 6h TTL. **Plan-phase decides** which storage shape; both are sub-millisecond at current scale.
- **Kodik fallback (spec §3.1):** if `watch_history.player == 'kodik'` then `unit = episode_count = 1` per row (Kodik writes one watch_history row per episode). For HiAnime / Consumet / AnimeLib: `unit = max(duration_watched / 60, 1)` minutes. The unit is the time-weight applied when computing `tf`.
- **Cold-start:** user with zero watch_history rows gets an empty `s5_affinity` vector. `Score` returns an empty map (not nil, not NaN). Ensemble normalizer treats absent entries as zero — same contract as S1/S2 (REC-SIG-03/04).

### Ensemble Registration

- In `services/player/internal/handler/recs.go`, the logged-in branch's ensemble registry adds one entry next to existing S1-S4:
  ```go
  ensemble := recs.NewEnsemble([]recs.WeightedSignal{
      {Module: h.s1, Weight: 0.30},
      {Module: h.s2, Weight: 0.20},
      {Module: h.s3, Weight: 0.20},
      {Module: h.s4, Weight: 0.10},
      {Module: h.s5, Weight: 0.20},  // NEW
  })
  ```
- The handler struct gets a new `s5 *signals.S5Attribute` field, wired via `NewRecsHandler(...)` constructor (similar to existing S1-S4 wiring from Phase 11).
- The user orchestrator (`services/player/internal/service/recs/user_orchestrator.go` from Phase 11) gets S5 added to its module list — one line.

### Backfill Strategy

- **One-time job, idempotent.** Lives in `services/maintenance/cmd/backfill-attributes/main.go` — a new tool that runs once on demand (not a cron). Wired into `Makefile` as a `make backfill-attributes` target.
- **Rate-limit-aware:** Shikimori at the existing rate limit; AniList at 90 req/min.
- **Progress reporting:** logs every 100 anime processed, plus a final summary (rows updated, rows skipped, rows failed-to-fetch).
- **Failure handling:** each anime is independent; a fetch failure logs the error and moves on. The job can be re-run; previously-populated rows are skipped via a `WHERE` clause on the new columns being NULL/empty.
- **Production deploy plan:** run the backfill BEFORE S5 is registered in the ensemble (Task 9 verification step), so the ensemble doesn't compute S5 against an empty schema.

### Locked from spec §13 (do not relitigate)

- S5 weight in the ensemble: **0.20**
- Per-attribute weights (with Decision §A2 collapse): tags 0.30, studios 0.25 (was 0.20 + producers 0.05), genres 0.15, demographic 0.10, source 0.10, type 0.10. Sum = 1.00.
- Kodik fallback math (integer episode count instead of duration)
- TF-IDF formula per spec §3.1
- 6-hour user-signal cron cadence (Phase 11's `UserOrchestrator` already runs this)
- Top-50 server / top-20 frontend slice (already wired in Phase 11)

### Claude's Discretion

- IDF cache shape (JSONB column vs Redis hash) — both work, pick the simpler one to test
- Whether `s5_affinity` JSONB key format is `{dim}:{attr_id}` or `{dim}.{attr_id}` — minor; pick once and stay consistent
- Whether the AniList client gets its own `cmd/` integration test or rides the parser unit-test pattern — match the existing Shikimori client test layout
- Whether the backfill job is parallelized (worker pool over rate-limit budget) or serial — at 3500 anime × 2 API calls × ~2s/call due to rate limits, serial is ~2 hours; parallel-with-rate-limit is ~30 min. Plan-phase chooses.
- Backfill error-bucket handling: keep going on individual failures vs. abort on a threshold (e.g., > 10% failures in any 100-row window)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/service/recs/{ensemble,normalize,signal,types}.go` — Phase 9 foundation. No changes needed.
- `services/player/internal/service/recs/user_orchestrator.go` — Phase 11. S5 just registers as another module in the cron's per-user run list.
- `services/player/internal/service/recs/signals/s1_score_cluster.go`, `s2_metadata.go` — Phase 11. S5 follows the same `SignalModule` interface contract; the test layout (`*_test.go` alongside) is the established pattern.
- `services/player/internal/repo/recs.go` — Phase 9. `UpsertUserSignals` already includes `s5_affinity` in the `DoUpdates` clause from the original Phase 9 schema. No repo change beyond confirming JSONB serialization round-trips correctly for the new map shape.
- `services/player/internal/domain/recs.go:23` — `S5Affinity string \`gorm:"type:jsonb;not null;default:'{}'"\`` already exists from Phase 9. Phase 12 starts populating it.
- `services/catalog/internal/parser/shikimori/client.go` — existing GraphQL client. Phase 12 adds `studios`, `kind`, `rating`, `source` to the existing query block (around line 105 — `shikimoriAnime` struct definition).
- `services/catalog/internal/domain/anime.go:32` — `Genres []Genre \`gorm:"many2many:anime_genres;"\`` is the established pattern. `Studios` and `Tags` mirror it.
- `libs/idmapping/` — ARM client already plumbed. `Resolve(shikimoriID).AniList` returns the AniList ID needed for tag fetching.
- `libs/cache` — used by AniList client for tag-cache (24h TTL).
- `services/maintenance/cmd/*` — existing one-shot maintenance tools live here; `backfill-attributes` is a natural fit.

### Established Patterns
- **Adding columns:** edit the GORM struct in `services/catalog/internal/domain/anime.go`, restart the catalog service. AutoMigrate adds the column. (Per CLAUDE.md: GORM only ADDS columns, never DROPS — safe for forward migrations.)
- **Adding m2m:** define the new entity (e.g., `Studio` with `ID`, `Name`), add `[]Studio` to `Anime` with `gorm:"many2many:anime_studios;"`, restart. Auto-creates the join table.
- **External GraphQL parser:** mirror `services/catalog/internal/parser/shikimori/client.go` for rate limiting, headers, structured logging, error handling.
- **Backfill scripts:** `services/maintenance/cmd/{name}/main.go` with a `flag.Parse()` for batch size, dry-run, etc. Existing `mal-import` is the closest analog.
- **TDD discipline (Phase 9/10/11 precedent):** S5 logic is non-trivial — write tests first. The TF-IDF formula has multiple branches (Kodik vs reliable players, attribute presence vs absence, cold-start) — each gets a fixture-driven test case.
- **JSONB persistence:** GORM serializes Go maps to JSONB cleanly; `S5Affinity string` (raw JSON) is the existing shape. Marshal/unmarshal happens in the S5 service layer, not in GORM.

### Integration Points
- **Catalog service:** Shikimori parser changes propagate when anime are searched/fetched. NEW anime get the new attributes immediately. EXISTING anime need the backfill job.
- **Player service:** S5 module added to `UserOrchestrator`'s module list and to the handler's logged-in ensemble registry. Both edits are one-line additions.
- **Maintenance service:** new backfill cmd. Wired into `Makefile` as `make backfill-attributes`. No long-running daemon — exits after completion.
- **No gateway changes** — `/api/users/recs` route unchanged.
- **No frontend changes** — Phase 11's "Up Next for you" row already renders the same payload shape; only ranking shifts.
- **No new env vars.**
- **Dependencies:** the existing `github.com/shurcooL/graphql` Go module (already used by Shikimori client) covers the AniList client too.

</code_context>

<specifics>
## Specific Ideas

- **Spec ref:** Design spec `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §3.1 is the binding TF-IDF formula. §13 locks the per-attribute weights; Decision §A2 (this CONTEXT.md) collapses producers into studios with documented rationale.
- **AniList rate budget:** 90 req/min × 60 min = 5400 req/h. Backfill of ~3500 anime is one fetch each = ~40 min serial. Parallel with a 2-token rate limiter cuts to ~20 min.
- **Shikimori `source` enum values** (verified from Shikimori's GraphQL playground): `original`, `manga`, `web_manga`, `four_koma_manga`, `digital_manga`, `novel`, `light_novel`, `visual_novel`, `game`, `card_game`, `book`, `picture_book`, `radio`, `music`, `other`, `unknown`. The S5 module treats each as an attribute key.
- **Testing fixtures:** `ui_audit_bot` is seeded with 8 watch_history entries across multiple players (per CLAUDE.md UI Audit Test User pattern). Use this account for the integration test of S5 — assert the affinity vector has non-zero entries on at least 3 of the 6 dimensions after a precompute.
- **Phase 11 deferred handoff:** Phase 11's CONTEXT.md explicitly handed the schema gap to Phase 12. The Phase 12 plan-phase has no further inventory work — the gap is known.
- **Per CLAUDE.md:** all new copy goes to BOTH EN and RU; JA can mirror EN. **There is NO new user-facing copy in this phase** — S5 just changes the order of cards in the existing row. Only thing to add: changelog.json entry on the after-update step (consistent with Phase 11).
- **Verification baseline (Phase 11 SUMMARY documented):** before S5 ships, `ui_audit_bot` sees a Phase-11 ordering of 50 recs. After S5 ships and a backfill+precompute completes, the top-3 anime should change. The Phase 12 verification step compares the two orderings — if the top-3 are identical, S5 is not contributing.

</specifics>

<deferred>
## Deferred Ideas

- S6 combo-watched-after pin → Phase 13
- Admin debug page (per-signal contribution table, S5 TF-IDF term breakdown, force-recompute) → Phase 14
- Per-signal CTR Prometheus metrics + `rec_click` / `rec_watched` events → Phase 14
- Tag rank-weighted TF-IDF (currently every AniList tag counts equally) → v2.1 once CTR data exists
- Producers as a separate dimension → v3.0 (no source separates them today)
- MAL tags as an additional source → v3.0 (AniList covers the same surface)
- Per-user adult-content filter → v2.1+ (out of scope until per-user setting exists)
- Demographic from genres derivation rule (e.g. shounen/seinen genre-tag mapping) → v2.1 polish if the rating-as-proxy approach undershoots
- Backfill rerun strategy on AniList rate-limit fail → manual for v2.0 (the job is idempotent so re-running picks up where it left off)

</deferred>
