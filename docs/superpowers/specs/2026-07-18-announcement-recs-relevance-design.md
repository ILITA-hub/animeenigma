# Announcement Recs — Relevance Hardening + MAL Popularity (Design)

**Date:** 2026-07-18
**Status:** Approved
**Supersedes/extends:** `2026-07-17-announcement-recs-spotlight-design.md` (the shipped "Upcoming for you" card)

## Problem

The shipped "Upcoming for you" card surfaced **"Witch Watch 2nd Season"** (announced) to user `tNeymik` labeled **"Matches your taste"** — a show whose **first season tNeymik never watched**.

Root cause (code-verified in `services/recs/internal/handler/upcoming.go`): the admission gate is an OR on raw scores —
`rawS8 >= MinS8 (franchise) OR rawS2 >= MinS2 (genre Jaccard)`. S8's candidate query requires `franchise <> ''`; both Witch Watch rows have an **empty franchise**, so `rawS8 = 0`. The title entered purely through **genre Jaccard (S2)** — the "taste" branch. The ranking ensemble already weights S5, but S5 is "ordering only, never gating," so **admission was effectively genre-only.**

Two product defects follow:
1. **Sequels/continuations admitted via taste** — recommending "Season 2" to someone who hasn't seen Season 1.
2. **Thin relevance** — a broad shared genre ("Comedy") is enough to admit, ignoring the ~10 richer scales the engine already computes.

## Goals

1. A **continuation** (later franchise entry) may surface **only through the franchise signal** (the user scored a prior franchise entry highly). Never through taste.
2. **Standalone** (first-entry / original) admission uses **rich attribute affinity (S5)** — tags, studio, genre, demographic, source, format — not genre alone.
3. Fold in **relative MAL popularity** so *featured/anticipated* announcements rank ahead of obscure ones.
4. The reason line states the **real driver** (franchise seed, top attribute, or anticipation), not a generic "Matches your taste."

Non-goal: changing the general logged-in/anon recs ensembles (`handler/recs.go`). This spec touches only the **announces (upcoming) path** and the **MAL popularity data foundation**.

## Signals available for UNAIRED content

Behavioral signals (S1 score-cluster, S3 trending, S4 recency) are structurally ~0 for announced titles — nobody has watched them. The usable scales are:

| Signal | Scales | Role in announces |
|---|---|---|
| **S5 attribute affinity** | tag 0.30, studio 0.25, genre 0.15, demographic 0.10, source 0.10, format 0.10 | **Standalone admission gate** + ranking |
| **S8 franchise** | best score in a scored franchise | **Continuation admission gate** + dominant ranking |
| **S9 MAL popularity** (NEW) | relative MAL members (log-scaled, pool-normalized) | Ranking booster (never gates) |
| **S7 dropped-penalty** | resemblance to dropped shows | Mild negative in ranking |
| **S2 genre** | genre Jaccard | Small ranking booster only (no longer gates) |

## Design

### 1. Continuation detection (`services/recs`)

A candidate is a **continuation** when EITHER holds:

- **Name heuristic** — `name` or `name_ru` matches a sequel marker (case-insensitive):
  `(2nd|3rd|4th|5th|second|third|fourth|fifth|final)\s+season` · `season\s+\d+` · `\bpart\s+(\d+|ii|iii|iv|v)\b` · `\d+(st|nd|rd|th)\s+(season|part|cour)` · `\bcour\s+\d+` · trailing roman numeral `\s(ii|iii|iv|v|vi)$` · RU: `\d+[\s-]*(й|-й|ый)?\s*сезон` · `часть\s+\d+`.
- **Franchise-structural** — candidate's `franchise <> ''` AND a sibling exists in that franchise with `status IN ('released','ongoing')` (i.e., an earlier entry already aired). One `EXISTS` query batched for the pool.

Rationale: the name heuristic catches unenriched sequels (Witch Watch's franchise is empty); the structural check catches subtitle-named sequels once franchises are enriched.

### 2. Admission gate rework (`upcoming.go::computeUpcoming`)

Per candidate, after `RankWithBreakdown`:

```
rawS8 := Raw["s8"]; rawS5 := Raw["s5"]
if isContinuation(id) {
    if rawS8 < cfg.MinS8 { continue }        // franchise-only for sequels
    franchiseReason = true
} else {
    if rawS8 < cfg.MinS8 && rawS5 < cfg.MinS5 { continue }  // rich taste OR franchise
    franchiseReason = rawS8 >= upcomingFranchiseReasonMinS8  // 0.4
}
```

- `MinS5` is a new env knob `RECS_UPCOMING_MIN_S5` (calibrated at verification against live `tNeymik` + known-good matches; conservative default).
- Genre (S2) is **removed from the gate** entirely.

### 3. Ranking (`ensemble` in `computeUpcoming`)

New weights: `{S8:0.40, S5:0.30, S9:0.15, S2:0.10, S7:-0.05}`. S8 stays dominant. After gating, **sort franchise-fired items strictly ahead of taste items**, then by `Final` desc, then take TopK — so a franchise continuation always outranks a taste standalone regardless of per-pool normalization quirks.

### 4. S9 MAL popularity signal (`services/recs/.../signals/s9_mal_popularity.go`)

- `Score` returns `log1p(mal_members)` for candidates with `mal_members > 0`; omits the rest.
- The ensemble's per-pool min-max normalization makes it **relative-within-the-current-announced-pool** popularity automatically.
- `Precompute` is a no-op (request-time, like S2/S8). Stateless.

### 5. Reason enrichment (`upcoming.go`)

`UpcomingReason.Kind` gains richer forms. Resolution order:
1. **franchise** (S8 fired) — existing seed lookup ("you rated *X* 9/10").
2. **attribute** — the single S5 dimension contributing the most to the candidate's raw S5, mapped to a human phrase + the shared attribute's name: `studio` → "Same studio as your favorites (*Studio*)"; `source` → "From a *manga/LN/…*, like you watch"; `tag`/`genre` → "Because you like *Tag*"; `demographic`/`format` → generic taste. Requires exposing S5's per-dimension breakdown for the top candidate (a small helper on S5 or a targeted query in the handler).
3. **anticipated** — fallback when no clear attribute dominates but the item is pool-popular (`relative popularity` high): "Highly anticipated".
4. **taste** — final generic fallback.

Frontend renders each kind; new i18n keys in en/ru/ja.

### 6. Catalog — MAL popularity data foundation (`services/catalog`)

- **Jikan client** (`parser/jikan/client.go`): extend `AnimeInfo` with `Members int json:"members"`, `Favorites int json:"favorites"`, `Popularity int json:"popularity"`.
- **Domain** (`domain.Anime`): add `MalMembers int` and `MalFavorites int` (GORM `AutoMigrate` adds the columns; no destructive change).
- **announcements-sync** (`service/catalog_sync.go::SyncAnnouncements`): for each announced title with a `mal_id`, fetch Jikan popularity (rate-limited by the client's existing limiter) and persist `mal_members`/`mal_favorites`. Failures are logged and skipped (best-effort; the card degrades to relevance-only ranking). Scope: announced pool only.

## Data model changes

- `animes.mal_members INT DEFAULT 0`, `animes.mal_favorites INT DEFAULT 0` (GORM auto-add).
- No new tables. No changes to `rec_announcement_dismissals`.

## Cache

- Upcoming key suffix `:upcoming:v1 → :upcoming:v2` (ranking + gate changed).

## Global Constraints

- Go: shared `libs/errors`, `libs/logger`; GORM conventions; snake_case files; table/column auto-migrate only (no drops).
- Recs reads `animes` directly (shared `animeenigma` Postgres) — the `mal_members` column must exist before S9 scores; catalog's AutoMigrate creates it.
- Effort metrics use UXΔ / CDI / MVQ — never time units.
- Frontend: bind to semantic tokens; i18n parity across en/ru/ja with matching ICU placeholders; `/frontend-verify` before finishing FE.
- Signal contract: `ID()`, `Precompute()`, `Score()` — new signals are purely additive to the registry.
- No hard external dependency: Jikan failures must never fail the sync or the card.

## Verification

- Unit: continuation detector (name + structural), S9 math, gate branches, reason resolution.
- Integration/live (post-deploy): `GET /api/users/recs/upcoming` as `tNeymik` no longer returns Witch Watch 2nd Season; returned items carry attribute/franchise/anticipated reasons; calibrate `RECS_UPCOMING_MIN_S5`.
