# Recommendations Readiness — what's still missing for REC-01 / REC-02

**Status:** Reference document. Phase 8 of the auto-pick milestone. No
recommendations engine is built in this phase — this file is the shopping
list for the next milestone.

**Context:** The auto-pick milestone (Phases 1–7) instrumented and rewrote
the Tier 2 picker. Several columns and events added during that work are
also reusable as recommendation inputs. This doc inventories what is now
available, what is still missing, and the minimum additions required to
ship REC-01 ("Because you watched X") and REC-02 ("Similar to X") from the
v2 backlog (PROJECT.md "Out of Scope").

## What we ship today (reusable for recs)

The columns and events below were added during Phases 1, 3, 5, and 6 and
already capture the implicit-feedback signal a basic recs engine needs.

| Source | Field / event | Captures | Phase |
|---|---|---|---|
| `watch_history` | `(user_id, anime_id, episode_number)` | implicit "user watched anime" event | pre-existing |
| `watch_history` | `duration_watched` | seconds spent on the episode (cumulative across heartbeats) | pre-existing |
| `watch_history` | `language, watch_type, translation_id, translation_title, player` | which combo the user actually consumed | pre-existing |
| `watch_history` | `session_id` | groups heartbeats from the same playback so a recs engine can distinguish a fresh open from a session-resume without double-counting | Phase 5 (G-04-lite) |
| `watch_history` | `watched_at` | timestamp for time-decayed weighting (already used by Tier 2 30-day half-life) | pre-existing |
| `watch_progress` | `completed` | canonical "user finished episode N" — single source of truth across the 4 players | Phase 3 |
| `watch_progress` | `watch_count` | rewatch detector (≥2 means user came back to the SAME episode after a completion) | Phase 5 (G-02) |
| `watch_progress` | `dropped_off_at` | seconds-into-episode at which the user closed the tab without completing — strongest "not for me" signal | Phase 5 (G-01) |
| `anime_list` | `score`, `status`, `episodes` | explicit user rating + status + canonical "watched episodes" derived from `watch_progress.completed` | pre-existing + Phase 3 |
| `anime_genres` | `(anime_id, genre_id)` | content metadata for content-based filtering | pre-existing |
| `combo_resolve_total` (Prom) | `tier, language, anon, player` | denominator for combo_override_total; identifies how many auto-picks landed | Phase 1 |
| `combo_override_total` (Prom) | `tier, dimension, language, anon, player` | numerator: which auto-picks the user rejected (and on which axis) | Phase 1 |
| `tier2_thin_signal_skip_total` (Prom) | `anon` | how often the Tier 2 confidence floor declined to lock — proxy for "this user has too thin history to personalize" | Phase 6 |

## What's still missing for REC-01 ("Because you watched X")

REC-01 is "show this user N anime they're likely to enjoy, given a specific
anchor anime they recently completed." A useful first version needs:

### 1. Item-item similarity matrix (computed offline)

We currently have `(user_id → anime_id → completed)` rows but no
precomputed similarity. The cheap-and-cheerful baseline is:

```
sim(A, B) = cosine(users_who_completed_A, users_who_completed_B)
```

**Required:**
- A nightly batch job (cron pod or scheduler service tick) that materializes
  a `recs_anime_similarity` table — `(anime_id_a, anime_id_b, score, computed_at)`
- Top-K cap per anchor (K=20 is enough for a "Because you watched" rail)
- Filter to community-public titles (respect existing `public_statuses` for the anchor user, but the offline job is privacy-safe because anime_id is not user-scoped)

**No new schema on read path** — the player frontend just reads
`recs_anime_similarity` via a thin catalog endpoint.

### 2. User → recently-completed anchor list

We have `watch_progress.completed=true` rows. Picking the anchor for
REC-01 needs:

```sql
SELECT anime_id, MAX(updated_at) AS finished_at
FROM watch_progress
WHERE user_id = ? AND completed = true
GROUP BY anime_id
ORDER BY finished_at DESC
LIMIT 5
```

**Required:** Nothing new. This is already query-able. A service method that
returns the top-N most recently completed anime would slot into the existing
`PreferenceService` cleanly.

### 3. Negative-signal filter (don't recommend what the user dropped)

The Phase 5 `dropped_off_at` column lets us EXCLUDE titles a user
abandoned. The existing logic doesn't use it.

**Required:**
- Define "dropped" semantically — proposal: `watch_progress.dropped_off_at IS NOT NULL AND completed = false AND dropped_off_at < (duration * 0.4)` (closed in the first 40% of an episode).
- Filter recommendation candidates against the user's dropped set.
- Trade-off: a user who dropped episode 1 of a sequel might still want the prequel — for v1, only filter at the SAME anime, not franchise.

### 4. Cold-start for users with thin history

The Phase 6 `tier2_thin_signal_skip_total` metric tells us roughly how many
users are below the personalization floor. A REC-01 cold-start fallback:

- For users with `watch_history` total weighted < 1800 (the same Tier 2
  floor), serve "globally trending this week" instead of personalized
  similarity.
- Keep the same component shape so the frontend doesn't branch.

**Required:** A `recs_trending` table populated by the same nightly job —
top-K anime by completed-rate within the last 7 days, segmented by language.

### 5. Read-path latency budget

REC-01 surfaces on Home (must hit p95 < 100ms for above-the-fold render).
The materialized similarity table makes this trivial: a single indexed
`SELECT ... WHERE anime_id_a IN (?...) ORDER BY score DESC LIMIT 20`.

**Required:** Index on `(anime_id_a, score DESC)`.

## What's still missing for REC-02 ("Similar to X")

REC-02 is the same surface but anchored on the anime currently being
viewed, not on the user's recent history. So:

### What's reusable from REC-01

- The `recs_anime_similarity` table is the same dataset.
- The cold-start fallback is different — when no similarity rows exist
  (e.g., a brand-new anime with no completion data), fall back to genre-
  based content similarity using `anime_genres`.

### What's specific to REC-02

- **No user_id required** — REC-02 is the same for every viewer of the same
  anime. Caches well at the catalog layer.
- **Must respect VAL-02 boundary** — recommendations should not cross
  language unless the user has explicitly mixed-language history. For v1,
  show the rec rail in the user's coarse-signal language (Tier 2 lock) when
  that lock exists; otherwise show all.

## What we INTENTIONALLY do NOT capture (privacy)

- **No per-user "skipped recommendation" event.** The override-rate metric
  already measures user dissatisfaction; an explicit "user dismissed rec X"
  click would invite cardinality blowup with little signal-to-noise gain.
  Re-evaluate if rec acceptance rate becomes a primary KPI.
- **No watch-position-by-anime time series.** `dropped_off_at` is one row
  per (user, anime, ep). A trajectory table was deferred in Phase 2 (G-03,
  G-05) and remains deferred.
- **No "user marked anime as not-interested" surface.** Listed in v2
  backlog as REC-03; outside C-04 scope.

## Build order suggestion (if the next milestone picks up rec work)

1. **Build the offline job first.** A read-only nightly process that writes
   `recs_anime_similarity` and `recs_trending`. Zero impact on hot path.
2. **Ship REC-02 next.** Anchored on currently-viewed anime, no user
   personalization in v1, easy to cache, hard to embarrass.
3. **Ship REC-01 last.** Needs per-user query on top of the offline tables;
   the latency budget tightens because Home is rendered for every visit.

## Cross-references

- Phase 2 audit: `docs/analytics-audit-2026-04-28.md` — original gap list
- Phase 5 SUMMARY: `.planning/phases/05-analytics-gap-fill/05-01-SUMMARY.md`
  — what was actually added vs deferred
- Phase 6 SUMMARY: `.planning/phases/06-tier-2-rewrite/06-01-SUMMARY.md` —
  the tier 2 weights documented here
- ROADMAP §"Out of Scope": REC-01, REC-02, REC-03 listed as v2 backlog
- PROJECT.md C-04: "Document downstream readiness — what additional
  capture would unlock a future recommendations engine, but do not build
  that engine here"

## Explicit non-build statement

**No recommendations engine is built in this phase.** This document is the
shopping list. Phase 8 ships the document; rec implementation is a separate
milestone with its own scoping, planning, and review.
