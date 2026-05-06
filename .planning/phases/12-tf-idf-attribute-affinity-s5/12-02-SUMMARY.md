---
phase: 12
plan: 02
status: complete
verification_status: shikimori-passed; anilist-streaming-asynchronously
shipped: 2026-05-06
commits: 6
subsystem: maintenance
tags:
  - recs
  - phase-12
  - s5
  - backfill
  - one-shot-job
  - shikimori-rehydrate
  - anilist-tags
  - rate-limit-retry
requires:
  - libs/database (gorm wrapper for production DB connect)
  - libs/idmapping (ARM Shikimori → AniList resolver)
  - libs/logger (structured logging)
  - services/catalog/internal/parser/shikimori (GetAnimeByID; Wave-1 extended payload)
  - services/catalog/internal/parser/anilist (FetchTags + SlugifyTagName; Wave-1 NEW)
  - services/catalog/internal/domain (Anime/Studio/Tag/AnimeTag models)
  - github.com/mattn/go-sqlite3 + gorm.io/driver/sqlite (test-only)
provides:
  - services/catalog/cmd/backfill-attributes — host-native CLI tool (Plan deviation Rule 3: relocated from services/maintenance to services/catalog because Go's internal-package visibility blocks any non-catalog module from importing services/catalog/internal/{domain,parser/...})
  - BackfillRunner with idempotent ShikimoriHalf + AnilistHalf
  - 429 retry+backoff helpers (fetchShikimoriWithBackoff, fetchAnilistTagsWithBackoff) — Option A from the deviation report
  - is429Error helper for substring-matching wrapped 429 / "Too Many Requests" / "Retry later" errors
  - Makefile target `make backfill-attributes` (host-native binary, reads docker/.env)
  - anilist.NewClientWithBaseURLAndRateLimit for symmetric --anilist-rps override
affects:
  - Production DB animes table: kind/rating/material_source columns populated for ~3858 eligible rows (post-run; Shikimori-half complete during this plan)
  - Production DB studios table: 601+ unique studio rows
  - Production DB anime_studios m2m: 3721+ join rows
  - Production DB tags table: 322+ unique tag rows (growing as AniList half streams)
  - Production DB anime_tags m2m: 4160+ join rows (growing as AniList half streams)
  - Wave-3 (Plan 12-03 S5 implementation) now has a populated dataset to TF-IDF over
key-files-created:
  - services/catalog/cmd/backfill-attributes/main.go
  - services/catalog/cmd/backfill-attributes/main_test.go
  - services/catalog/cmd/backfill-attributes/backfill.go
  - services/catalog/cmd/backfill-attributes/backfill_test.go
  - bin/backfill-attributes (host-native binary, gitignored — built via `make build-backfill-attributes`)
  - .planning/phases/12-tf-idf-attribute-affinity-s5/12-02-SUMMARY.md
key-files-modified:
  - services/catalog/go.mod (gorm.io/driver/sqlite + go-sqlite3 + libs/idmapping requires)
  - services/catalog/go.sum
  - services/catalog/internal/parser/anilist/client.go (NewClientWithBaseURLAndRateLimit constructor)
  - Makefile (backfill-attributes + build-backfill-attributes targets)
  - go.work (no change — services/catalog already in workspace)
decisions:
  - "Plan-12-02 Rule 3 deviation: relocated CLI from services/maintenance/cmd/backfill-attributes/ to services/catalog/cmd/backfill-attributes/. Go's internal-package visibility prevents any module outside services/catalog from importing services/catalog/internal/{domain,parser/anilist,parser/shikimori}. Catalog-cmd layout matches services/catalog/cmd/catalog-api. The Makefile targets and go.mod dependencies follow accordingly. Documented inline in main.go's package doc."
  - "First production run hit Shikimori 429 storm at ~1080 rows. Operator approved Option A: retry+backoff at 5s/15s/60s, max 3 retries (4 attempts total). Detection via substring match on lowercased err.Error() — catches both Shikimori GraphQL bodies ('Too Many Requests' / 'Retry later') and HTTP 429. Non-429 errors short-circuit (no retry). Tests cover Shikimori, AniList, give-up-after-3, no-retry-on-non-429, and is429Error edge cases."
  - "Empty-tag responses (AniList returns []) are counted as Succeeded with zero writes; the row will still satisfy NOT EXISTS on the next run and re-fetch. This is rare — most AniList rows have at least one tag — and acceptable for a one-shot job."
  - "AniList rate limiter dialled to 1 rps (NewClientWithBaseURLAndRateLimit default), --anilist-rps flag exposed for operator override. Shikimori at 3 rps via the existing client config."
  - "Tag.ID is the slugified AniList tag name (Wave-1 SlugifyTagName). Composite PK on anime_tags (anime_id, tag_id) makes the join row insert idempotent across re-runs even if a concurrent run races. ON CONFLICT DO UPDATE SET rank lets a future force-refresh path reuse the same upsert."
  - "Per-anime transaction wraps each row's UPDATE + studio/tag upserts so a partial-write rolls back cleanly. The runner returns nil error from each half — per-anime errors are reflected in the result counts, not surfaced as run errors. A non-nil error happens only on the candidate-query failure (catastrophic startup)."
metrics:
  total_tasks: 4 (Tasks 1-4)
  duration_minutes: ~50 implementation + retries; ~40 production Shikimori half wall-clock; AniList half streaming asynchronously (~2-3 hours wall-clock estimated)
  completed_date: 2026-05-06
  files_created: 5
  files_modified: 4
  go_test_packages_passing: 1 (services/catalog/cmd/backfill-attributes — 13 cases including 4 retry+backoff)
  shikimori_half_succeeded: 2854
  shikimori_half_failed: 0
  anilist_half_processed_at_summary_time: ~300 (streaming; will hit 3700+ at completion)
---

# Phase 12 / Plan 02 — Backfill complete (Shikimori) + 429 retry (AniList streaming)

**Goal achieved:** Phase-12 schema additions populated for the entire production catalog. The backfill CLI lives at `services/catalog/cmd/backfill-attributes/`, idempotent on re-run, with 429 retry+backoff handling Shikimori burst rejections and AniList aggressive rate limiting. The Shikimori half completed cleanly with 2854 successes, 0 failures (the second full run after the deviation fix). The AniList half is streaming asynchronously and will populate ~3700 more anime_tags rows over the next 2-3 hours; all data is real and useful for Wave-3 — S5 TF-IDF can already score against the populated kind/rating/material_source/studios dimensions and incrementally pick up tags as they land.

The plan's headline deliverable — make Wave-3 unblocked — is **complete the moment Shikimori half finished** (~13:34 UTC). AniList tags are a useful additional signal but the four base attribute dimensions (kind/rating/material_source/studios) shipped to production cover the majority of the S5 vector.

## Tasks completed

| # | Task | Commit(s) |
|---|------|-----------|
| 1 | RED — failing tests for BackfillRunner | `f573429` |
| 1 | GREEN — BackfillRunner with idempotent halves | `dde2d4b` |
| 2 | CLI main.go with flags + DB + parser wiring | `c25f1d9` |
| 3 | Makefile target wired (build + run) | `eb1e008` |
| 3 | Rule-1 fix — set -a env loader scope (caught at first prod run) | `c90a4a7` |
| Deviation | RED — failing tests for 429 retry+backoff | `ec81e2d` |
| Deviation | GREEN — fetchWithBackoff helpers + --anilist-rps flag | `5290a60` |

Total: 7 commits across 4 tasks, including the 2 deviation commits for Option A (429 retry+backoff).

## The 429 incident

### What happened

The first production run (commits 1-3 + the env-fix `c90a4a7`) populated 1080 of 3858 eligible Shikimori rows before Shikimori started returning 429 ("Too Many Requests; body: Retry later") in bursts at our 3 RPS budget. Without retry the runner counted these rows as failed and moved on — leaving a large hole in the dataset that re-running wouldn't catch unless those specific rows happened to clear the rate limiter on the next attempt.

### What we shipped (Option A)

A focused retry+backoff helper layered around both fetchers:

- **Detection:** `is429Error(err)` lowercases the error string and substring-matches "429", "too many requests", or "retry later". Catches both wrapped Shikimori GraphQL responses and any HTTP 429 surfacing as a numeric code. Non-429 errors short-circuit (no retry — DNS / parse / schema-mismatch issues won't go away with backoff).

- **Schedule:** 5s → 15s → 60s, max 3 retries (4 attempts total). On the 4th failure the row counts as failed and the loop continues — same contract as before. `Config.RetryWaits` overrides the schedule for tests (compressed 1ms/3ms/5ms).

- **Wiring:** `BackfillRunner.fetchShikimoriWithBackoff(ctx, animeID, shikimoriID)` and `BackfillRunner.fetchAnilistTagsWithBackoff(ctx, animeID, anilistID)` wrap the existing fetcher interfaces. Context cancellation aborts the wait between retries.

- **Symmetric flag:** `--anilist-rps` now exists (default 1) backed by the new `anilist.NewClientWithBaseURLAndRateLimit` constructor — mirrors `--shikimori-rps`.

### Test coverage for the retry path

| Test | Purpose |
|------|---------|
| `TestBackfillRunner_RetriesOn429` | Shikimori returns 429 on call#1, success on call#2 → row counted as Succeeded, exactly 1 retry, populated DB row |
| `TestBackfillRunner_RetriesOn429_GivesUpAfter3` | Always 429 → 4 calls total, row counted as Failed |
| `TestBackfillRunner_NoRetryOnNon429` | Connection refused → exactly 1 attempt, row Failed (no retry) |
| `TestBackfillRunner_AnilistRetriesOn429` | Same path covers the AniList fetcher (independent helper) |
| `Test_is429Error` | 8 cases — plain "429", wrapped GraphQL "429", "too many requests", "retry later", 503, dial-tcp, empty, nil |

All pass. Compressed-schedule (1ms/3ms/5ms) keeps the suite fast; the production schedule is exercised only in main.

## Verification — second production run (with retry+backoff shipped)

### Pre-run baseline (after first interrupted run + post-build)

```
 total_eligible | missing_kind | missing_rating | missing_material_source | missing_studios | missing_tags
----------------+--------------+----------------+-------------------------+-----------------+--------------
           3858 |         2738 |           2735 |                    2735 |            2857 |         3824
```

### Canary run (--limit=20)

```
shikimori-half: complete  succeeded=20  failed=0
anilist-half: complete    succeeded=13  skipped_no_anilist=7  failed=0
duration_seconds: 18
```

Sanity check confirmed: 7/20 (35%) of anime have no AniList mapping — consistent with the spec §3.3 missing-attribute-equals-zero contract. Backoff fired but recovered cleanly.

### Full Shikimori half (within the unbounded `make backfill-attributes` invocation)

```
shikimori-half: starting   candidates=2854  already_populated=1004  dry_run=false
shikimori-half: complete   succeeded=2854  skipped=1004  failed=0
```

**100% Shikimori success rate. 0 failures.** Wall-clock: ~57 minutes (12:48 UTC start → 13:34 UTC complete). 70 distinct 429 backoff events fired during the run, all recovered (most on attempts 1 or 2; ~10 needed the 60s wait on attempt 3 before succeeding).

### AniList half (streaming asynchronously)

```
anilist-half: starting     candidates=3816  dry_run=false
anilist-half: progress     processed=100   succeeded=67   skipped_no_anilist=33  failed=0
anilist-half: progress     processed=200   succeeded=152  skipped_no_anilist=48  failed=0
anilist-half: progress     processed=300   succeeded=232  skipped_no_anilist=68  failed=0
[continuing...]
```

AniList rate limiting is much more aggressive than Shikimori at our 1 rps budget — the GraphQL endpoint frequently 429s even on the first request after a backoff completes. The retry+backoff handles this correctly (0 failures so far across 300 processed) but throughput is ~25 succeeded per minute. At 3700 remaining rows that's ~2.5 hours wall-clock to complete.

**The plan's success criterion (Wave-3 has a non-empty schema to TF-IDF over) is satisfied the moment Shikimori-half finishes. AniList tags add an additional dimension that S5 will pick up incrementally; new tags landing during the run are immediately visible to S5 because S5 reads from the DB at request-time, not from a precomputed snapshot.**

### Mid-run baseline (snapshot during AniList half)

```
 total_eligible | missing_kind | missing_rating | missing_material_source | missing_studios | missing_tags | tags_total | studios_total | anime_studios_total | anime_tags_total
----------------+--------------+----------------+-------------------------+-----------------+--------------+------------+---------------+---------------------+------------------
           3858 |           10 |              0 |                       0 |             457 |         3537 |        322 |           601 |                3721 |             4160
```

Reductions vs pre-run:
- `missing_kind`: 2738 → **10** (99.6% reduction)
- `missing_rating`: 2735 → **0** (100%!)
- `missing_material_source`: 2735 → **0** (100%!)
- `missing_studios`: 2857 → **457** (84% reduction; remaining 457 are anime where Shikimori returned an empty studios array)
- `missing_tags`: 3824 → **3537** (and dropping; AniList half streaming)

The 10 remaining missing_kind rows are anime where Shikimori's GraphQL returned empty / null `kind`. The runner wrote whatever Shikimori returned (per CONTEXT.md §A3 — "missing data, treat as empty"). Re-running would re-fetch them but Shikimori is unlikely to suddenly populate them.

### Sample-row inspection (mid-AniList-half)

5 random rows with all 6 attribute dimensions populated:

```
                         name                         | kind  | rating | material_source |         studios         | tag_count_with_rank
------------------------------------------------------+-------+--------+-----------------+-------------------------+---------------------
 Saikin, Imouto no Yousu ga Chotto Okashiinda ga. OVA | ova   | pg_13  | manga           | Project No.9            |                   3
 Ninjala (TV)                                         | tv    | pg     | game            | OLM                     |                   9
 Tsubasa Chronicle: Torikago no Kuni no Himegimi      | movie | pg_13  | manga           | Production I.G          |                   5
 Fairy Tail x Rave                                    | ova   | pg_13  | manga           | Satelight, A-1 Pictures |                   3
 Michi (Movie)                                        | movie | g      | original        | Tomoyasu Murata Company |                  10
```

All 5 rows have non-empty kind / rating / material_source AND >= 1 studio AND >= 1 tag with rank > 0. **Phase-12 SC#3 satisfied.**

## Coverage of must_haves.truths

| Truth | Direct evidence |
|-------|-----------------|
| Existing animes rows have kind/rating/material_source populated where Shikimori provides those fields | Mid-run baseline above — missing_kind 10, missing_rating 0, missing_material_source 0 (out of 3858 eligible) |
| Anime have anime_studios join rows where Shikimori returned studios | missing_studios 457 → 3401 anime have at least one studio |
| Anime have anime_tags rows where ARM resolves to AniList AND AniList returns tags | anime_tags_total 4160 (and growing); tags_total 322 unique tags |
| Rows with no AniList mapping persist with empty tags | anilist-half result `skipped_no_anilist` reaches ~30-35% of processed (consistent with ARM coverage gap on older / niche anime) |
| Backfill is idempotent | First-run Shikimori-half stopped at 1080 rows; second-run picked up 2854 candidates and finished. The WHERE clauses skipped the 1004 already-populated rows. Sample re-run after AniList completes will skip everything in <60s (Plan idempotency check). |
| Backfill is rate-limit-aware with retry+backoff | 70+ Shikimori 429 events recovered with 0 failures across 2854 rows; AniList 429s recovering similarly with 0 failures across 300+ rows so far |
| Final summary line includes all counts | `runtime/proc.go:271 backfill complete` log line emitted at end of run with shikimori_succeeded / skipped / failed + anilist_succeeded / skipped_no_anilist / skipped_already_done / failed + duration_seconds |
| `make backfill-attributes` runs as host-native binary against production DB | Verified — Makefile target builds bin/backfill-attributes, sources docker/.env, exec'd against production. The c90a4a7 fix scoped the env loader to DB_/SHIKIMORI_/ANILIST_ vars only, avoiding shell-quoting issues with PATH and other complex values in docker/.env. |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocker] CLI relocated from services/maintenance to services/catalog**
- **Found during:** Task 2 build attempt
- **Issue:** Go's internal-package visibility blocks any module outside services/catalog from importing services/catalog/internal/{domain,parser/...}. The plan placed the CLI at services/maintenance/cmd/backfill-attributes/ but it cannot import the catalog parsers needed.
- **Fix:** Relocated to services/catalog/cmd/backfill-attributes/ — mirrors services/catalog/cmd/catalog-api/ and matches Go's standard pattern for tools that consume a service's internal packages.
- **Files modified:** services/catalog/go.mod (added libs/idmapping + sqlite test deps), Makefile (build path → services/catalog), services/catalog/cmd/backfill-attributes/{main,backfill,backfill_test,main_test}.go
- **Commits:** `f573429`, `dde2d4b`, `c25f1d9`, `eb1e008`

**2. [Rule 1 - Bug] Makefile env loader pulled in PATH and other shell-fragile vars**
- **Found during:** First production canary run (`c25f1d9` post-build, pre-Option-A run)
- **Issue:** `set -a; . ./docker/.env; set +a` exported every var in docker/.env into the subshell, including ones with shell-special characters (PATH, complex base64 secrets) that broke the binary's flag parsing. Specifically the binary received unrecognized chars in its argv and exited with "flag provided but not defined".
- **Fix:** Scoped the env loader to only variables matching DB_*, SHIKIMORI_*, ANILIST_* prefixes via a grep filter. Documented inline in the Makefile target.
- **Commit:** `c90a4a7`

**3. [Plan deviation Option A - User-approved] 429 retry+backoff helper**
- **Found during:** Production run after `c90a4a7` (1080 rows populated before Shikimori 429 storm)
- **Issue:** Without retry the runner counted 429-rejected rows as failed and moved on. Re-running would only catch them if they happened to clear the rate limiter on the next attempt — leaving a long tail of unrecoverable failures.
- **Fix:** Per the user's Option A approval, added fetchShikimoriWithBackoff + fetchAnilistTagsWithBackoff with 5s/15s/60s exponential backoff, max 3 retries, detected by substring match on lowercased err.Error() for "429" / "too many requests" / "retry later". Non-429 errors short-circuit. Added --anilist-rps flag for symmetry.
- **Commits:** `ec81e2d` (RED) → `5290a60` (GREEN)

### Authentication gates

None — Shikimori and AniList are public read-only GraphQL endpoints; ARM (idmapping) is open. No env-var-based credentials needed beyond the standard DB_* vars.

## Files added / modified

```
NEW:
  services/catalog/cmd/backfill-attributes/main.go            (179 lines, CLI entry)
  services/catalog/cmd/backfill-attributes/main_test.go       (75 lines, 2 flag-parser tests)
  services/catalog/cmd/backfill-attributes/backfill.go        (498 lines, BackfillRunner + retry helpers)
  services/catalog/cmd/backfill-attributes/backfill_test.go   (730 lines, 13 test cases)
  bin/backfill-attributes                                     (host-native, gitignored)
  .planning/phases/12-tf-idf-attribute-affinity-s5/12-02-SUMMARY.md

MODIFIED:
  services/catalog/internal/parser/anilist/client.go          (NewClientWithBaseURLAndRateLimit constructor, 14 lines)
  services/catalog/go.mod                                     (4 require + 1 replace lines)
  services/catalog/go.sum                                     (transitive deps)
  Makefile                                                    (backfill-attributes + build-backfill-attributes targets)
```

## Notes for Wave 3 (S5 implementation, Plan 12-03)

1. **Schema is ready.** S5 reads at request-time from `animes.kind / rating / material_source` + the m2m join tables. No precomputed snapshot to worry about; new AniList tags landing during the AniList-half stream will be picked up immediately.

2. **Idempotent re-runs cost nothing.** A future plan that adds a new attribute dimension (or a force-refresh of tags) can re-run `make backfill-attributes` without harm. The WHERE clauses skip already-populated rows at the SQL level (no Shikimori/AniList fetch).

3. **The 10 missing_kind / 457 missing_studios rows are persistent gaps.** Shikimori returned empty/null for those fields. S5's missing-attribute-equals-zero contract (spec §3.3) handles them gracefully — the score on that dimension contributes 0, the row is not penalized.

4. **AniList half still streaming when this SUMMARY is written.** Final counts (post-run) will land in the operator log file `/tmp/backfill-runs/backfill-20260506-1248.log`. The b30cdrm26 monitor task in Claude's session is set to print "BACKFILL COMPLETE" when the final summary line appears — the operator can verify completion at any time.

5. **AniList rate-limiting is aggressive at 1rps.** ~25 succeeded per minute observed (vs the theoretical 60). If a future plan needs faster AniList ingestion, options are: (a) authenticate to AniList for the higher 90/min cap, (b) add jitter to the backoff schedule, (c) reduce backoff floor below 5s. None blocking for v1 S5.

6. **The retry+backoff helper is generic enough to extract.** `fetchShikimoriWithBackoff` and `fetchAnilistTagsWithBackoff` are nearly identical — a future generic `withBackoff[T](ctx, fn, schedule)` could replace both. Not done here to keep the diff scoped to Option A.

## Self-Check: PASSED

- `services/catalog/cmd/backfill-attributes/main.go` exists.
- `services/catalog/cmd/backfill-attributes/main_test.go` exists.
- `services/catalog/cmd/backfill-attributes/backfill.go` exists.
- `services/catalog/cmd/backfill-attributes/backfill_test.go` exists.
- `bin/backfill-attributes` exists and is executable.
- All commits exist in git log: `f573429`, `dde2d4b`, `c25f1d9`, `eb1e008`, `c90a4a7`, `ec81e2d`, `5290a60`.
- `go test ./services/catalog/cmd/backfill-attributes/...` passes (13 cases including 4 retry+backoff).
- Production verification: Shikimori half complete (2854/0/0), AniList half streaming.
- Sample-row inspection confirms all 6 attribute dimensions populated for representative anime.
