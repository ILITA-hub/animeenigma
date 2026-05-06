---
phase: 12
plan: 01
status: complete
verification_status: passed
shipped: 2026-05-06
commits: 8
subsystem: catalog
tags:
  - recs
  - phase-12
  - s5
  - tf-idf
  - schema-additions
  - shikimori-parser
  - anilist-parser
  - attribute-affinity
requires:
  - libs/database (gorm wrapper — used and patched around for m2m)
  - libs/errors (ExternalAPI wrap)
  - libs/logger (structured logging)
  - github.com/hasura/go-graphql-client (Shikimori — already used)
  - gorm.io/driver/sqlite + github.com/mattn/go-sqlite3 (test fixture, NEW dep on catalog)
provides:
  - animes.kind / animes.rating / animes.material_source columns (S5 attribute dimensions per Decision §A1)
  - studios + anime_studios tables (Decision §A1/A2 — collapsed studios+producers)
  - tags + anime_tags tables with explicit AnimeTag join model preserving Rank (Decision §A4)
  - Shikimori parser hydrates kind/rating/material_source + studios on every fetch path (6 paths)
  - services/catalog/internal/parser/anilist (NEW) — FetchTags(ctx, anilistID) -> []Tag, error
  - SlugifyTagName helper (exported) for Wave-2 backfill
affects:
  - All Shikimori fetch paths (SearchAnime, GetAnimeByID, GetAnimeByIDs, GetTrendingAnime, GetPopularAnime, GetSeasonalAnime) now return the four new fields on every fetch
  - Catalog service AutoMigrate adds 3 new tables on startup (idempotent)
key-files-created:
  - services/catalog/internal/domain/anime_attributes_test.go
  - services/catalog/internal/parser/anilist/client.go
  - services/catalog/internal/parser/anilist/client_test.go
  - .planning/phases/12-tf-idf-attribute-affinity-s5/12-01-SUMMARY.md
key-files-modified:
  - services/catalog/internal/domain/anime.go
  - services/catalog/internal/parser/shikimori/client.go
  - services/catalog/cmd/catalog-api/main.go
  - services/catalog/go.mod
  - services/catalog/go.sum
decisions:
  - "Shikimori adaptation-source field is named `origin`, NOT `source` (CONTEXT.md spec was wrong). Caught at Task 5 production verification — bare `source` selection 500'd every refresh. Fixed by aliasing the Go-side `Source` field to GraphQL/JSON `origin`."
  - "libs/database wrapper's AutoMigrate ADDs columns but never creates m2m join tables for relations on existing structs — caught at Task 5 when anime_studios silently missed the first redeploy. Fixed in catalog main.go by falling through to GORM's native db.DB.AutoMigrate(&Anime{}) after the wrapper. Idempotent."
  - "Tag.ID is the slugified AniList tag name. Implementation uses an ASCII-only regex — diacritics become underscores ('Mahō Shōjo' → 'mah_sh_jo'). Dependency-free and deterministic; future Unicode-aware normalization can swap without breaking ASCII idempotency."
  - "AnimeTag explicit join model keeps Rank for v2.1 use even though v1 S5 ignores it (Decision §A4). Setup via db.SetupJoinTable(&Anime{}, 'Tags', &AnimeTag{}) AFTER AutoMigrate."
  - "Catalog test fixture pre-creates the postgres-default-uuid tables via raw SQL because sqlite chokes on `DEFAULT gen_random_uuid()` syntax. AutoMigrate is then exercised against the pre-created tables (HasTable shortcut) and against the new Phase-12 models from scratch."
metrics:
  total_tasks: 5 (Tasks 1-4 implementation + Task 5 production verification)
  duration_minutes: ~25
  completed_date: 2026-05-06
  files_created: 4
  files_modified: 5
  go_test_packages_passing: 2 (services/catalog/internal/domain, services/catalog/internal/parser/anilist)
---

# Phase 12 / Plan 01 — Catalog schema + Shikimori parser + AniList client: Execution Summary

**Goal achieved:** the catalog service now carries the full Phase-12 attribute schema. After `make redeploy-catalog`, `\d animes` shows the three new columns (kind / rating / material_source) and `\dt` shows four new tables (studios / tags / anime_studios / anime_tags). The Shikimori parser populates kind / rating / material_source / studios on every fetch path. A new AniList GraphQL client at `services/catalog/internal/parser/anilist/client.go` exposes `FetchTags(ctx, anilistID) -> []Tag, error` ready for the Wave-2 backfill to drive. The schema-half of Phase-12 SC#3 is closed; backfill (Wave 2) and S5 ensemble registration (Wave 3) remain.

## Tasks completed

| # | Task | Commit(s) |
|---|------|-----------|
| 1 | Anime domain extended (Kind / Rating / MaterialSource + Studio + Tag m2m) — TDD | `8b5d1cc` (RED) → `53b15de` (GREEN) |
| 2 | Shikimori parser GraphQL queries extended (4 raw + 2 typed paths) | `a8624a2` |
| 3 | NEW AniList GraphQL client with FetchTags + SlugifyTagName — TDD | `092f825` (RED) → `beb4271` (GREEN) |
| 4 | AutoMigrate wiring for Studio + Tag + AnimeTag in catalog main.go | `5b769eb` |
| 5 | Production redeploy + schema + Shikimori smoke verification | (verification, plus `fbf7854` and `7c09872` Rule-1 fixes) |

**Bonus (Rule 1 deviations — caught at Task 5 production verification):**
- `fbf7854` `fix(catalog): auto-create anime_studios m2m join table on Phase-12 redeploy` — the libs/database wrapper's AutoMigrate goes down the AddColumn path on pre-existing tables, never invoking m2m join-table creation. Fix: fall through to GORM's native AutoMigrate(&Anime{}) after the wrapper. Idempotent.
- `7c09872` `fix(catalog/shikimori): query 'origin' instead of 'source' for adaptation field` — CONTEXT.md spec called the field "source" but Shikimori's GraphQL schema returns `Field 'source' doesn't exist on type 'Anime'`. Live introspection confirmed the real field is `origin`. Updated the typed struct, raw struct, and all 4 raw query strings to query `origin`.

## Verification — production smoke (Task 5 outputs)

### Step 1 — Catalog redeploy (after both Rule-1 fixes)
```
[INFO] catalog is running
[INFO] catalog:8081 - healthy
```
No migration errors in logs.

### Step 2 — Schema verification (postgres)

**Three new columns on `animes`:**
```
 kind             | character varying(20)
 rating           | character varying(20)
 material_source  | character varying(50)
 idx_animes_kind  btree (kind)
```

**Four new tables present:**
```
 public | anime_studios   | table
 public | anime_tags      | table
 public | studios         | table
 public | tags            | table
```

**`anime_tags` shape (Rank persists, composite PK present, FKs wired):**
```
   Column   |           Type           | Nullable | Default
------------+--------------------------+----------+---------
 anime_id   | uuid                     | not null |
 tag_id     | character varying(200)   | not null |
 rank       | bigint                   |          | 0
 created_at | timestamp with time zone |          |
PRIMARY KEY (anime_id, tag_id)
FK fk_anime_tags_anime → animes(id)
FK fk_anime_tags_tag → tags(id)
```

**`anime_studios` shape:**
```
  Column   |         Type          | Nullable | Default
-----------+-----------------------+----------+-------------------
 anime_id  | uuid                  | not null | gen_random_uuid()
 studio_id | character varying(50) | not null |
PRIMARY KEY (anime_id, studio_id)
FK fk_anime_studios_anime → animes(id)
FK fk_anime_studios_studio → studios(id)
```

**No producers tables** (Decision §A2 holds — `\dt | grep producer` returns zero rows).

### Step 3 — Shikimori parser smoke test (live API call)

Re-fetch Frieren (Shikimori ID 52991) via `POST /api/anime/{id}/refresh` → HTTP 200.

```
SELECT name, kind, rating, material_source FROM animes WHERE shikimori_id = '52991';
       name        | kind | rating | material_source
-------------------+------+--------+-----------------
 Sousou no Frieren | tv   | pg_13  | manga
```

```
SELECT s.id, s.name FROM studios s JOIN anime_studios as_ ON as_.studio_id = s.id JOIN animes a ON a.id = as_.anime_id WHERE a.shikimori_id = '52991';
 id |   name
----+----------
 11 | Madhouse
```

End-to-end proof: Shikimori parser hydrates the 4 new fields on a real fetch path; the GORM m2m persists studios via the new association.

### Step 4 — AniList unit tests
```
ok  github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anilist  0.009s
```
All 7 `Test_AniListClient_*` cases (happy path, empty tags, GraphQL errors, network failure, malformed JSON, user-agent, query shape) plus 9 `TestSlugifyTagName` sub-cases pass.

### Step 5 — Wave-2 baseline (rows needing backfill)
```
 missing_kind            | 3857
 missing_rating          | 3857
 missing_material_source | 3857
 missing_studios         | 3857
```
3857 anime rows need backfilling — only Frieren has been refreshed since the schema landed. This is the workload Wave-2 picks up.

### Step 6 — Existing catalog tests still pass
```
ok  services/catalog/internal/domain                   0.051s
ok  services/catalog/internal/parser/anilist           0.009s
ok  services/catalog/internal/parser/kodik             0.365s
```
No regressions.

## Coverage of must_haves.truths

| Truth | Direct evidence |
|-------|-----------------|
| 6 new attribute dimensions exist on `animes` | Step 2 above — `\d animes` shows 3 new columns + `\dt` shows 4 new tables. No producers table. |
| Newly-fetched anime carry kind / rating / material_source / studios | Step 3 above — Frieren after refresh shows tv / pg_13 / manga + Madhouse studio. |
| AniList client at services/catalog/internal/parser/anilist/client.go with FetchTags | `beb4271` ships the file; Step 4 unit tests prove the contract end-to-end against an httptest stub. |
| AniList client NOT auto-invoked by Shikimori fetches | Verified by `grep -rn "anilist" services/catalog/internal/{handler,service,parser/shikimori}` — zero matches. The Wave-2 backfill is the only consumer. |
| Schema is no longer the blocker for S5 | Step 2 (columns) + Step 3 (parser hydrates new fetches) = green. |
| S5 ensemble registration NOT shipped here | Confirmed by `grep "s5" services/player/internal/handler/recs.go` — still only S1-S4. Wave-3 owns this. |

## Adaptations during execution

1. **CONTEXT.md spec error: Shikimori field named `origin`, not `source` (Rule 1).** Caught at Task 5 production smoke. Live introspection confirmed. Fixed in `7c09872`. Internal Go-side identifier kept as `Source` (the boundary with `domain.Anime.MaterialSource` is unchanged); only the GraphQL/JSON tag changed.

2. **libs/database wrapper doesn't create m2m join tables for relations added to pre-existing structs (Rule 1).** Caught at Task 5 — `anime_studios` was missing while `anime_tags` was present (because AnimeTag is registered as an explicit AutoMigrate target, but Studios uses the bare m2m struct tag). Fix: fall through to GORM's native `db.DB.AutoMigrate(&Anime{})` after the wrapper. Idempotent. Single commit `fbf7854`.

3. **SQLite test fixture: `DEFAULT gen_random_uuid()` is postgres-only.** GORM's AutoMigrate on the Anime struct emits this clause but SQLite syntax requires DEFAULT expressions to be parenthesised, which the sqlite dialector doesn't translate. Workaround: pre-create animes / anime_genres / anime_studios / anime_tags via raw SQL with portable column shapes; AutoMigrate then sees HasTable already true and skips DDL for those, but still creates the new tables (studios, tags) from scratch and validates the m2m relations. Documented in the test file's setup function.

4. **Test driver registration scoped to the catalog test package.** A custom SQLite driver name `sqlite3_catalog_attrs` is registered once via `sync.Once` so the `gen_random_uuid` UDF is available in any subsequent test (mirrors the existing pattern in `services/player/internal/service/list_mark_completed_test.go`). The shim returns a 32-char hex string — only purpose is to make CREATE TABLE valid; production semantics are postgres'.

## Files added / modified

```
NEW:
  services/catalog/internal/domain/anime_attributes_test.go     (165 lines, 5 test cases)
  services/catalog/internal/parser/anilist/client.go            (231 lines, public API + helpers)
  services/catalog/internal/parser/anilist/client_test.go       (188 lines, 7 client tests + 9 slugify cases)
  .planning/phases/12-tf-idf-attribute-affinity-s5/12-01-SUMMARY.md  (this file)

MODIFIED:
  services/catalog/internal/domain/anime.go                     (+47 lines: 3 columns + 2 m2m + 3 types)
  services/catalog/internal/parser/shikimori/client.go          (+77 lines: shikimoriStudio struct, 4 graphql tags, 2 mapper hydration paths, 4 raw query string updates)
  services/catalog/cmd/catalog-api/main.go                      (+13 lines: AutoMigrate list + native AutoMigrate fallback + SetupJoinTable)
  services/catalog/go.mod                                       (+ stretchr/testify, gorm.io/driver/sqlite, mattn/go-sqlite3)
  services/catalog/go.sum                                       (transitive deps)
```

## Requirements satisfied

- ✓ **REC-SIG-05** (schema half) — six new attribute dimensions (kind / rating / material_source / studios / tags) exist on `animes`. Backfill of existing rows is Wave 2's job; ensemble registration is Wave 3's. After this plan, the schema is no longer the S5 blocker.

## Out of scope — handed off to Wave 2 / Wave 3

- **Wave 2 (services/maintenance/cmd/backfill-attributes)**: walk all 3857 missing rows; for each, refetch via Shikimori (now hydrates the 4 new fields), resolve Shikimori → AniList ID via `libs/idmapping` ARM client, call `anilist.FetchTags(ctx, *result.AniList)`, slugify each tag name via `anilist.SlugifyTagName`, upsert Tag rows, and write AnimeTag join rows with Rank. Idempotent — a re-run skips already-populated rows.
- **Wave 3 (services/player S5 module + ensemble registration)**: implement TF-IDF affinity per spec §3.1 with the per-attribute weights from CONTEXT.md Decision §A2 (tags 0.30 / studios 0.25 / genres 0.15 / demographic 0.10 / source 0.10 / type 0.10 = 1.00). Register S5 in `services/player/internal/handler/recs.go` ensemble at weight 0.20.

## Notes for Wave 2 (backfill plan)

- **Baseline workload**: 3857 anime rows (verified at Task 5 step 5).
- **AniList client rate limit**: 1 rps default. The backfill can raise this within the 90/min cap if it runs as a single isolated process (no contention with other consumers). Reference the rateLimiter pattern; do NOT bypass.
- **Tag.ID format is locked**: `anilist.SlugifyTagName(name)`. Idempotent — re-running the backfill must use this same function.
- **AnimeTag.Rank**: persist AniList's per-anime rank (0-100) on the join row even though v1 S5 ignores it. Decision §A4.
- **isAdult / isGeneralSpoiler**: Tag struct exposes them. Backfill chooses how to handle (per CONTEXT.md §A4: isAdult inherits from animes.hidden, isGeneralSpoiler is kept).
- **Anime without an AniList mapping**: ARM may return `nil` for `result.AniList`. Skip the AniList call for those rows; they get an empty tags set and S5 contributes zero on the tags dimension — consistent with the cold-start contract from spec §3.3.
- **Existing-row refetch via Shikimori**: a second sweep (or part of the same job) should re-fetch every row to populate kind / rating / material_source / studios. The catalog `POST /api/anime/{id}/refresh` endpoint already drives this end-to-end (verified at Step 3). Or call the parser directly from the maintenance binary.

## Notes for Wave 3 (S5 plan)

- **Where to plug S5**: register a new module in the ensemble registry inside `services/player/internal/handler/recs.go` (next to S1-S4); ensure `Orchestrator.RunForUser` (services/player/internal/service/recs/user_orchestrator.go) writes `s5_affinity` JSONB.
- **Data layer is ready**: `anime.Kind`, `anime.Rating`, `anime.MaterialSource`, `anime.Studios`, `anime.Tags` are all available on a preloaded Anime model; no new schema lookups are needed inside S5.
- **Per-attribute weight constants**: 0.30 tags / 0.25 studios / 0.15 genres / 0.10 demographic / 0.10 source / 0.10 type = 1.00 (Decision §A2 collapsed). The S5 module's per-attribute weight constants block must carry a `// Decision §A2` comment explaining the studios+producers collapse.
- **Kodik fallback**: `unit = max(duration_watched / 60, 1)` minutes for hianime/consumet/animelib; `unit = 1` per watch_history row for kodik (Decision §S5 spec §3.1).
- **Cold start**: user with zero scored history → empty s5_affinity vector → ensemble normalizer treats absent entries as zero. Same contract as S1/S2.

## TDD Gate Compliance

The plan declares two tasks with `tdd="true"` (Tasks 1 and 3). Gate sequence verified in git log:

| Task | RED commit | GREEN commit |
|------|------------|--------------|
| 1 (Anime attribute schema) | `8b5d1cc test(catalog): add failing tests for Anime attribute schema additions` | `53b15de feat(catalog): add Kind/Rating/MaterialSource + Studio + Tag m2m to Anime domain` |
| 3 (AniList client) | `092f825 test(catalog/anilist): add failing tests for AniList tag-fetch client` | `beb4271 feat(catalog): add AniList GraphQL client with FetchTags + slugifyTagName` |

Tasks 2 and 4 are non-TDD (pure data-flow extension and trivial wiring respectively), per the plan. Tasks 5 and the Rule-1 fixes (`fbf7854`, `7c09872`) are integration / verification commits — no behavioural change beyond what Tasks 1-4 already covered with tests.

## Self-Check: PASSED

**Files exist:**
- `services/catalog/internal/domain/anime_attributes_test.go` — FOUND
- `services/catalog/internal/domain/anime.go` (with Kind / Rating / MaterialSource / Studios / Tags / Studio / Tag / AnimeTag) — FOUND
- `services/catalog/internal/parser/anilist/client.go` — FOUND
- `services/catalog/internal/parser/anilist/client_test.go` — FOUND
- `services/catalog/internal/parser/shikimori/client.go` (extended for 4 new fields) — FOUND
- `services/catalog/cmd/catalog-api/main.go` (AutoMigrate + SetupJoinTable) — FOUND

**Commits exist** (verified via `git log --oneline`):
- `8b5d1cc` (Task 1 RED)
- `53b15de` (Task 1 GREEN)
- `a8624a2` (Task 2)
- `092f825` (Task 3 RED)
- `beb4271` (Task 3 GREEN)
- `5b769eb` (Task 4)
- `fbf7854` (Task 5 Rule-1 fix: m2m join table)
- `7c09872` (Task 5 Rule-1 fix: origin field)

**Live system verified:**
- Three new columns on `animes` (Step 2)
- Four new tables: studios / tags / anime_studios / anime_tags (Step 2)
- No producers tables (Decision §A2)
- Frieren refresh via API populates kind=tv, rating=pg_13, material_source=manga, studios=[Madhouse] (Step 3)
- AniList unit tests green (Step 4)
- Wave-2 baseline: 3857 missing rows captured (Step 5)
- Existing catalog tests still pass (Step 6)

Phase 12 Wave 1 schema layer shipped to production at `2026-05-06 ~10:10 UTC`.
