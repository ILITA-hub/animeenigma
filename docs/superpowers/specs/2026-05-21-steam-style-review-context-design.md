# Steam-style review context — design spec

**Date:** 2026-05-21
**Origin:** Telegram feature request from Yegor Bankir (20/05/2026 22:33)
> "When a user writes a review, we need to save which episode they're on and
> their watch status of the anime. Like on Steam. So we can immediately spot
> the assholes who haven't actually watched the anime."

## Problem

The anime review card on `/anime/:id` currently shows username, date,
score, and review text. Nothing tells you whether the reviewer actually
watched the show. A 1/10 review from a user who never opened episode 1
reads identically to a 1/10 from a user who finished the series — same
credibility signal, zero useful context.

Steam solved this with "X hours on record" next to every review. Episodes
watched is our equivalent.

## Goal

Render two pieces of context inline next to each review's date:

1. **Episode progress** — `episodes_watched / episodes_total`
2. **Watch status** — Watching / Completed / Dropped / On hold / Plan to watch

Goal is visual credibility cue, not enforcement. Anyone can still review
without watching; the card just makes that legible at a glance.

## Non-goals (v1)

- Filtering / sorting reviews by completion %
- Blocking review submission for `plan_to_watch` users
- Time-travel snapshot (showing "what they had watched when they reviewed")
  — values are **live**, not snapshotted
- Anonymizing or hiding the badge on the reviewer's own card

## Design decisions

### Live values, no new columns

`anime_list` already carries `status` and `episodes` on the same row that
absorbs the review (post-Phase 1 social merge — see `domain/watch.go:58`).
The frontend already gets `anime.episodes_count` inlined via the
`AnimeInfo` projection on each review response.

We expose those existing fields through the wire shape. No DB migration,
no backfill, no snapshot drift.

**Trade-off accepted:** if a user reviews at episode 3 then finishes the
show, the badge updates to show the new state. This is the Steam
behavior ("X hrs on record" keeps climbing after you review) and matches
user intuition.

### Wire shape change

`services/player/internal/handler/review.go`:

```go
type reviewResponse struct {
    ID         string            `json:"id"`
    UserID     string            `json:"user_id"`
    AnimeID    string            `json:"anime_id"`
    Username   string            `json:"username"`
    Score      int               `json:"score"`
    ReviewText string            `json:"review_text"`
    CreatedAt  time.Time         `json:"created_at"`
    Anime      *domain.AnimeInfo `json:"anime,omitempty"`
    // New — exposed live from anime_list row, NOT snapshotted.
    Status   string `json:"status"`   // watching/completed/dropped/on_hold/plan_to_watch
    Episodes int    `json:"episodes"` // count the reviewer says they've watched
}
```

`review_shape_test.go` updated to assert the two new fields are present
(removing the "exactly 7 fields" assertion, asserting the new 9-field
shape instead). The SOCIAL-NF-01 contract intent — "no internal fields
leak unintentionally" — is preserved: these two are intentional adds,
not accidental leakage of `notes` / `tags` / `mal_id`.

### UI: inline with date, Steam-style

```
┌──────────────────────────────────────────┐
│ (👤) Bankir_E                    ★ 8     │
│      May 20, 2026 · 📺 3/12 · Watching   │
│                                          │
│ "Great show, dropped at episode 3        │
│  because the pacing fell apart..."       │
└──────────────────────────────────────────┘
```

Single line under the username, three dot-separated segments:
`{date} · 📺 {watched}/{total} · {status_label}`.

Rendering rules:

| Case | Render |
|---|---|
| `episodes_count > 0` | `📺 {episodes}/{episodes_count} · {status}` |
| `episodes_count == 0` (ongoing, unknown total) | `📺 {episodes} eps · {status}` |
| `status == 'plan_to_watch'` | `📺 {episodes}/{total} · Plan to watch ⚠️` — amber/red tint (⚠️ driven by status, not episode count) |
| `episodes == 0 && status != 'plan_to_watch'` | `📺 0/{total} · {status} ⚠️` — same flag treatment (driven by episode count) |

Status strings reuse existing `watchlist.*` i18n keys (`en.json:399-403`):
`Watching`, `Plan to Watch`, `Completed`, `On Hold`, `Dropped`. RU/JA
already covered.

The ⚠️ icon and amber tint are CSS-only — the card stays the same height,
no layout shift, no separate component needed.

### Rewatch context — TODO, not in v1

`anime_list.is_rewatching` (bool) and `WatchProgress.watch_count` (1 =
first watch, 2+ = rewatch) already exist. A future enhancement could
render "🔁 On rewatch" or "🔁 2nd watch" as a 4th segment.

Reason for deferring: the rewatch signal is mostly *positive* credibility
("they liked it enough to watch again"), whereas v1 is specifically
about catching *low-credibility* reviewers. The visual treatment for the
two is different and worth designing separately.

A code comment in `handler/review.go` next to the new fields will link
back to this spec so the next implementer has the context.

### Passive-watcher false negative — TODO, not in v1

Edge case: some users watch the full series on AnimeEnigma (logged in
`watch_history` per episode) without ever editing their `anime_list`
status or episode count. Their `anime_list.episodes` stays at the
default `0`, so their review card would falsely read "📺 0/12 · Watching ⚠️".

A v1.1 fix would replace `anime_list.episodes` with
`max(anime_list.episodes, COUNT(DISTINCT episode_number) FROM watch_history WHERE user_id=$1 AND anime_id=$2 AND completed=true)`
in the review query. Adds a subquery per review render — acceptable cost
if we see real complaints, but not worth paying speculatively.

Same TODO-with-spec-link treatment in code.

## Files changed

### Backend

- `services/player/internal/handler/review.go` — extend `reviewResponse`,
  update `toReviewResponse` projection, add the two TODO comments
- `services/player/internal/handler/review_shape_test.go` — assert the
  9-field shape (was 7)

### Frontend

- `frontend/web/src/views/Anime.vue:736-767` — extend the review card to
  render the inline metadata line
- `frontend/web/src/locales/{en,ru,ja}.json` — add
  `anime.reviewStats.watched` / `watchedOpen` / `noProgress` strings
  (status labels reused from existing `watchlist.*` keys)

### Ops

- `frontend/web/public/changelog.json` — user-facing entry in the
  enthusiastic-with-emojis tone the project uses

## Metrics (per CLAUDE.md convention)

- **UXΔ = +2 (Better)** — review credibility legible at-a-glance.
  Mild downside: passive-watchers (no list update) get a false-negative
  ⚠️ badge until v1.1 fallback ships.
- **CDI = 0.01 * 5** — single wire-shape extension, one card-section
  template change, three locale-file additions. No new tables, no
  migration, no cross-service refactor.
- **MVQ = Sprite 75%/85%** — small playful detail that surfaces hidden
  truth. High slop-resistance: signal is mechanical (live DB columns),
  not heuristic.

## Future enhancements (linked from code TODOs)

1. **Rewatch badge** — render `is_rewatching` / `watch_count` as a 4th
   segment with positive treatment ("🔁 On rewatch")
2. **watch_history fallback** — fix the passive-watcher false-negative
   by merging `anime_list.episodes` with `watch_history` completed-episode
   count
3. **Per-card filter** — "Hide reviews from users with < N episodes
   watched" toggle above the reviews list (Steam has a similar slider)
