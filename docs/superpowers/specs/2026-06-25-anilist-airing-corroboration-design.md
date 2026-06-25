# AniList Airing Corroboration — Design Spec

**Date:** 2026-06-25
**Status:** Approved (design) — pending implementation plan
**Topic:** Corroborate Shikimori's next-episode date against AniList for ongoing anime, adopting AniList's date when it is later.

---

## 1. Problem

Shikimori's calendar (`/api/calendar`) returns a `next_episode_at` that is effectively **"last-aired + 1 week"**. It does **not** model broadcast hiatuses. For ongoing shows that go on break, our stored next-episode date drifts wrong.

Observed case: **Re:Zero Season 4, episode 12.**

- Our catalog (Shikimori-sourced): **2026-07-01**
- AniList (broadcaster schedule): **2026-08-12** (ep13 → 08-19, etc.)
- Reality: AniList is correct; Shikimori ignored the broadcast gap.

Two consumers depend on `NextEpisodeAt` being right:

1. **Next-episode ordering** in browse/calendar surfaces.
2. **New-episode notifications** — the notifications detector (`services/notifications`) reads episode timing through catalog; a wrong date means mistimed "new episode" pushes.

AniList tracks broadcaster `nextAiringEpisode` schedules and models hiatuses. We already map Shikimori → AniList (`libs/idmapping`), so the plumbing is mostly in place.

## 2. Goal & Non-Goals

**Goal:** During catalog calendar sync, corroborate each **ongoing** anime's Shikimori next-episode date against AniList's `nextAiringEpisode.airingAt`, and adopt AniList's date **only when it is later** than Shikimori's. Record which source won.

**Non-Goals:**

- No UI change. This is a data-correctness fix to an existing field (`next_episode_at`).
- Not surfacing the discrepancy to users (deferred alternative — out of scope here).
- Not changing the per-anime ID-mapping callers of `libs/idmapping`.
- Not adopting AniList for any other metadata (titles, posters, scores) — airing date only.
- No new cron / scheduler job. Reconciliation rides the existing calendar sync.

## 3. Architecture & Data Flow

```
scheduler CalendarSyncJob ──POST /api/anime/calendar-sync──▶ catalog.SyncCalendar
                                                                  │
                              Shikimori /api/calendar ───────────┤  S = naive date, per ongoing anime
                                                                  │
                              idmapping.AniListAiringByMALID ─────┤  A = nextAiringEpisode.airingAt
                              Media(idMal:shikimoriID){status,        (single GraphQL call; idMal == shikimoriID)
                                nextAiringEpisode{episode,airingAt}}
                                                                  ▼
                                          reconcile (later-wins): A > S ? → write A + source=anilist
                                                                            else → keep S + source=shikimori
                                                                  ▼
                                          Anime.NextEpisodeAt / Anime.NextEpisodeSource
                                          (consumed by next-episode ordering + notifications detector)
```

Reused infrastructure:

- **Shikimori IDs == MAL IDs** (`libs/idmapping/client.go`), so AniList's `Media(idMal:…)` selector takes our Shikimori id directly.
- AniList returns `nextAiringEpisode` in the **same single call** as the id lookup — no extra hop, and **ARM is not involved** (ARM has no airing data).
- The AniList HTTP client lives in `libs/idmapping` with **IPv4-forced transport + per-call timeout + egress-recording wrap**, already wired into catalog DI at `services/catalog/cmd/catalog-api/main.go:487`.

## 4. `libs/idmapping` — new airing method

New exported method on `Client`. It does **not** alter the existing `resolveAniList` ID-mapping path; factor the shared GraphQL-POST mechanics into a small private helper reused by both.

```go
type AniListAiring struct {
    AniListID    int        // AniList Media.id
    Status       string     // RELEASING | FINISHED | NOT_YET_RELEASED | CANCELLED | HIATUS
    NextEpisode  int        // nextAiringEpisode.episode; 0 if none scheduled
    NextAiringAt *time.Time // from nextAiringEpisode.airingAt (unix seconds → UTC); nil if none
}

// AniListAiringByMALID queries AniList for the airing schedule by MAL/Shikimori id.
//   (result, nil) — Media found
//   (nil, nil)    — no Media for that id, or no upcoming episode scheduled
//   (nil, err)    — transport / GraphQL error
func (c *Client) AniListAiringByMALID(ctx context.Context, malID string) (*AniListAiring, error)
```

GraphQL query:

```graphql
query ($mal: Int) {
  Media(idMal: $mal, type: ANIME) {
    id
    status
    nextAiringEpisode { episode airingAt }
  }
}
```

`airingAt` is unix seconds; convert with `time.Unix(airingAt, 0).UTC()`. When `Media` is null or `nextAiringEpisode` is null, return `(nil, nil)` — a clean "nothing to corroborate" signal, distinct from an error.

## 5. `catalog` — reconciliation rule (later-wins, ongoing-only)

Reconciliation is a small, independently testable helper fed through an interface so the AniList dependency is mockable (project convention: **handwritten fakes, no testify/mock**).

```go
type AniListAiringFetcher interface {
    AniListAiringByMALID(ctx context.Context, malID string) (*idmapping.AniListAiring, error)
}
```

Rule, applied per **ongoing** calendar anime. Scope gate: `CalendarEntry.Anime.Status == "ongoing"` — the field is already present on the calendar payload (`parser/shikimori/client.go` `CalendarEntry.Anime`), so **no extra DB read** is needed to scope.

1. `A := fetch(shikimoriID)`. On error, no mapping, or `A.NextAiringAt == nil` → **keep Shikimori**, `source = "shikimori"`.
2. If `A.NextAiringAt` is **strictly after** Shikimori's date (exact "any later" — no margin) → `NextEpisodeAt = A.NextAiringAt`, `source = "anilist"`.
3. Else (AniList earlier than or equal to Shikimori) → **keep Shikimori**, `source = "shikimori"`.

"Later-wins" is the safe asymmetry: Shikimori's failure mode is reporting a date that is **too early** (it ignores hiatuses), so only a *later* AniList date represents new information. An earlier AniList date is treated as noise and ignored.

Wired into **both** `SyncCalendar` write paths so reconciliation runs wherever the calendar writes the field:

- `importMissingCalendarAnime` (`catalog.go:976`) — sets `anime.NextEpisodeAt`.
- `updateExistingCalendarEpisodes` (`catalog.go:1029`) — sets `existing.NextEpisodeAt`.

## 6. Storage — `next_episode_source` column + batch-refresh guard

New field on `domain.Anime` (`services/catalog/internal/domain/anime.go`), created via GORM auto-migrate (no manual migration):

```go
NextEpisodeSource string `gorm:"size:16;default:'shikimori';column:next_episode_source" json:"next_episode_source,omitempty"`
```

**Critical guard — `refreshStaleAnime` (`catalog.go:665`).** The daily `shikimori_sync` batch refresh rebuilds a `fresh` Anime from Shikimori and calls a full-row `animeRepo.Update`, preserving only a handful of fields (ID/HasVideo/CreatedAt/poster). Without a guard it would **clobber** an AniList correction on the next nightly run. The stored AniList value must defend itself, using the same later-wins logic (no AniList call in this path):

```go
if existing.NextEpisodeSource == "anilist" && existing.NextEpisodeAt != nil &&
   (fresh.NextEpisodeAt == nil || existing.NextEpisodeAt.After(*fresh.NextEpisodeAt)) {
    fresh.NextEpisodeAt     = existing.NextEpisodeAt
    fresh.NextEpisodeSource = existing.NextEpisodeSource
}
```

This makes `NextEpisodeAt` **order-independent across both cron writers** (calendar sync and batch refresh): an AniList-sourced later date survives a Shikimori batch refresh, but if Shikimori ever produces an even-later date (the show resumed and slipped further) that newer date is allowed through and the source reverts to `shikimori`.

## 7. Error handling, rate limiting, observability

- **Fail-safe:** any AniList error or absent mapping **never fails the sync** — the anime keeps its Shikimori date and the loop continues. Reconciliation is strictly additive; calendar sync correctness is never coupled to AniList availability.
- **Rate limiting:** the ongoing set is bounded (dozens, not thousands). Calls are made **sequentially with conservative pacing** (≤ ~2 req/s) plus the existing per-call `aniListTimeout` already in `libs/idmapping`. AniList's public GraphQL allows ~90 req/min; sequential pacing stays well under it. **No Redis cache** — the job runs ~daily over a small set, so caching is YAGNI.
- **Observability:**
  - Counter `catalog_next_episode_source_total{source="anilist"|"shikimori"}` incremented per reconciled ongoing anime.
  - Info log on each actual override: shikimori id, old date → new date, AniList episode number.

## 8. Testing

Project convention: handwritten fakes/stubs, no testify/mock; table-driven where natural.

**`libs/idmapping` — `AniListAiringByMALID`** (against an `httptest` stub AniList server, matching existing `client_test.go` style):

- Concrete `nextAiringEpisode` → parsed `AniListAiring` with correct UTC time.
- `nextAiringEpisode: null` → `(nil, nil)`.
- `Media: null` (unknown id) → `(nil, nil)`.
- Non-200 / GraphQL `errors` body → `(nil, err)`.

**`catalog` — reconcile helper** (fake `AniListAiringFetcher`):

- AniList strictly later → override, `source = "anilist"`.
- AniList earlier → keep Shikimori, `source = "shikimori"`.
- AniList equal → keep Shikimori.
- AniList null / fetch error → keep Shikimori, sync does not fail.
- Non-ongoing anime → fetcher not called, Shikimori kept.

**`catalog` — `refreshStaleAnime` guard:**

- `existing` is anilist-sourced with a later date, `fresh` (Shikimori) earlier → AniList date + source survive.
- `existing` is anilist-sourced, `fresh` even later → Shikimori newer date wins, source reverts to `shikimori`.

## 9. Effort metrics

- **UXΔ = +2 (Better)** — correct upcoming-episode dates and correctly-timed new-episode notifications for ongoing shows. Invisible until a hiatus occurs, hence +2 rather than +3.
- **CDI = 0.04 * 8** — Spread × Shift ≈ 0.04 (two modules touched: `libs/idmapping` + catalog `service`/`domain`); Effort_Fib = 8 (new method + reconcile helper + DI wiring + column + batch-refresh guard + tests).
- **MVQ = Griffin 85%/80%** — clean integration that reuses existing AniList/transport/egress infrastructure; high slop-resistance via the source column and the order-independence guard.

## 10. Files Touched (implementation preview)

- `libs/idmapping/client.go` — `AniListAiring` type + `AniListAiringByMALID` method + shared GraphQL-POST helper.
- `libs/idmapping/client_test.go` — stub-server tests for the new method.
- `services/catalog/internal/domain/anime.go` — `NextEpisodeSource` column.
- `services/catalog/internal/service/catalog.go` — reconcile helper + wiring into `importMissingCalendarAnime` / `updateExistingCalendarEpisodes`; guard in `refreshStaleAnime`.
- `services/catalog/internal/service/catalog_test.go` (or co-located) — reconcile + guard tests with fakes.
- `services/catalog/cmd/catalog-api/main.go` — inject the existing idmapping client as the `AniListAiringFetcher` (reuse the IPv4 + egress-wrapped client already built at `main.go:487`).
- Metrics registration for `catalog_next_episode_source_total` (alongside existing catalog counters).
