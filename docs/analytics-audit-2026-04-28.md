# Analytics Audit — Smart Watch Picker Overhaul

**Audit date:** 2026-04-28
**Status:** Phase 2 deliverable — locked input to Phase 5 (gap-fill), Phase 6 (Tier 2 rewrite), Phase 8 (recs readiness)
**Scope:** Read-only investigation. No schema or code changes ship in this audit.
**Audience:** `gsd-phase-researcher` and `gsd-planner` for Phase 5/6/8.

> Citations point at `file:line` ranges captured during this audit pass.
> All empirical row counts came from `SELECT`-only queries against the production
> `animeenigma` database on 2026-04-28.

## Executive Summary

- 4 player-domain tables audited: `watch_history` (127 rows, 7 users), `watch_progress` (385 rows, 13 users, **0 with `completed=true`**), `anime_list` (2569 rows, 11 users), `reviews` (111 rows).
- 5 gaps for smart episode selection scored on (value × inverse-risk); top 3 LOCKED as Phase 5 input shopping list (G-02 rewatch, G-04-lite session_id, G-01 drop-off). G-03 trajectory and G-05 intro/outro skip explicitly DEFERRED with rationale.
- 5 hygiene items found incidentally (ghost columns, read-orphan endpoints, index drift, write-only fields, the `completed=0/385` bug). Out of scope for Phases 5-8; documented for milestone backlog consideration.
- Strongest existing rec-engine signals identified for Phase 8 use; weakest signals (`is_rewatching`, `priority`, `notes`, `tags`) flagged as not actionable.

## Cross-References

- `.planning/PROJECT.md` — overall project context, validated/active requirements, key decisions
- `.planning/REQUIREMENTS.md` — this audit closes C-01 (column inventory) and C-02 (gap analysis); C-03 (gap-fill) is Phase 5
- `.planning/ROADMAP.md` §"Phase 2: Analytics Audit" — success criteria
- `.planning/phases/01-instrumentation-baseline/01-CONTEXT.md` — instrumentation already shipped
- `.planning/PROJECT.md` §"Loki retention constraint" — Loki retention is **168h / 7 days** (not 31d as Phase 1 CONTEXT D-06 incorrectly stated). Per-event analytics windows beyond 7 days require a Phase 5 schema-add, not Loki tuning.

## Methodology

1. Read the canonical domain models in
   `services/player/internal/domain/watch.go` (lines 30–103).
2. Cross-referenced every column against handler / service / repo writers and
   readers in `services/player/internal/{handler,service,repo}/`.
3. Cross-referenced frontend consumers in `frontend/web/src/{api,views,stores,components,composables}/`.
4. Compared the GORM model against `\d <table>` from the live DB to find drift
   (ghost legacy columns, missing indexes).
5. Pulled empirical row populations to gauge whether a column is "really used"
   beyond just being written.

## Column Inventory

### watch_history (127 rows, 7 distinct users, 28 unique user×anime pairs)

| Column | Type | Written by | Read by | Usage | Unused? |
|---|---|---|---|---|---|
| `id` | uuid PK | DB default `gen_random_uuid()` (`services/player/internal/domain/watch.go:72`) | not read by name | row identity | No |
| `user_id` | uuid | `services/player/internal/service/list.go:223–234` (constructs `WatchHistory`) → `services/player/internal/repo/preference.go:95` (Create) | `services/player/internal/repo/history.go:21` (`GetByUser`); `services/player/internal/repo/preference.go:47,64` (Tier 2 aggregation) | scope rows to a user | No |
| `anime_id` | uuid | same as above | `services/player/internal/repo/preference.go:78` (Tier 3 aggregation, by anime) | scope rows to an anime | No |
| `episode_number` | bigint | `services/player/internal/service/list.go:226` | not read | episode that the watch row represents | **Yes — write-only** (no reader queries on `episode_number`) |
| `player` | varchar(20) NOT NULL | `services/player/internal/service/list.go:227` | `services/player/internal/repo/preference.go:46,63,77` (Tier 2/3 GROUP BY) | combo dimension | No |
| `language` | varchar(5) NOT NULL | `services/player/internal/service/list.go:228` | same as `player` | combo dimension; lock signal | No |
| `watch_type` | varchar(5) NOT NULL | `services/player/internal/service/list.go:229` | same | combo dimension; lock signal | No |
| `translation_id` | varchar(50) | `services/player/internal/service/list.go:230` | `services/player/internal/repo/preference.go:77` (Tier 3 only) | fine-grained team key (e.g. Kodik translation_id) | Partially — Tier 2 GROUP BY uses `translation_title`, not `translation_id` (`services/player/internal/repo/preference.go:46,63`) |
| `translation_title` | varchar(200) | `services/player/internal/service/list.go:231` | `services/player/internal/repo/preference.go:46,63,77` | team display name; primary fine-pick key | No |
| `duration_watched` | bigint default 0 | `services/player/internal/service/list.go:218–222,232` (snapshot of `watch_progress.progress` at mark-watched time) | **none** at query time — written on every history insert but no reader uses it | intended for Tier 2 weighting per ROADMAP Phase 6 SC-1 | **Yes — write-only today**; Phase 6 promises to consume it |
| `watched_at` | timestamptz default now | DB default | `services/player/internal/repo/history.go:23` (ORDER BY) | recency for `/users/history` | No (but only the history endpoint reads it; see "orphan endpoint" note below) |

**Domain drift / observations:**
- The model declares composite indexes `idx_wh_user_combo` and `idx_wh_anime_combo` (`services/player/internal/domain/watch.go:73,74,76,77,78`) but only `idx_watch_history_user_id` and `idx_watch_history_anime_id` exist on the live DB. AutoMigrate did not add the composite indexes after they were declared. Tier 2/3 GROUP BY queries are running without the index they were designed for.
- `services/player/internal/handler/history.go:25` exposes `GET /users/history` and `services/player/internal/transport/router.go:74` routes it, but **no frontend consumer** calls `userApi.getWatchHistory` (the only reference is the API client stub at `frontend/web/src/api/client.ts:213`). The endpoint is reachable but UI-orphan.
- Combo distribution from live DB: 60 kodik/ru/dub, 47 kodik/ru/sub, 17 hianime/en/sub, 2 consumet/en/sub, 1 animelib/ru/dub. So 84 % of all history rows come from Kodik (which has *no* video event access — see CLAUDE.md line "Kodik = iframe only, no video event access"). That severely limits the meaningfulness of `duration_watched` for the dominant cohort.
- `duration_watched > 0` populated for 122 / 127 rows — backend is reliably snapshotting from `watch_progress`, the value is just unread.

---

### watch_progress (385 rows, 13 users, 0 with completed=true)

| Column | Type | Written by | Read by | Usage | Unused? |
|---|---|---|---|---|---|
| `id` | uuid PK | DB default `uuid_generate_v4()` (`services/player/internal/domain/watch.go:31`) | not read by name | row identity | No |
| `user_id` | uuid | `services/player/internal/service/progress.go:29` (`UpsertAnimePreference` builds row) | `services/player/internal/repo/progress.go:44,53` | scope to user | No |
| `anime_id` | uuid | `services/player/internal/service/progress.go:30` | same | scope to anime | No |
| `episode_number` | bigint | `services/player/internal/service/progress.go:31` | `services/player/internal/repo/progress.go:53` (`GetByUserAnimeEpisode`); `services/player/internal/repo/progress.go:45` (ORDER BY) | per-episode key | No |
| `progress` | bigint default 0 | `services/player/internal/service/progress.go:32` (raw seconds) | `services/player/internal/service/list.go:218–222` (snapshot into history.duration_watched on mark-watched); also implicitly read by clients via the orphan `GET /progress/{animeId}` endpoint | seconds played in episode | Partially — written from all four players (`KodikPlayer.vue:275,689`, `AnimeLibPlayer.vue:666`, `HiAnimePlayer.vue:1229`, `ConsumetPlayer.vue:1089`); GET endpoint exists but **no frontend reader** uses it (resume CTAs read `localStorage[watch_progress:<animeId>]` instead — see `frontend/web/src/views/Anime.vue:745`, `frontend/web/src/components/player/AnimeLibPlayer.vue:655` etc). Server is the write-only sink. |
| `duration` | bigint | `services/player/internal/service/progress.go:33` (kept as `GREATEST(existing, new)` per `services/player/internal/repo/progress.go:33`) | server-side: not read; effectively only useful via the orphan GET | episode total length | Same partial-orphan pattern as `progress` |
| `completed` | boolean default false | hard-coded `false` at `services/player/internal/service/progress.go:34` (comment: "User marks manually") and `services/player/internal/repo/progress.go:35` (set by upsert) | not read | Phase 3 in ROADMAP names this column the new single source of truth for "watched" | **Yes — column is write-only AND nobody ever sets it to true** (live DB: 0/385 rows true). Phase 3 will fix this. |
| `last_watched_at` | timestamptz default now | `services/player/internal/repo/progress.go:23,35` | not read at SQL level | recency | Effectively yes |
| `created_at` | timestamptz | `services/player/internal/repo/progress.go:26` (only on insert) | not read | bookkeeping | Yes — write-only |
| `updated_at` | timestamptz | `services/player/internal/repo/progress.go:24,36` | not read | bookkeeping | Yes — write-only |

**Domain drift / observations:**
- Domain only declares `index` on `user_id`, `anime_id`, `episode_number`, but the live DB has **two** copies of the user index (`idx_watch_progress_user`, `idx_watch_progress_user_id`) and a unique constraint on `(user_id, anime_id, episode_number)` not declared in the model — drift in the other direction.
- 374 / 385 rows have `duration > 0` and `progress > 0`. Mean progress / duration ≈ 89 %. 325 rows are at ≥ 95 % of duration (i.e. the user effectively finished the episode in the player). **Yet `completed` is false on every single row**, which is the bug Phase 3 explicitly targets — and it means the reliability check we'd want for any analytics ("did the user finish ep N") has to be computed from `progress / duration`, not from the boolean.
- The orphan `GET /progress/{animeId}` (`services/player/internal/transport/router.go:71`, `services/player/internal/handler/progress.go:58`) means we have a server table that never returns to the client. Resume across devices is impossible today; only localStorage drives the resume CTA.

---

### anime_list (2569 rows, 11 users — completed 1952 / plan_to_watch 351 / dropped 199 / watching 51 / on_hold 16; 648 scored / 1921 unscored)

| Column | Type | Written by | Read by | Usage | Unused? |
|---|---|---|---|---|---|
| `id` | uuid PK | DB default | not read | row identity | No |
| `user_id` | uuid | `services/player/internal/service/list.go:70` | `services/player/internal/repo/list.go:54–227` (every list query); export at `services/player/internal/handler/export.go:96` | scope | No |
| `anime_id` | uuid NOT NULL | same | same | scope | No |
| `status` | varchar(20) NOT NULL default 'plan_to_watch' | `services/player/internal/service/list.go:72`; auto-promoted by `services/player/internal/repo/list.go:117–135` (`IncrementEpisodes`) | `services/player/internal/repo/list.go:93,142,155,191,198,210` (filters & stats); `services/player/internal/handler/export.go:132`; `frontend/web/src/views/Profile.vue` (filtering) | watchlist tab key | No |
| `score` | int default 0 | `services/player/internal/service/list.go:75–80` | `services/player/internal/repo/list.go:181,195`; `services/player/internal/repo/review.go:81–84,110–113` (combined-rating UNION with `reviews`); `frontend/web/src/views/Profile.vue:247–265` | user rating; **also feeds anime average rating** | No |
| `episodes` | int NOT NULL default 0 | `services/player/internal/service/list.go:81–86,200`; `services/player/internal/repo/list.go:117–135` (`IncrementEpisodes`) | `services/player/internal/repo/list.go:181,196`; `frontend/web/src/views/Profile.vue:274–296` | per-user episode count | No (but Phase 3 makes this a derived field, which is the correctness fix) |
| `notes` | text | `services/player/internal/service/list.go:87–89`; preserved-on-empty in `services/player/internal/repo/list.go:67` | not read by any frontend or other service handler | freeform user notes | **Yes — empirically 0 / 2569 rows non-empty.** Schema present, no UI reads or writes it. |
| `tags` | text | `services/player/internal/service/list.go:91–93` | not read | freeform tags | **Yes — empirically 0 / 2569 rows non-empty.** |
| `is_rewatching` | boolean default false | `services/player/internal/service/list.go:95–97`; only ever set by MAL import (`services/player/internal/handler/mal_import.go:295`) and Shikimori import (`services/player/internal/handler/shikimori_import.go:250–252`) | not read by any handler, service, or frontend file | rewatch indicator | **Yes — and it is only LIST-level, not per-episode.** Empirically 1 / 2569 true. The boolean cannot answer "is the user re-watching ep 5 right now"; it can only answer "did MAL/Shikimori say they were re-watching this series at import time". For Phase 6 weighting it is essentially noise. |
| `priority` | varchar(20) default 'medium' | `services/player/internal/service/list.go:99–101`; only ever set by MAL import (handler) | not read by any frontend file | MAL parity | **Yes — read-orphan.** 753 / 2569 set to "low" (all from MAL imports), 8 to "medium", 1808 NULL. |
| `mal_id` | int | `services/player/internal/service/list.go:103–105`; written by both imports | `services/player/internal/handler/export.go:130`; `services/player/internal/service/mal_export.go` | round-trip identity | No |
| `started_at` | timestamptz | `services/player/internal/service/list.go:108–115`; auto-set when status="watching"; auto by `IncrementEpisodes` | `frontend/web/src/views/Profile.vue:305–311,430–433`; `services/player/internal/handler/export.go:139` | history dates | No |
| `completed_at` | timestamptz | `services/player/internal/service/list.go:117–125`; auto-set when status="completed"; auto by `IncrementEpisodes` when last episode reached | same as above | history dates | No (1956 / 2569 populated) |
| `created_at`, `updated_at` | timestamptz | GORM | export + ORDER BY | bookkeeping | No |
| **Ghost columns (in DB, not in domain model):** `anime_title`, `anime_cover`, `anime_type`, `anime_total_episodes` | varchar / text / int | DB has the columns; the Go struct has no field, so AutoMigrate does **not** populate them. Live DB shows 1115 / 2569 with `anime_title` set, 1052 / 2569 with `anime_type` — populated by an older codepath that no longer runs. | not read | dead legacy | **Yes — partially populated dead columns.** |

---

### reviews (111 rows) — brief

| Column | Type | Written by | Read by | Usage | Unused? |
|---|---|---|---|---|---|
| `id` | uuid PK | DB default | n/a | identity | No |
| `user_id` | uuid NOT NULL | `services/player/internal/repo/review.go:21–36` | `services/player/internal/repo/review.go:50–56,80,111`; uniqueness | No |
| `anime_id` | uuid NOT NULL | same | `services/player/internal/repo/review.go:39–46,77,107` | scope | No |
| `username` | varchar(32) NOT NULL | `services/player/internal/repo/review.go:31` (preserves on empty) | included in payload | display denorm | No |
| `score` | int 1..10 NOT NULL | `services/player/internal/repo/review.go:32` | `services/player/internal/repo/review.go:77,80,107,110` (combined-rating UNION with `anime_list.score`) | rating | No |
| `review_text` | text | `services/player/internal/repo/review.go:33` | `services/player/internal/handler/review.go:85,143` | review body | No |
| `created_at`, `updated_at` | timestamptz | repo | ORDER BY (`services/player/internal/repo/review.go:44,54`) | recency | No |
| **Ghost columns:** `anime_title`, `anime_cover` | varchar / text | DB has them; struct does not. 100 / 111 rows have `anime_title` populated by an older write path. | not read | dead legacy | Yes |

For recommendations: `(user_id, anime_id, score, review_text)` is the strongest user-stated-affinity signal we have. 111 rows is thin but real.

---

## Gap Analysis

Score = value-for-this-project (1–5) × inverse-of-risk-to-add (1–5). Both axes are
ordinal heuristics; the product is just to rank.

### G-01: Drop-off / abandon point within an episode

- **Value: 3** — A user who quits at minute 4 of episode 1 is a different signal
  than one who watches 22/24 minutes. Today both produce a `watch_progress` row
  with `completed=false`; we cannot distinguish "abandoned the show" from
  "paused for dinner". For Phase 6 Tier 2 weighting, abandoned watches should
  weigh near zero. For Phase 8 recs, "user abandons isekai consistently" is a
  high-signal feature.
- **Risk: 3** — We already capture `progress` and `duration` per
  `(user, anime, episode)` (`services/player/internal/domain/watch.go:30–41`).
  We do **not** capture an explicit "session ended" event with the final
  position; we only have whatever progress the player happened to flush last.
  Backend cost is moderate: either a new `last_position_seconds` snapshot column
  or an `events`-style table, plus `sendBeacon` from the player on
  unload (`KodikPlayer.vue:689` already does this).
- **Score: 9.** Every player already pings progress; adding "this was the final
  ping for this session" is mostly client-side discipline.
- Citations: `services/player/internal/domain/watch.go:30–41`,
  `services/player/internal/repo/progress.go:21–39`,
  `frontend/web/src/components/player/KodikPlayer.vue:262–331,676–693`,
  `frontend/web/src/components/player/HiAnimePlayer.vue:1218–1229`,
  `frontend/web/src/components/player/ConsumetPlayer.vue:1078–1089`,
  `frontend/web/src/components/player/AnimeLibPlayer.vue:655–666`.
- **Especially valuable for recs (Phase 8): yes.** Drop-off rate per genre /
  studio / season is a textbook collaborative-filtering negative signal.

### G-02: Rewatch detection (per-episode, not per-list)

- **Value: 4** — Phase 6 wants to weight history by `duration_watched` and
  decay it. A rewatch-of-favorite is double-counting unless we know it is a
  rewatch. The existing `is_rewatching` boolean on `anime_list`
  (`services/player/internal/domain/watch.go:57`) is **list-level only** and
  in production has 1 / 2569 rows true. It also only reflects the import-time
  state from MAL/Shikimori — there is no UI to flip it
  (`grep "is_rewatching"` returns zero frontend matches outside the type).
- **Risk: 2** — The signal can be *derived* without any new column: a second
  `watch_progress` row for `(user, anime, ep)` after the first `completed=true`
  is, definitionally, a rewatch. Once Phase 3 fixes `completed`, no schema
  change is required at all — only a query. Even cheaper: a `watch_count int`
  on `watch_progress` incremented when an episode is re-finished.
- **Score: 8.** Cheapest of all gaps because Phase 3 produces the precondition
  (a reliable `completed` boolean) for free.
- Citations: `services/player/internal/domain/watch.go:30–41,47–67`,
  `services/player/internal/handler/mal_import.go:295`,
  `services/player/internal/handler/shikimori_import.go:250–252`,
  `services/player/internal/service/list.go:95–97`.
- **Especially valuable for recs: yes.** Rewatches are the strongest possible
  positive signal for "user loved this".

### G-03: Completion-percentage trajectory per episode

- **Value: 3** — Tells us "user finishes 95 %+ of episodes" vs "skips the last
  10 %" (credits-skipper) vs "drops at 60 %" (lost interest). Today
  `watch_progress` records the *final* position as a single number; we have no
  trajectory across re-opens.
- **Risk: 4** — Schema-wise, the *aggregate* (final / max progress) is already
  there. The actual trajectory (samples through time) requires a new
  append-only `progress_events` table, which is much heavier than column
  additions and doubles the write traffic on every player heartbeat. The
  90th-percentile case ("did they finish?") is fully addressable today by
  comparing `progress` against `duration`; the 99th-percentile case
  (full trajectory) is expensive.
- **Score: 12 — but the value is mostly already captured by the cheap
  derivation `progress / duration > 0.9`.** The expensive trajectory variant
  is poor ROI.
- Citations: `services/player/internal/repo/progress.go:21–39`. Empirically
  325 / 385 live rows are at ≥ 95 % of duration; 343 at ≥ 50 %; mean
  progress/duration ≈ 89 %. The simple ratio already tells most of the story.
- **Especially valuable for recs: medium.** "User completes shows" is a
  binary feature; the curve adds little.

### G-04: Session length

- **Value: 2** — Distinguishes a binge session (5 episodes back-to-back) from
  scattered viewing. Mostly useful for understanding *user behaviour* and for
  the resume state machine (Phase 4) which wants to know if a user is "in a
  binge" so it can preselect ep N+1 versus ep N.
- **Risk: 4** — Requires a `session_id` on every progress save and a
  client-side session start/end heuristic (e.g. "no progress event for 30 min
  → new session"). New cookie / localStorage on the client + new column on
  `watch_progress` (and probably `watch_history`).
- **Score: 8.** Phase 5 already names a smaller version of this:
  *"distinguish session-start from session-resume on every progress save"*
  (ROADMAP Phase 5 SC-2). That single bit is most of the value here for far
  less cost.
- Citations: ROADMAP Phase 5 SC-2;
  `frontend/web/src/components/player/KodikPlayer.vue:271–283` (every save is
  currently context-free).
- **Especially valuable for recs: low.** Session length per se does not move
  recs.

### G-05: Intro / outro skip patterns

- **Value: 1** — "User skips intros" is a binge re-watch tell, but it is the
  weakest signal in this list. Two of our four players (Kodik, AnimeLib's
  Kodik fallback) cannot detect skip events at all because they are iframe-only
  (CLAUDE.md "Kodik = iframe only, no video event access"). Coverage is at
  most ~16 % of current rows (Hianime + Consumet + AnimeLib MP4).
- **Risk: 4** — Requires (a) chapter / intro-marker metadata per episode (we
  do not have this), (b) per-event capture distinct from `progress`, (c) a
  new write path. New table or new event column. Large surface area.
- **Score: 4.** The combination of partial player coverage and missing
  upstream chapter metadata makes this expensive for a thin signal.
- Citations: CLAUDE.md "Kodik = iframe only";
  `frontend/web/src/components/player/KodikPlayer.vue` and
  `AnimeLibPlayer.vue` (no `seek` / `chapter` events captured anywhere); live
  DB: 84 % of WH rows are Kodik.
- **Especially valuable for recs: very low.**

---

## Phase 5 Candidate Lock

The following 3 gaps are LOCKED as the input shopping list for **Phase 5
(Analytics Gap-Fill)**. Phase 5 must address all three or downgrade with
explicit justification recorded in its CONTEXT.md.

### Locked

1. **G-02: Per-episode rewatch detection.** Implement as
   `watch_progress.watch_count INT NOT NULL DEFAULT 0`, incremented when an
   episode transitions from `completed=false → true` and an existing row is
   found. Hard-depends on Phase 3 (a reliable `completed` boolean).
   Effort: tiny — one column add and one branch in the upsert. Highest
   single-feature value: unblocks Phase 6 weighting (rewatches count once,
   not N times) AND the strongest positive rec-engine signal.

2. **G-04-lite: Session-start vs session-resume bit.** Implement as
   `watch_progress.session_id UUID NULLABLE` (and same column on
   `watch_history`), generated client-side on first save in a session,
   re-used on subsequent saves within 30 min idle, regenerated after idle.
   Roadmap Phase 5 SC-2 names this directly. Effort: small (one nullable
   uuid column on each table; client-side session-id helper). Unlocks
   Phase 6 ("a single mega-binge no longer locks the wrong combo for
   everyone"). Cheapest path to most of G-04's value without full
   session-tracking apparatus.

3. **G-01: Drop-off / abandon point.** Implement as a final-flush beacon on
   player unload (extend the `KodikPlayer.vue:689` `navigator.sendBeacon`
   pattern to all four players); abandoned-derivation:
   `progress / duration < 0.5` AND no follow-up history row at higher
   `episode_number`. Zero-schema if we treat abandoned as derived; one
   optional `abandoned BOOL` column if pre-computation matters for
   query-time performance. Unlocks Phase 6 (down-weight abandoned watches)
   and Phase 8 (negative rec signal).

### Deferred

- **G-03 (trajectory):** the cheap derivation `progress / duration ≥ 0.9`
  already captures the 90 % case. Full trajectory requires an append-only
  events table that doubles player write traffic — poor ROI.
- **G-05 (intro/outro skip):** ~84 % of rows are Kodik (iframe-only, no
  skip events possible) and we lack upstream chapter metadata. Signal-to-
  noise too low for the engineering cost.

---

## Cleanup / Hygiene Items (Out of Scope for Phases 5–8)

Found incidentally while auditing. These are NOT inputs to Phase 5/6/8 — they
are recommended for milestone backlog consideration. Filing here so they do not
get lost.

- **Ghost columns in `anime_list`:** `anime_title`, `anime_cover`, `anime_type`,
  `anime_total_episodes`. 1115 / 2569 rows have `anime_title` populated by an
  old write path the current Go code does not exercise. Per CLAUDE.md "GORM only
  creates new tables/columns, it does NOT modify or drop existing columns" this
  is invisible to AutoMigrate. Recommend explicit migration to drop.
- **Ghost columns in `reviews`:** `anime_title`, `anime_cover`. Same disposition.
- **Read-orphan endpoints:** `GET /users/history`
  (`services/player/internal/transport/router.go:74`) and
  `GET /users/progress/{animeId}` (line 71) are both reachable but no frontend
  consumer calls them. The watch-progress orphan is the more interesting one
  because it forces every resume CTA to read `localStorage` (`Anime.vue:745`),
  which breaks cross-device resume. **ROADMAP Phase 7 SC-3 already names
  cross-device-staleness as in-scope** — Phase 7 should consider wiring the
  GET `/progress/{animeId}` endpoint into the resume flow rather than leaving
  it orphaned.
- **Domain-vs-DB index drift on `watch_history`:** model declares
  `idx_wh_user_combo` and `idx_wh_anime_combo` composite indexes
  (`services/player/internal/domain/watch.go:73,76,77,78`); live DB has only
  the simpler `idx_watch_history_user_id` and `idx_watch_history_anime_id`.
  AutoMigrate did not retroactively add the composites. **Tier 2/3 GROUP BY
  queries in `repo/preference.go:42–80` are running without their intended
  index.** Phase 6 should benchmark the rewrite both with and without these
  composites; if performance is acceptable without, drop the model
  declarations to remove the lie.
- **Write-only fields:** `notes`, `tags`, `is_rewatching`, `priority` on
  `anime_list` are settable via API but never displayed in the UI. Either ship
  the Profile UI for them (already half-built — Profile already shows
  `started_at`/`completed_at`) or stop accepting them at the API. **`is_rewatching`
  specifically should be replaced by the per-episode G-02 signal** — once G-02
  ships, the list-level boolean is strictly weaker and can be removed.
- **`watch_progress.completed = false` on every row:** 0 / 385 rows true.
  ROADMAP Phase 3 will fix this by making the auto-mark + manual-mark paths
  both set `completed=true`. This is not a hygiene item — it is the headline
  bug for Phase 3.

---

## Notes for Phase 8 (Recommendations Readiness Documentation)

### Especially-valuable existing signals for a "because you watched X" engine

- **`reviews.score` + `anime_list.score`** — combined via the UNION pattern at
  `services/player/internal/repo/review.go:75–87,105–117`. This is the strongest
  user-stated affinity signal; 648 + 111 = 759 distinct (user, anime, score)
  triples even today.
- **`watch_progress.progress / duration` ratio** — implicit completion signal,
  reliable for HiAnime / Consumet / AnimeLib MP4 (~16 % of rows but with full
  player event access). For Kodik (~84 %) progress quality is weaker because
  the iframe boundary limits event fidelity.
- **`anime_list.status` transitions** — the `activity_events` table
  (`services/player/internal/repo/activity.go`) already records `status_change`
  events with old/new values when entries change status
  (`services/player/internal/service/list.go:148–164`). That is a free
  trajectory log Phase 8 should not overlook.
- **Combo signal (`watch_history.player + language + watch_type`)** — useful as
  a *user clustering* feature ("EN-sub HiAnime users") even if it is not a
  direct affinity signal.

### Especially-valuable signals to *add* for recs (i.e. close in Phase 5 — already locked above)

- **G-02 rewatch flag** (per-episode `watch_count`) — strongest possible positive
  affinity signal.
- **G-01 abandoned flag** — strongest possible negative affinity signal.
- A `session_id` per `watch_progress` save (G-04-lite) so a binge can be
  collapsed into one signal instead of N.

### Especially-valuable signals to *not* bother with for recs

- **`is_rewatching` boolean on `anime_list`** — list-level, never updated via
  UI, 1 / 2569 populated. Drop or replace with the per-episode signal in G-02.
- **`priority`, `notes`, `tags`** — none are read or displayed; only `priority`
  is auto-populated by MAL import. Not actionable as a rec feature.
- **G-05 intro/outro skip** — coverage too low to be worth the engineering
  cost.

---

*Audit closed 2026-04-28. Closes REQUIREMENTS.md C-01 and C-02. Inputs to
Phase 5 (locked candidates above), Phase 6 (Tier 2 design), Phase 8 (recs
readiness).*
