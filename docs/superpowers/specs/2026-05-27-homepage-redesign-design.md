# Design: Homepage Redesign — "Neon Tokyo, Cinematic"

**Date:** 2026-05-27
**Status:** Design approved; implementation plan pending (delivery scope = spec + plan, then stop)
**Source design:** `/tmp/Animeenigma.zip` → `design_handoff_homepage_redesign/` (hifi React-via-Babel prototype). `styles.css` in that bundle is the **source of truth** for every token, size, and value. This spec records the *reconciliation* decisions against the existing codebase; where a value isn't restated here, defer to the handoff `styles.css`.

---

## 1. Overview

Refine the existing **Neon Tokyo** system (cyan/pink on near-black, glass) into a **cinematic, editorially-ordered** homepage that leads with content instead of an admin "Platform Stats" card.

Net user-facing shifts vs. production:

1. Hero defaults to a **full-bleed featured anime** (poster + RU title + JP accent + score/genre meta + play CTA) instead of platform stats. Stats becomes one rotating slide.
2. **Continue Watching is promoted above** the 3-column grid.
3. **3-column grid reframed** — Ongoing gets a next-episode countdown line; Top Anime uses giant background rank numerals (top-3 accent-tinted); Announcements gets season chips.
4. Darker page base (`#08080f`) + two ambient radial gradients, applied **globally**.
5. Self-hosted editorial type system (Manrope display + JetBrains Mono mono + Inter UI).

**Explicitly NOT in the mock and dropped:** the genre-pill filter row.

---

## 2. Locked decisions (from brainstorm 2026-05-27)

| # | Decision | Choice |
|---|----------|--------|
| D1 | Delivery scope this session | **Spec + implementation plan, then stop.** No code. |
| D2 | Hero carousel reconciliation | **Replace `anime_of_day` with `featured`** — clean rename of the existing card, reusing & extending its resolver logic. Card count stays **9** (not 10). Keep the other 8 types; restyle the whole carousel; PlatformStats stays as one slide. |
| D2a | Discriminator rename | `anime_of_day` → `featured` everywhere (clean break per `feedback_replace_dont_preserve.md`): resolver `Type()`, cache key, `types.go` struct, TS union, dispatch `v-if`/`switch`, component file, `tokens.ts` key, `spotlight.animeOfDay.*` → `spotlight.featured.*`, tests + snapshots. |
| D3 | Fonts | **Self-host** Manrope, JetBrains Mono, Inter (no Google CDN). |
| D4 | Genre-pill row | **Dropped entirely.** |
| D5 | "Сейчас в эфире" hero side panel | **Deferred to TODO** (needs a today-airing endpoint that doesn't exist). Featured hero ships with bottom-left content full-width — already a designed responsive state. |
| D6 | Page base color | **Darken globally** `--color-base` `#121218` → `#08080f`; add the two ambient radial gradients to `body`. |
| D7 | Tweaks panel, brief banner, accent/density toggles | **Out of scope.** Commit to **cyan + cozy**. |
| D8 | Collections row (exists today, not in mock) | **Kept & restyled**, placed between the 3-column grid and the activity grid. |

---

## 3. Approach

**Chosen: A — Extend the theme + restyle existing components in place; repurpose Anime-of-the-Day into the cinematic Featured hero.**

Add the missing tokens + self-hosted fonts to `main.css`, then edit the components that already exist to match the mock. Rather than introduce a 10th card type, **transform the existing `anime_of_day` card into `featured`** (the inverse of the "add a card" recipe — same 5 anchors touched, but renamed in place). Its resolver already produces a `domain.Anime` with every field the cinematic hero needs (`name_ru`, `name_jp`, `description`, `year`, `season`, `status`, `episodes_aired`, `score`, `poster_url`, `genres`), so no new data plumbing is required — only a richer presentation + a small selection extension. Reuses the carousel state machine, composables, APIs, and test harness.

**Rejected:**
- **B — Parallel `HomeV2` behind a flag.** Duplicates the entire carousel + 9-card system for what is a reskin + one new card. Higher CDI, more dead code to reconcile later.
- **C — Pure CSS reskin.** Cannot deliver the structural changes (featured hero, CW promotion, rank-numeral background type), so it misses the brief.

---

## 4. Token & font layer

**File:** `frontend/web/src/styles/main.css` (+ new `frontend/web/public/fonts/`)

### 4.1 Self-hosted fonts
Bundle WOFF2 files under `public/fonts/` and declare `@font-face` in `main.css` (not via `<link>` to Google). Preload the two most above-the-fold faces (Manrope 800, Inter 400) in `index.html`.

- **Manrope** 700, 800 → `--f-display`
- **Inter** 400, 500, 600, 700 → `--f-ui`
- **JetBrains Mono** 400, 500 → `--f-mono`
- **Noto Sans JP** 400, 500 → `--f-jp` (already referenced; self-host to match)

### 4.2 Tokens to add
Map to Tailwind `@theme` where a utility class should exist; add the rest as semantic `:root` custom properties.

| Token | Value | Notes |
|---|---|---|
| `--color-base` (existing) | `#08080f` | **Global darken** (D6) |
| `--color-surface` (existing) | `#11111c` | darken to match (was `#1a1a24`); global, follows base |
| `--surface-2` | `#161623` | new |
| `--elevated` | `#1c1c2c` | new |
| `--line` | `rgba(255,255,255,0.06)` | new |
| `--line-strong` | `rgba(255,255,255,0.12)` | new |
| `--ink` / `--ink-2` / `--ink-3` / `--ink-4` | `#fff` / `.78` / `.56` / `.36` white | new text ramp |
| `--accent` + `-soft` + `-line` + `-glow` | `#00d4ff` family | semantic alias of cyan-400 |
| `--pink` / `--pink-soft` | `#ff2d7c` / `.14` | alias of pink-500 |
| `--violet` | `#a78bfa` | new (season chips) |
| `--success` / `--warning` | existing | reuse |
| `--r-sm/md/lg/xl/2xl` | `8/12/16/22/28 px` | new radii scale |
| `--f-display/-ui/-jp/-mono` | see 4.1 | new |

### 4.3 Page background (global)
```css
body {
  background:
    radial-gradient(1000px 600px at 80% -200px, rgba(0,212,255,0.08), transparent 60%),
    radial-gradient(800px 500px at -10% 10%, rgba(255,45,124,0.06), transparent 60%),
    var(--color-base);
  background-attachment: fixed;
}
```

### 4.4 Cascade-trap guard (mandatory)
Per `reference_tailwind_v4_css_cascade.md`: unlayered custom classes beat utilities, and a second plain `@layer components` block gets tree-shaken. New home CSS classes MUST be added inside the existing properly-layered block, and any cascade interaction (e.g. utilities overriding `.hero h1`) MUST be **verified in-browser**, not in jsdom. Replace the current `from-gray-900 via-gray-900 to-black` body gradient on `Home.vue` so it doesn't fight the new global background.

---

## 5. Component design

All exact sizes/colors/radii/shadows are in the handoff `styles.css`. Below is the *what-changes-where* map. Russian copy moves to `ru.json`/`en.json` — no hardcoded strings.

### 5.1 Navbar — `components/layout/Navbar.vue`
- **Brand mark:** 28×28 rounded-8 cyan→pink gradient square, 4px inset cutout (`--base`), "AE" cyan 11px Manrope 800, accent glow. Extract to a small `BrandMark.vue`.
- Wordmark "Anime"(cyan)+"Enigma"(white) Manrope 800 18px.
- **Active link:** animated 2px cyan underline pill 16px below link with `0 0 10px` glow (0.2s ease).
- Right tools: search icon → lang pill ("RU ▾") → bell w/ 6px pink notification dot (driven by real unread count) → 36×36 avatar w/ success online dot.
- Icon buttons 36×36 rounded-10, hover `rgba(255,255,255,.05)`.

### 5.2 Search row — `views/Home.vue`
- Grid `1fr auto`, gap 12. Input 56px, rounded-16, `⌘K` kbd, focus→`--accent-line`. (Reuse existing `SearchAutocomplete`, restyle wrapper.)
- "Расписание" button 56px, accent-tinted (`--accent-soft`/`--accent-line`/`--accent`).

### 5.3 Hero — `components/home/spotlight/`
**Repurpose `anime_of_day` → `featured`** (D2/D2a). The same 5 anchors as the "add a card" recipe, but renamed in place — reusing and extending the existing logic:

1. **Resolver** — rename `cards/anime_of_day.go` → `cards/featured.go`, `AnimeOfDayResolver` → `FeaturedResolver`, `Type()` returns `"featured"`, cache key `spotlight:featured:<date>`. **Reuse** the date-seeded daily pick from the top-200 `score ≥ 8.0` pool (incl. the deliberate "don't cache empty" discipline). **Extend** selection: a curated `sort_priority` pin (existing pattern, `CLAUDE.md` §"Pinning anime") wins first; else fall back to the daily-seeded top-rated pick. (See §7.)
2. **Data** — rename `AnimeOfDayData` → `FeaturedData` in `types.go` (same shape: `Anime domain.Anime` + optional reason key). Update `types_test.go` round-trip.
3. **DI** — update the `spotlightResolvers` slice in `catalog-api/main.go` to `cards.NewFeaturedResolver(...)`, placed **first** so featured wins tie-break display order (the default slide).
4. **SFC** — rename `cards/AnimeOfDayCard.vue` → `cards/FeaturedCard.vue` and **transform** it from the current poster-beside-text layout into the mock's **full-bleed cinematic hero**: `background-image` poster + dual scrim, eyebrow+pulse with **status-aware label** (`ongoing→Сейчас выходит`, `announced→Анонс сезона`, else `Рекомендуем сегодня`), RU `<h1>` + JP `.jp` subtitle (0.42em), meta row (score warning chip / year / `N эп.` / up-to-3 genre chips), 3-line clamped description, primary CTA **status-aware** (`ongoing→Смотреть · эп. {aired+1}`, `announced→Напомнить о выходе`, else `Начать просмотр`) + secondary "В список". Honors UI-SPEC weight/padding/height contract. Rename `.spec.ts` → `FeaturedCard.spec.ts` (≥5 assertions).
5. **Dispatch + i18n + tokens** — rename the union member `{ type: 'anime_of_day' }` → `{ type: 'featured' }` in `types/spotlight.ts`; rename the `v-if`/`switch` branch + import in `HeroSpotlightBlock.vue` (keep typed chain, NOT `<component :is>`); rename the `cardTokens.anime_of_day` key in `tokens.ts`; move `spotlight.animeOfDay.*` → `spotlight.featured.*` in BOTH locales (add the new status eyebrows + CTAs; parity test guards this).

- **Restyle the carousel chrome** to the mock: arrows 36×36 glass round at 20px sides; dot nav bottom-right (26×4 inactive / 36×4 active cyan w/ glow). Keep the shipped 7s auto-cycle + pause-on-hover + reduced-motion behavior.
- **Restyle the other 8 cards** to the new tokens (notably PlatformStats → matches the mock's `hero-stats` two-column variant: "Работает: ДА", uptime line, ASCII tree, quote, 2×2 stat tiles).
- **Side panel `Сейчас в эфире` → deferred** (D5). Add a TODO comment + backlog note; do not build now.

> **Rename impact (must do as one change):** flush `spotlight:*` Redis keys on deploy (`feedback_spotlight_cache_shape_migration.md` — stale `anime_of_day` JSON must not linger), regenerate spotlight `__snapshots__`, and confirm no `anime_of_day` references remain (`grep -rn anime_of_day frontend/ services/`). The user-facing concept becomes "Featured"; no `anime_of_day` identity is left behind (`feedback_replace_dont_preserve.md`).

### 5.4 Continue Watching — `components/home/ContinueWatchingRow.vue` (+ reorder in `Home.vue`)
- **Promote above the 3-column grid** (D3 ordering).
- Cards: 16:9 (cozy), rounded-16, image + bottom scrim, centered 52px cyan play button (hover reveal scale 1.06), episode label (mono cyan), title (single-line ellipsis), time-left, bottom 3px progress bar (cyan fill w/ glow). Hover lift -2px + accent border + glow.
- Section header: h2 22px "Продолжить просмотр" + mono count badge + right "Вся история →".

### 5.5 3-column grid — `views/Home.vue` (column markup) + shared `ColumnItem`
Consider extracting a `HomeColumn.vue` + `ColumnItem.vue` to cut `Home.vue` size (it's 523 lines and growing).
- Column shell: gradient bg, `--line` border, rounded-22, padding 18; header = 36×36 semantic icon tile + h3 17px + mono sub + "Все" pill.
- **Ongoing:** green icon, `● Выходит` success chip, optional score chip, **next-ep line** ("Серия N · Сегодня в HH:MM", mono cyan + clock) when `next_episode_at` present.
- **Top:** gold icon, score chip, **giant rank numeral** (Manrope 800 56px) absolutely positioned right-center, `rgba(255,255,255,.04)`; ranks 1–3 → `rgba(0,212,255,.08)`. (The codebase already has subtle rank numerals — restyle to the mock's size/position.)
- **Announcements:** cyan icon, `Анонс` chip + violet season chip.
- Item row: grid `56px 1fr`, poster 2:3 rounded-8, hover bg + title→accent.

### 5.6 Activity grid — `components/home/ActivityFeed.vue` + `LastUpdates.vue`
- 2-col, restyle only. Feed: 28px avatar + `@user` + action + accent title + optional italic excerpt + mono time. Last updates: 36×48 thumb + title/sub + right mono "when", hover bg.

### 5.7 Collections — `components/home/CollectionsRow.vue`
- Kept (D8), restyled to new card tokens, placed between grid and activity. Self-hides when empty (unchanged behavior).

### 5.8 Section order (before → after)

| Before | After |
|---|---|
| Search → Hero → 3-col grid → Collections → Continue Watching → Activity | Search → Hero(featured default) → **Continue Watching** → 3-col grid → Collections → Activity |

*(Genre-pill row never added.)*

---

## 6. i18n
New/changed keys in BOTH `ru.json` + `en.json` (parity test enforces):
- `spotlight.featured.*` — eyebrows (`Сейчас выходит`/`Анонс сезона`/`Рекомендуем сегодня`), CTAs (`Смотреть · эп. {n}`/`Напомнить о выходе`/`Начать просмотр`/`В список`).
- Column subs already exist (`home.updated`, etc.); add `home.nextEpisodeLine`, season-chip label if missing.
- Reuse existing `home.*`, `activity.*`, `updates.*`, `collections.*`.

---

## 7. Backend — `featured` resolver (renamed & extended from `anime_of_day`)
`services/catalog/internal/service/spotlight/cards/featured.go` (was `anime_of_day.go`), implementing `spotlight.Resolver`:
- `Type() → "featured"`; cache key `spotlight:featured:<DateKeyUTC>`.
- **Reuse:** the existing date-seeded pick from the top-200 `score ≥ minScore (8.0)` candidate pool, the manual `cache.Get`/`Set` + `errors.Is(err, cache.ErrNotFound)` discipline, and the **don't-cache-empty** `(nil, nil)` eligibility path (Pitfall 5). Returns `(*Card,nil)` / `(nil,nil)` if pool empty / `(nil,err)` on failure.
- **Extend (selection):** before the daily-seeded pick, query for a curated pin — highest `sort_priority` (> 0), tie-broken by score — and use it when present. This makes the hero admin-controllable via the documented `UPDATE animes SET sort_priority=…` pattern, mirroring the README's "hand-curated CMS list" note. When no pin exists, behavior is identical to today's anime-of-day.
- Keep the co-located `_test.go` (renamed), handwritten fakes (no testify/mock); add a case for the pin-wins path.
- **Cache-shape caution** (`feedback_spotlight_cache_shape_migration.md`): the key *renames* (`anime_of_day` → `featured`), so flush `spotlight:*` on deploy and runtime-smoke the live `/api/home/spotlight` so no stale struct ships.

---

## 8. Testing & verification
- `cd services/catalog && go test ./internal/service/spotlight/... -count=1 -race`
- `cd frontend/web && bunx vitest run src/components/home/ src/locales/__tests__/spotlight-keys.spec.ts && bunx tsc --noEmit`
- Regenerate spotlight snapshot tests after restyle.
- **In-browser smoke** (cascade-trap guard §4.4 + i18n key guard `feedback_smoke_verify_i18n.md`): confirm fonts load, featured slide renders, CW sits above grid, rank numerals position correctly, no raw i18n key strings.
- Playwright `e2e/spotlight.spec.ts` remains the carousel regression.

---

## 9. Deferred / TODO (tracked, not built)
- **`Сейчас в эфире` today-airing hero side panel** — needs a `schedule-today (±window)` endpoint. Track as a backlog item; revisit after this redesign.

## 10. Out of scope
Tweaks panel, designer brief banner, accent/density toggles, genre-pill row.

---

## 11. Metrics (per `.planning/CONVENTIONS.md`)

| Item | UXΔ | CDI | MVQ |
|------|-----|-----|-----|
| **Redesign aggregate** | `+3 (Better)` | `0.18 * 34` | `Dragon 88%/90%` |
| Token + self-hosted font layer | `+1 (Better)` | `0.08 * 8` | `Sprite 80%/90%` |
| `anime_of_day` → `featured` (rename + cinematic transform + pin) | `+3 (Better)` | `0.07 * 13` | `Phoenix 88%/90%` |
| Carousel + 8-card restyle | `+2 (Better)` | `0.05 * 8` | `Griffin 82%/85%` |
| CW promotion + 16:9 cards | `+2 (Better)` | `0.02 * 5` | `Sprite 85%/88%` |
| 3-column reframe (next-ep, rank, season) | `+2 (Better)` | `0.04 * 8` | `Griffin 80%/85%` |
| Navbar refresh | `+1 (Better)` | `0.02 * 5` | `Sprite 78%/85%` |

CDI reasoning: spread is moderate (touches `main.css`, `index.html`, `Home.vue`, navbar, several home components, `types/spotlight.ts`, one catalog resolver + DI + locales) but coherence shift stays low — every change extends an existing, documented pattern (the 5-anchor card recipe, the token system). Effort `34` = significant phase-of-work.

---

## 12. Files touched (anticipated)
**Frontend:** `styles/main.css`, `index.html`, `public/fonts/*`, `views/Home.vue`, `components/layout/Navbar.vue` (+ new `BrandMark.vue`), `components/home/spotlight/HeroSpotlightBlock.vue`, `cards/AnimeOfDayCard.vue` → **rename** `cards/FeaturedCard.vue` (+ `.spec.ts` rename) + transform, restyle of the other 8 cards, `ContinueWatchingRow.vue`, `CollectionsRow.vue`, `ActivityFeed.vue`, `LastUpdates.vue`, new `HomeColumn.vue`/`ColumnItem.vue`, `tokens.ts` (key rename), `types/spotlight.ts` (union member rename), `locales/{ru,en}.json` (namespace move + new keys), `__snapshots__/*` regen.
**Backend:** `cards/anime_of_day.go` → **rename** `cards/featured.go` (+ test rename, reuse + pin extension), `spotlight/types.go` (struct rename, +test), `catalog-api/main.go` (DI rename + order).
**Migration:** flush `spotlight:*` Redis keys on deploy; `grep -rn anime_of_day` must return zero after the rename.
