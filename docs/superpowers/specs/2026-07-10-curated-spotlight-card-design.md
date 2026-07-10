# Curated Spotlight Card — Design

**Date:** 2026-07-10 · **Status:** approved-pending-spec-review · **Workstream:** hero-spotlight

## 1. Summary

Add a new **`curated`** spotlight card type — displayed kicker **"Curator Recommends"**
(«Куратор рекомендует» / 「キュレーターのおすすめ」) — that features **one hand-picked
anime** on the home-page hero carousel. The launch pick is **Yani Neko / Табакошка /
Chainsmoker Cat** (ヤニねこ, `shikimori_id 63403`), a Summer-2026 ongoing TV series.

Two supporting mechanisms are introduced:

1. **Numeric card priority** — a new `priority` float on every spotlight `Card`
   (default `1.0`); the curated card carries **`1.5`**. Priority is a **weight** in the
   carousel's *random* initial-slide pick — it does **not** force ordering.
2. **Airing-status season gate** — the card is eligible **only while its anime is
   `ongoing`**. When the show flips to `released` the resolver drops it and the slide
   disappears on its own. No dates to maintain — this is the entire "active this season
   only" behaviour.

## 2. Locked decisions (from brainstorming)

| # | Decision | Choice |
|---|----------|--------|
| D1 | Build approach | New dedicated card type via the standard 5-anchor recipe (`docs/spotlight-card-guidelines.md`). Distinct from the existing `featured` («Рекомендуем сегодня») card. |
| D2 | Card name / type discriminator | `curated` (component `CuratedCard.vue`, resolver `curated.go`, i18n `spotlight.curated.*`). |
| D3 | Displayed kicker | "Curator Recommends" / «Куратор рекомендует» / 「キュレーターのおすすめ」. (One-word "Curated" is a trivial i18n swap if preferred later.) |
| D4 | Priority | Real numeric `priority` field on all cards; curated = `1.5`; consumed as a **weight in the FE's weighted-random start** (not forced order). |
| D5 | Season window | Tie to airing status — eligible only while `anime.status == "ongoing"`. |
| D6 | Featured anime | Env-configurable `SPOTLIGHT_CURATED_SHIKIMORI_ID`, **default `63403`**; empty ⇒ card off (lets the pick be re-pointed / retired next season with no code change). |

## 3. Backend changes

### 3.1 Card priority field — `spotlight/types.go` + `spotlight/aggregator.go`
- Add `Priority float64 \`json:"priority"\`` to the `Card` struct.
- The aggregator **normalizes** on collection: a card whose resolver returned
  `Priority == 0` is set to `defaultCardPriority = 1.0` immediately before it is
  appended to the response slice. This means the **other 8 resolvers are not touched** —
  only the new curated resolver sets a non-default value (`1.5`).
- **No sorting** is added. Response order stays as today (resolver-completion order);
  priority is purely a per-card weight the frontend reads. This keeps the change minimal
  and the existing rotation feel intact.
- `types_test.go`: extend the Card marshal round-trip to assert `"priority"` serializes.

### 3.2 `CuratedData` payload — `spotlight/types.go`
```go
// CuratedData is the payload for Card{Type: "curated"} — a single
// hand-picked anime surfaced while it is currently airing.
type CuratedData struct {
    Anime domain.Anime `json:"anime"`
}
```
- `types_test.go`: add a marshal/unmarshal round-trip test (mirrors `FeaturedData`).

### 3.3 Resolver — `spotlight/cards/curated.go`
Mirrors `featured.go` (manual cache Get/Set + `errors.Is(err, cache.ErrNotFound)`;
no-cache-on-empty). Constant `curatedPriority = 1.5`.

```go
type animeGetter interface {
    GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}

type CuratedResolver struct {
    repo        animeGetter
    cache       cache.Cache
    log         *logger.Logger
    shikimoriID string
}

func (r *CuratedResolver) Type() string { return "curated" }
```

`Resolve(ctx, _ *string)` (userID ignored — global pick):
1. If `r.shikimoriID == ""` → `(nil, nil)` (card disabled). Do **not** cache.
2. Cache key `spotlight:curated:<DateKeyUTC(now)>`; on hit return the cached `CuratedData`.
   (Warnw + fall through on a non-`ErrNotFound` Redis error — never fail the request.)
3. `anime, err := r.repo.GetByShikimoriID(ctx, r.shikimoriID)`.
   `GetByShikimoriID` already returns `(nil, nil)` on not-found — so `anime == nil`
   ⇒ `(nil, nil)`, uncached (the anime hasn't been populated yet).
4. **Airing gate:** if `anime.Status != string(domain.StatusOngoing)` → `(nil, nil)`,
   uncached. This is the season self-expiry.
5. Build `Card{Type: "curated", Priority: curatedPriority, Data: CuratedData{Anime: *anime}}`,
   `cache.Set` best-effort (`cardTTL = 24h`), return.

`curated_test.go` — handwritten fake `animeGetter` (no testify), cases:
- eligible ongoing anime → card with `Priority == 1.5`;
- `released` anime → `(nil, nil)`, nothing cached;
- empty `shikimoriID` → `(nil, nil)`;
- not-found (`GetByShikimoriID` → nil) → `(nil, nil)`;
- cache-hit path returns without calling the repo.

### 3.4 DI — `cmd/catalog-api/main.go`
- Read `SPOTLIGHT_CURATED_SHIKIMORI_ID` (default `"63403"`).
- Append `cards.NewCuratedResolver(animeRepo, redisCache, log, curatedShikID)` to the
  `spotlightResolvers` slice.

## 4. Frontend changes

### 4.1 Types — `src/types/spotlight.ts`
- Add `CuratedData`:
  ```ts
  export interface CuratedData { anime: SpotlightAnime }
  ```
- Add the union member and a shared `priority` via intersection (keeps discriminated-union
  narrowing intact for the v-if chain / vue-tsc):
  ```ts
  export type SpotlightCard = (
    | { type: 'featured'; data: FeaturedData }
    | /* …existing 8… */
    | { type: 'curated'; data: CuratedData }
  ) & { priority?: number }
  ```

### 4.2 `CuratedCard.vue` (+ `.spec.ts`)
- Mirrors `FeaturedCard.vue`: single-root `SpotlightCardShell`, `backdrop="poster-blur"`
  with the anime poster, hero layout, CTA `router-link` (`buttonVariants`) to the watch page.
- **Accent `cyan`** (the "content-core" triad slot). Differentiated from Featured by a
  distinct kicker **icon** (e.g. `Award` / `BadgeCheck`) — no 4th hue (DS-lint rule).
- Kicker `t('spotlight.curated.title')`, `font-medium` (never semibold).
- Poster via `cardPosterUrl(url, width)`, DS shimmer→fade (SpotlightPoster or the
  raw-`<img>` fade pattern). `min-h-[400px] md:min-h-[340px] lg:min-h-[320px]`,
  `p-4 md:p-6 lg:p-8`, Tailwind-utility-only.
- `.spec.ts` ≥5 assertions (renders title, poster, CTA href, single article root, external-safe attrs).

### 4.3 `tokens.ts`
- Add a `curated` entry: `accent: 'cyan'`, `kickerKey: 'spotlight.curated.title'`, `icon`
  (matching CuratedCard). Missing entry ⇒ generic-sparkles fallback + wrong a11y label.

### 4.4 `HeroSpotlightBlock.vue`
- Import `CuratedCard`; add dispatch branch
  `v-else-if="active.type === 'curated'"` with `:key="\`curated:${currentIndex}\`"`.
- **Weighted-random start:** replace the uniform pick at line ~430
  (`currentIndex.value = Math.floor(Math.random() * n)`) with
  `currentIndex.value = weightedRandomIndex(cards.value)`, a module-scope helper:
  ```ts
  function weightedRandomIndex(cards: SpotlightCard[]): number {
    const weights = cards.map((c) => (c.priority ?? 1))
    const total = weights.reduce((a, w) => a + w, 0)
    let r = Math.random() * total
    for (let i = 0; i < weights.length; i++) {
      r -= weights[i]
      if (r < 0) return i
    }
    return cards.length - 1
  }
  ```
  When every card is `1.0` this is exactly the current uniform distribution
  (backward-compatible); a `1.5` card is 1.5× likelier to be the opening slide.
- `cardTitle()`: add `case 'curated'` returning the localized anime title (snake_case fields).
- `cardImageUrls()` (idle-prefetch map): register CuratedCard's poster surface at the
  **exact** proxy width buckets it renders — a bucket mismatch silently breaks prefetch.

### 4.5 `useSpotlight.ts`
- No transform (snake_case passthrough). Verify `priority` survives the defensive
  `{success,data}` unwrap so the FE weighting sees it; add a passthrough assertion to
  `useSpotlight.spec.ts`.

### 4.6 i18n — `en.json` / `ru.json` / `ja.json` (parity enforced)
`spotlight.curated`:
- `title`: `"Curator Recommends"` / `«Куратор рекомендует»` / `「キュレーターのおすすめ」`
- `cta`: watch-button label, e.g. `"Watch now"` / `«Смотреть»` / `「今すぐ見る」`
  (reuse an existing generic key if one fits rather than adding a duplicate).

The parity test `src/locales/__tests__/spotlight-keys.spec.ts` fails on any en/ru/ja drift.

## 5. Caching & rollout
- The new `priority` field changes the cached payload shape ⇒ **same-day flush** of both
  `spotlight:*` card keys **and** `spotlight:snapshot:*` after deploy (guidelines §5;
  stale old-shape JSON deserializes into empty structs).
- Live-smoke `GET /api/home/spotlight`: confirm a `{"type":"curated","priority":1.5,...}`
  card is present while Табакошка is `ongoing`, and that other cards now carry
  `"priority":1`.

## 6. Testing
- **BE:** `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race`
  (curated resolver cases + aggregator priority-normalization + types round-trip).
- **FE:** `cd frontend/web && bunx vitest run src/components/home/spotlight/ src/locales/__tests__/spotlight-keys.spec.ts && bunx tsc --noEmit`, then DS-lint and `/frontend-verify`.
- **E2E regression:** `frontend/web/e2e/spotlight.spec.ts` (single-root article; no carousel wedge).

## 7. Metrics (CONVENTIONS.md)
- **UXΔ = +2 (Better)** — a prominent, hand-picked, self-expiring recommendation on the home hero.
- **CDI = 0.04 × 13** — small spread (spotlight package + one cross-cutting `priority` field), moderate effort (new card type + weighted-start change across BE & FE).
- **MVQ = Griffin 85% / 80%** — well-trodden 5-anchor recipe with one genuinely new axis (priority weighting).

## 8. Out of scope (YAGNI)
- No admin CRUD UI for curated slides — a single env-configured pick.
- No general multi-slide curation system / DB schema.
- No forced or deterministic carousel ordering — priority is weight-only.
