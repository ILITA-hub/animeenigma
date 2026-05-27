# Homepage Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recreate the "Neon Tokyo, Cinematic" homepage redesign in the existing Vue 3 + TS + Tailwind v4 codebase, leading with a full-bleed featured-anime hero and reordered content sections.

**Architecture:** Extend the theme tokens + self-host fonts, then restyle the existing home components in place. The shipped `anime_of_day` spotlight card is **renamed to `featured`** end-to-end and its frontend card transformed into the cinematic hero (no new card type). All other infra — the carousel state machine, composables, APIs, tests — is reused.

**Tech Stack:** Vue 3 SFC, TypeScript, Tailwind v4 (`@theme`), Vitest + Vue Test Utils, Pinia, vue-i18n; Go (catalog service) for the spotlight resolver.

**Spec:** `docs/superpowers/specs/2026-05-27-homepage-redesign-design.md`. The design bundle `/tmp/Animeenigma.zip → design_handoff_homepage_redesign/styles.css` is the **source of truth** for every exact pixel/color/radius value. When a step says "per handoff," open that file and copy the literal value — do not approximate.

**Conventions:**
- Frontend uses `bun`/`bunx` (never npm). After service code changes: `make redeploy-<service>`.
- Citations use grep anchors, not line numbers (`CLAUDE.md` audit rule).
- No time estimates anywhere; score with UXΔ/CDI/MVQ if adding plan notes (`.planning/CONVENTIONS.md`).
- Commit co-authors (every commit):
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Do NOT auto-run `/animeenigma-after-update`** — this plan ends at "ready to deploy"; the user triggers deploy.

---

## Metrics (per `.planning/CONVENTIONS.md`)

Scored per-task; aggregate is the blended redesign. CDI two-number form (Distribution Spread × Coherence Shift) `*` Fibonacci Effort — not pre-multiplied.

| Task | UXΔ | CDI | MVQ |
|------|-----|-----|-----|
| **Aggregate (whole redesign)** | `+3 (Better)` | `0.18 * 34` | `Dragon 88%/90%` |
| 1 — Token & font foundation | `+1 (Better)` | `0.08 * 8` | `Sprite 80%/90%` |
| 2 — Backend `featured` rename + curated pin | `+1 (Better)` | `0.03 * 8` | `Phoenix 78%/88%` |
| 3 — Frontend rename plumbing | `0 (Ambiguous)` | `0.04 * 3` | `Sprite 65%/85%` |
| 4 — `FeaturedCard` cinematic transform | `+3 (Better)` | `0.02 * 8` | `Phoenix 90%/92%` |
| 5 — Carousel chrome + 8-card restyle | `+2 (Better)` | `0.05 * 8` | `Griffin 82%/85%` |
| 6 — Continue Watching promote + 16:9 | `+2 (Better)` | `0.02 * 5` | `Sprite 85%/88%` |
| 7 — 3-column reframe + extraction | `+2 (Better)` | `0.04 * 8` | `Griffin 80%/85%` |
| 8 — Navbar refresh | `+1 (Better)` | `0.02 * 5` | `Sprite 78%/85%` |
| 9 — Search row + activity/collections restyle | `+1 (Better)` | `0.03 * 5` | `Sprite 75%/85%` |

Notes:
- **Task 3** is a pure rename refactor — `0 (Ambiguous)` because it ships no standalone user-visible change (it's only visible once Task 4 lands).
- **Task 4** is the centerpiece — a Phoenix (the old `anime_of_day` card literally rises into the cinematic hero), highest UXΔ.
- Coherence shift stays low across the board (`1–2`): every change extends a documented pattern (5-anchor card recipe, token system, `sort_priority` pinning). The aggregate effort `34` = significant phase-of-work.

---

## File Structure (decomposition)

**Foundation (Task 1):**
- `frontend/web/public/fonts/*.woff2` (new) — self-hosted Manrope/Inter/JetBrains Mono/Noto Sans JP
- `frontend/web/src/styles/main.css` (modify) — `@font-face`, tokens, global base, body gradients
- `frontend/web/index.html` (modify) — font preload

**Featured card rename (Tasks 2–4):**
- `services/catalog/internal/service/spotlight/types.go` (modify) — `AnimeOfDayData` → `FeaturedData`
- `services/catalog/internal/service/spotlight/cards/anime_of_day.go` → rename `featured.go` (modify) — resolver + pin
- `services/catalog/internal/service/spotlight/cards/anime_of_day_test.go` → rename `featured_test.go`
- `services/catalog/cmd/catalog-api/main.go` (modify) — DI
- `frontend/web/src/types/spotlight.ts` (modify) — union + interface rename
- `frontend/web/src/components/home/spotlight/tokens.ts` (modify) — key rename
- `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` (modify) — dispatch rename
- `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.vue` → rename `FeaturedCard.vue` (modify) — transform
- `frontend/web/src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts` → rename `FeaturedCard.spec.ts`
- `frontend/web/src/locales/{ru,en}.json` (modify) — `animeOfDay` → `featured` + new keys

**Restyle + reorder (Tasks 5–9):**
- spotlight `CarouselControls.vue`, `CarouselDots.vue`, other 8 cards (modify)
- `frontend/web/src/components/home/ContinueWatchingRow.vue` (modify)
- `frontend/web/src/views/Home.vue` (modify) — search row, section reorder, column extraction
- `frontend/web/src/components/home/HomeColumn.vue`, `ColumnItem.vue` (new)
- `frontend/web/src/components/layout/Navbar.vue` (modify) + `BrandMark.vue` (new)
- `frontend/web/src/components/home/{ActivityFeed,LastUpdates,CollectionsRow}.vue` (modify)

---

## Task 1: Token & font foundation

**Files:**
- Create: `frontend/web/public/fonts/` (woff2 files)
- Modify: `frontend/web/src/styles/main.css` (the `@theme` block + base styles, top of file)
- Modify: `frontend/web/index.html` (`<head>`)
- Modify: `frontend/web/src/views/Home.vue` (root wrapper class)

- [ ] **Step 1: Vendor the self-hosted font files**

Download WOFF2 for the four families and place under `frontend/web/public/fonts/`. Use the `@fontsource` packages as the source of the files (do not add them as runtime deps — just copy the woff2):
```bash
cd /data/animeenigma/frontend/web
mkdir -p public/fonts
# Pull woff2 from fontsource CDN (build-time fetch, files are then committed locally):
for f in \
  "https://cdn.jsdelivr.net/fontsource/fonts/manrope@latest/latin-700-normal.woff2:manrope-700.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/manrope@latest/latin-800-normal.woff2:manrope-800.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-400-normal.woff2:inter-400.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-500-normal.woff2:inter-500.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-600-normal.woff2:inter-600.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/inter@latest/latin-700-normal.woff2:inter-700.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/jetbrains-mono@latest/latin-400-normal.woff2:jetbrains-mono-400.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/jetbrains-mono@latest/latin-500-normal.woff2:jetbrains-mono-500.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/noto-sans-jp@latest/japanese-400-normal.woff2:noto-sans-jp-400.woff2" \
  "https://cdn.jsdelivr.net/fontsource/fonts/noto-sans-jp@latest/japanese-500-normal.woff2:noto-sans-jp-500.woff2" \
; do url="${f%%:*}"; out="${f##*:}"; curl -fsSL "$url" -o "public/fonts/$out"; done
ls -la public/fonts/
```
Expected: 10 `.woff2` files, each non-zero size. (If a fontsource path 404s, resolve the exact versioned URL from https://fontsource.org and re-fetch — the goal is local woff2 files, however obtained.)

- [ ] **Step 2: Declare `@font-face` + add tokens in `main.css`**

At the **top** of `frontend/web/src/styles/main.css`, immediately after `@import "tailwindcss";`, add the `@font-face` block:
```css
@font-face { font-family: 'Manrope'; font-style: normal; font-weight: 700; font-display: swap; src: url('/fonts/manrope-700.woff2') format('woff2'); }
@font-face { font-family: 'Manrope'; font-style: normal; font-weight: 800; font-display: swap; src: url('/fonts/manrope-800.woff2') format('woff2'); }
@font-face { font-family: 'Inter'; font-style: normal; font-weight: 400; font-display: swap; src: url('/fonts/inter-400.woff2') format('woff2'); }
@font-face { font-family: 'Inter'; font-style: normal; font-weight: 500; font-display: swap; src: url('/fonts/inter-500.woff2') format('woff2'); }
@font-face { font-family: 'Inter'; font-style: normal; font-weight: 600; font-display: swap; src: url('/fonts/inter-600.woff2') format('woff2'); }
@font-face { font-family: 'Inter'; font-style: normal; font-weight: 700; font-display: swap; src: url('/fonts/inter-700.woff2') format('woff2'); }
@font-face { font-family: 'JetBrains Mono'; font-style: normal; font-weight: 400; font-display: swap; src: url('/fonts/jetbrains-mono-400.woff2') format('woff2'); }
@font-face { font-family: 'JetBrains Mono'; font-style: normal; font-weight: 500; font-display: swap; src: url('/fonts/jetbrains-mono-500.woff2') format('woff2'); }
@font-face { font-family: 'Noto Sans JP'; font-style: normal; font-weight: 400; font-display: swap; src: url('/fonts/noto-sans-jp-400.woff2') format('woff2'); }
@font-face { font-family: 'Noto Sans JP'; font-style: normal; font-weight: 500; font-display: swap; src: url('/fonts/noto-sans-jp-500.woff2') format('woff2'); }
```

In the existing `@theme { ... }` block, **change** the two existing colors and **add** the font tokens:
```css
  /* was #121218 / #1a1a24 — global darken per spec D6 */
  --color-base: #08080f;
  --color-surface: #11111c;

  /* Editorial type system (self-hosted) */
  --font-sans: 'Inter', 'Noto Sans JP', system-ui, sans-serif;
  --font-display: 'Manrope', 'Inter', system-ui, sans-serif;
  --font-mono: 'JetBrains Mono', ui-monospace, monospace;
  --font-jp: 'Noto Sans JP', 'Inter', sans-serif;
```

After the `@theme` block (so they are plain custom properties usable in component CSS), add a `:root` block with the semantic tokens the handoff uses:
```css
:root {
  --surface-2: #161623;
  --elevated: #1c1c2c;
  --line: rgba(255, 255, 255, 0.06);
  --line-strong: rgba(255, 255, 255, 0.12);
  --ink: #ffffff;
  --ink-2: rgba(255, 255, 255, 0.78);
  --ink-3: rgba(255, 255, 255, 0.56);
  --ink-4: rgba(255, 255, 255, 0.36);
  --accent: #00d4ff;
  --accent-soft: rgba(0, 212, 255, 0.14);
  --accent-line: rgba(0, 212, 255, 0.28);
  --accent-glow: 0 0 30px rgba(0, 212, 255, 0.28);
  --pink: #ff2d7c;
  --pink-soft: rgba(255, 45, 124, 0.14);
  --violet: #a78bfa;
  --r-sm: 8px; --r-md: 12px; --r-lg: 16px; --r-xl: 22px; --r-2xl: 28px;
  --f-display: var(--font-display);
  --f-ui: var(--font-sans);
  --f-mono: var(--font-mono);
  --f-jp: var(--font-jp);
}
```
(`--success`/`--warning` already exist as `--color-success`/`--color-warning`; reference those in component CSS or alias here if a bare `--success` is cleaner.)

- [ ] **Step 3: Add the global ambient page background**

In `main.css`, find the existing `body { ... }` rule (grep anchor: `background-color: var(--color-base);`) and replace its background with the handoff's layered radials:
```css
body {
  background:
    radial-gradient(1000px 600px at 80% -200px, rgba(0, 212, 255, 0.08), transparent 60%),
    radial-gradient(800px 500px at -10% 10%, rgba(255, 45, 124, 0.06), transparent 60%),
    var(--color-base);
  background-attachment: fixed;
  color: white;
  font-family: var(--font-sans);
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
```

- [ ] **Step 4: Drop the competing gradient on Home.vue root**

In `frontend/web/src/views/Home.vue`, the root element (grep anchor: `bg-gradient-to-b from-gray-900 via-gray-900 to-black`) fights the new global body background. Replace that class with a transparent passthrough:
```html
<div class="min-h-screen">
```

- [ ] **Step 5: Preload the two above-the-fold faces in index.html**

In `frontend/web/index.html`, inside `<head>` (after the existing `<link rel="preconnect" ...>` lines), add:
```html
<link rel="preload" href="/fonts/manrope-800.woff2" as="font" type="font/woff2" crossorigin>
<link rel="preload" href="/fonts/inter-400.woff2" as="font" type="font/woff2" crossorigin>
```

- [ ] **Step 6: Build & verify in browser**

Run:
```bash
cd /data/animeenigma/frontend/web && bun run build
```
Expected: build succeeds, no missing-asset errors for `/fonts/*`.

Then start dev (`make dev` if not running) and open the homepage. In DevTools → Network filter `font`, confirm the woff2 files load (200, not 404). In Console: `getComputedStyle(document.body).fontFamily` includes `Inter`; on a Manrope heading element it resolves to `Manrope`. Confirm the page base is visibly the deeper black with the subtle cyan/pink corner glows.

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma
git add frontend/web/public/fonts frontend/web/src/styles/main.css frontend/web/index.html frontend/web/src/views/Home.vue
git commit -m "feat(home): self-hosted fonts + Neon Tokyo token/base foundation"
```

---

## Task 2: Backend — rename `anime_of_day` → `featured` resolver + curated-pin extension (TDD)

**Files:**
- Modify: `services/catalog/internal/service/spotlight/types.go` (`AnimeOfDayData` → `FeaturedData`)
- Rename+modify: `services/catalog/internal/service/spotlight/cards/anime_of_day.go` → `featured.go`
- Rename+modify: `services/catalog/internal/service/spotlight/cards/anime_of_day_test.go` → `featured_test.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (DI)

- [ ] **Step 1: Rename the data struct in `types.go`**

Find (grep anchor: `// AnimeOfDayData is the payload`) and rename:
```go
// FeaturedData is the payload for `Card{Type: "featured"}` — a single
// hero anime. ReasonI18nKey is optional — omitted from JSON when empty.
type FeaturedData struct {
	Anime         domain.Anime `json:"anime"`
	ReasonI18nKey string       `json:"reason_i18n_key,omitempty"`
}
```
Grep for any other `AnimeOfDayData` references in `services/catalog` and update them.

- [ ] **Step 2: Rename the resolver file & symbols**

```bash
cd /data/animeenigma/services/catalog/internal/service/spotlight/cards
git mv anime_of_day.go featured.go
git mv anime_of_day_test.go featured_test.go
```
In `featured.go`: `AnimeOfDayResolver` → `FeaturedResolver`, `NewAnimeOfDayResolver` → `NewFeaturedResolver`, `minScoreAnimeOfDay` → `minScoreFeatured`, `animeOfDayPoolSize` → `featuredPoolSize`. Change `Type()` to return `"featured"` and the cache key prefix to `spotlight:featured:`.

- [ ] **Step 3: Write the failing test for the curated-pin extension**

In `featured_test.go`, first update the existing tests' resolver/struct names so they compile, then add a pin-priority test. The fake `animeSearcher` returns whatever the test injects; the resolver must consult a pin query first. Add to the `FeaturedResolver` a second search (sort_priority filter) — model it in the fake:
```go
func TestFeaturedResolver_CuratedPinWins(t *testing.T) {
	pin := &domain.Anime{ID: "pin-1", Name: "Pinned Hero", Score: 7.0, SortPriority: 5}
	daily := &domain.Anime{ID: "day-1", Name: "Daily Pick", Score: 9.5}
	repo := &fakeSearcher{
		// pinned query (sort_priority desc, priority>0) returns the pin;
		// the score>=min pool returns the daily candidate.
		byCall: [][]*domain.Anime{{pin}, {daily}},
	}
	r := NewFeaturedResolver(repo, newNoopCache(), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := card.Data.(spotlight.FeaturedData).Anime.ID
	if got != "pin-1" {
		t.Fatalf("expected curated pin to win, got %q", got)
	}
}

func TestFeaturedResolver_FallsBackToDailyWhenNoPin(t *testing.T) {
	daily := &domain.Anime{ID: "day-1", Name: "Daily Pick", Score: 9.5}
	repo := &fakeSearcher{byCall: [][]*domain.Anime{{}, {daily}}} // no pin, then pool
	r := NewFeaturedResolver(repo, newNoopCache(), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if card.Data.(spotlight.FeaturedData).Anime.ID != "day-1" {
		t.Fatalf("expected daily fallback")
	}
}
```
Adjust the existing `fakeSearcher` to support sequential per-call returns (`byCall [][]*domain.Anime` + a call counter), matching the handwritten-fake pattern already in the file. Reuse the existing test helpers (`newNoopCache`, `testLogger`) — if the current file names them differently, keep those names.

- [ ] **Step 4: Run the test to verify it fails**

Run:
```bash
cd /data/animeenigma/services/catalog && go test ./internal/service/spotlight/cards/ -run TestFeaturedResolver -v
```
Expected: FAIL — `CuratedPinWins` returns `day-1` (no pin logic yet) or compile error referencing the new fake fields.

- [ ] **Step 5: Implement the pin selection in `featured.go`**

In `Resolve`, after the cache-miss branch and before the daily-seeded pick, add the pin query (reuse the existing `domain.SearchFilters` shape):
```go
	// --- Curated pin (sort_priority) wins when present -------------------
	one := 1
	pinned, _, perr := r.repo.Search(ctx, domain.SearchFilters{
		Sort:            "sort_priority",
		Order:           "desc",
		SortPriorityMin: &one, // only sort_priority > 0
		Page:            1,
		PageSize:        1,
	})
	if perr == nil && len(pinned) > 0 {
		data := spotlight.FeaturedData{Anime: *pinned[0]}
		if err := r.cache.Set(ctx, key, data, cardTTL); err != nil {
			r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", key, "error", err)
		}
		return &spotlight.Card{Type: r.Type(), Data: data}, nil
	}
```
Then change the daily-pick path to build `spotlight.FeaturedData` (not `AnimeOfDayData`).

**If `domain.SearchFilters` has no `SortPriorityMin` field:** check `services/catalog/internal/domain` (grep `type SearchFilters struct`). If absent, add `SortPriorityMin *int` to the struct and honor it in the repo's `Search` query builder (grep the repo for `sort_priority` to find where ordering is applied; add `WHERE sort_priority >= ?` when the filter is set). Keep that change minimal and covered by the test above.

- [ ] **Step 6: Run the test to verify it passes**

Run:
```bash
cd /data/animeenigma/services/catalog && go test ./internal/service/spotlight/... -count=1 -race
```
Expected: PASS (all spotlight tests, including the renamed existing ones).

- [ ] **Step 7: Update DI in `main.go`**

In `services/catalog/cmd/catalog-api/main.go`, find `cards.NewAnimeOfDayResolver(animeRepo, redisCache, log),` (in the `spotlightResolvers := []spotlight.Resolver{` slice) and rename to:
```go
		cards.NewFeaturedResolver(animeRepo, redisCache, log),
```
Keep it as the **first** entry in the slice (default-slide display order).

- [ ] **Step 8: Verify backend builds & no stale references**

Run:
```bash
cd /data/animeenigma/services/catalog && go build ./... && go vet ./internal/service/spotlight/...
grep -rn "anime_of_day\|AnimeOfDay" services/catalog/  # expect: zero
```
Expected: builds clean; grep returns nothing.

- [ ] **Step 9: Commit**

```bash
cd /data/animeenigma
git add services/catalog
git commit -m "feat(catalog): rename anime_of_day spotlight resolver to featured + curated-pin selection"
```

---

## Task 3: Frontend — rename plumbing (types, dispatch, tokens, i18n)

**Files:**
- Modify: `frontend/web/src/types/spotlight.ts`
- Modify: `frontend/web/src/components/home/spotlight/tokens.ts`
- Modify: `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`
- Modify: `frontend/web/src/locales/ru.json`, `frontend/web/src/locales/en.json`

- [ ] **Step 1: Rename the type + union member**

In `frontend/web/src/types/spotlight.ts`:
- Rename `export interface AnimeOfDayData` → `export interface FeaturedData` (keep the `anime` + `reason_i18n_key?` shape).
- In the `SpotlightCard` union, change `| { type: 'anime_of_day'; data: AnimeOfDayData }` → `| { type: 'featured'; data: FeaturedData }`.
- Update the comment "Shared anime sub-shape used by anime_of_day + random_tail" → "...featured + random_tail".

- [ ] **Step 2: Rename the token key**

In `frontend/web/src/components/home/spotlight/tokens.ts`:
- In the token map type, `anime_of_day: CardToken & { genreColors: ... }` → `featured: CardToken & { genreColors: ... }`.
- In the token values, rename the `anime_of_day: { accent: 'cyan', kickerKey: 'spotlight.animeOfDay.title', icon: 'sparkles', ... }` key to `featured:` and set `kickerKey: 'spotlight.featured.title'`.
- Grep the file for any remaining `anime_of_day` and fix.

- [ ] **Step 3: Rename the dispatch branch + switch case in HeroSpotlightBlock.vue**

In `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`:
- Import line: `import AnimeOfDayCard from './cards/AnimeOfDayCard.vue'` → `import FeaturedCard from './cards/FeaturedCard.vue'`.
- The template branch:
```html
          <FeaturedCard
            v-if="active.type === 'featured'"
            :key="`featured:${currentIndex}`"
            :data="active.data"
          />
```
- In `cardTitle(card)`, change `case 'anime_of_day':` → `case 'featured':` (keep it grouped with `'random_tail'`).

- [ ] **Step 4: Move the i18n namespace + add new keys (RU)**

In `frontend/web/src/locales/ru.json`, rename the `"animeOfDay": { ... }` block (grep anchor: `"animeOfDay": {`) to `"featured":` and extend it with the status-aware strings:
```json
    "featured": {
      "title": "Рекомендуем сегодня",
      "eyebrowOngoing": "Сейчас выходит",
      "eyebrowAnnounced": "Анонс сезона",
      "eyebrowDefault": "Рекомендуем сегодня",
      "watchEpisode": "Смотреть · эп. {n}",
      "watchCta": "Начать просмотр",
      "remindCta": "Напомнить о выходе",
      "addCta": "В список",
      "scoreLabel": "Рейтинг",
      "episodesLabel": "{n} эп."
    },
```

- [ ] **Step 5: Move the i18n namespace + add new keys (EN)**

In `frontend/web/src/locales/en.json`, the matching block:
```json
    "featured": {
      "title": "Featured today",
      "eyebrowOngoing": "Now airing",
      "eyebrowAnnounced": "Season announcement",
      "eyebrowDefault": "Featured today",
      "watchEpisode": "Watch · ep. {n}",
      "watchCta": "Start watching",
      "remindCta": "Remind me",
      "addCta": "Add to list",
      "scoreLabel": "Score",
      "episodesLabel": "{n} ep."
    },
```
(Both files must have the **same keys** — the parity test enforces this. Drop the old `addCtaComingSoon` key since the disabled add button is gone.)

- [ ] **Step 6: Verify types + locale parity (the card itself still references old name — that's Task 4)**

The component file rename happens in Task 4; this step only checks the plumbing compiles against the *renamed* card path. To keep the tree green between tasks, do Task 4 Step 1 (the `git mv` of the card file) **now** if executing strictly task-by-task — or accept a transient red until Task 4. Then run:
```bash
cd /data/animeenigma/frontend/web && bunx vitest run src/locales/__tests__/spotlight-keys.spec.ts
```
Expected: PASS (RU/EN `spotlight.featured.*` parity holds).

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/types/spotlight.ts frontend/web/src/components/home/spotlight/tokens.ts frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue frontend/web/src/locales/ru.json frontend/web/src/locales/en.json
git commit -m "refactor(spotlight): rename anime_of_day type/dispatch/tokens/i18n to featured"
```

---

## Task 4: FeaturedCard.vue — cinematic full-bleed transform (TDD for logic)

**Files:**
- Rename+modify: `cards/AnimeOfDayCard.vue` → `cards/FeaturedCard.vue`
- Rename+modify: `cards/AnimeOfDayCard.spec.ts` → `cards/FeaturedCard.spec.ts`
- Reference (exact CSS values): handoff `styles.css` `.hero`, `.hero-bg`, `.hero-content`, `.hero-eyebrow`, `.hero-meta`, `.hero-desc`, `.btn-primary`, `.btn-secondary`.

- [ ] **Step 1: Rename the files**

```bash
cd /data/animeenigma/frontend/web/src/components/home/spotlight/cards
git mv AnimeOfDayCard.vue FeaturedCard.vue
git mv AnimeOfDayCard.spec.ts FeaturedCard.spec.ts
```

- [ ] **Step 2: Write the failing spec for status-aware eyebrow + CTA**

Replace the body of `FeaturedCard.spec.ts` with assertions on the status→label/CTA mapping. Use the existing test setup pattern in the file (i18n mock + `mount`). Real assertions (≥5):
```ts
import { mount } from '@vue/test-utils'
import { describe, it, expect } from 'vitest'
import FeaturedCard from './FeaturedCard.vue'
import { i18n } from '@/test/i18n' // use whatever the sibling specs import; else build a minimal i18n

function mk(status: string, episodes_aired = 0) {
  return { anime: { id: 'a1', name: 'Test', name_ru: 'Тест', name_jp: 'テスト', status, episodes_aired, score: 8.4, year: 2026, episodes_count: 12, genres: [] } }
}

describe('FeaturedCard', () => {
  it('shows "now airing" eyebrow + episode CTA for ongoing', () => {
    const w = mount(FeaturedCard, { props: { data: mk('ongoing', 7) }, global: { plugins: [i18n] } })
    expect(w.text()).toContain('Now airing') // en locale
    expect(w.text()).toContain('ep. 8')      // aired+1
  })
  it('shows announcement eyebrow + remind CTA for announced', () => {
    const w = mount(FeaturedCard, { props: { data: mk('announced') }, global: { plugins: [i18n] } })
    expect(w.text()).toContain('Season announcement')
    expect(w.text()).toContain('Remind me')
  })
  it('shows default eyebrow + start CTA for released', () => {
    const w = mount(FeaturedCard, { props: { data: mk('released') }, global: { plugins: [i18n] } })
    expect(w.text()).toContain('Featured today')
    expect(w.text()).toContain('Start watching')
  })
  it('renders the JP subtitle and score', () => {
    const w = mount(FeaturedCard, { props: { data: mk('released') }, global: { plugins: [i18n] } })
    expect(w.text()).toContain('テスト')
    expect(w.text()).toContain('8.4')
  })
  it('links the primary CTA to the watch route', () => {
    const w = mount(FeaturedCard, { props: { data: mk('ongoing', 7) }, global: { plugins: [i18n] } })
    expect(w.find('a[href*="/anime/a1"]').exists()).toBe(true)
  })
})
```
Match the i18n import/locale to the sibling specs (e.g. `RandomTailCard.spec.ts`) so the locale is set to `en` for these assertions.

- [ ] **Step 3: Run the spec to verify it fails**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx vitest run src/components/home/spotlight/cards/FeaturedCard.spec.ts
```
Expected: FAIL — old card markup has no status-aware eyebrow/CTA.

- [ ] **Step 4: Rewrite FeaturedCard.vue as the cinematic hero**

Replace the SFC. Script computes status-aware eyebrow + CTA; template is full-bleed per handoff `styles.css`. Use the new tokens via inline style/CSS classes; honor UI-SPEC weights (`font-medium`/`font-semibold` only) and the spotlight frame height.
```vue
<template>
  <article class="featured-hero">
    <div class="featured-bg" :style="{ backgroundImage: posterBg }" />
    <div class="featured-content">
      <p class="featured-eyebrow">
        <span class="pulse" aria-hidden="true" />
        {{ eyebrow }}
        <template v-if="data.anime.season"><span class="sep">·</span>{{ data.anime.season }}</template>
      </p>
      <h1 class="featured-title">
        {{ getLocalizedTitle(data.anime.name, data.anime.name_ru, data.anime.name_jp) }}
        <span v-if="data.anime.name_jp" class="jp">{{ data.anime.name_jp }}</span>
      </h1>
      <div class="featured-meta">
        <span v-if="data.anime.score" class="score">
          <SpotlightIcon name="star" class="w-3.5 h-3.5" /> {{ data.anime.score.toFixed(1) }}
        </span>
        <span v-if="data.anime.year">{{ data.anime.year }}</span>
        <span v-if="data.anime.episodes_count" class="dot" />
        <span v-if="data.anime.episodes_count">{{ t('spotlight.featured.episodesLabel', { n: data.anime.episodes_count }) }}</span>
        <span v-for="g in (data.anime.genres || []).slice(0, 3)" :key="g.id" class="chip-genre">
          {{ locale === 'ru' ? (g.russian || g.name) : (g.name || g.russian) }}
        </span>
      </div>
      <p v-if="data.anime.description" class="featured-desc">{{ data.anime.description }}</p>
      <div class="featured-actions">
        <router-link :to="watchTo" class="btn-primary-hero">
          <SpotlightIcon name="play" class="w-4 h-4" /> {{ primaryCta }}
        </router-link>
        <router-link :to="`/anime/${data.anime.id}`" class="btn-secondary-hero">
          {{ t('spotlight.featured.addCta') }}
        </router-link>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import type { FeaturedData } from '@/types/spotlight'
import SpotlightIcon from '../SpotlightIcon.vue'

const props = defineProps<{ data: FeaturedData }>()
const { t, locale: i18nLocale } = useI18n()
const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const posterBg = computed(() =>
  props.data.anime.poster_url ? `url("${props.data.anime.poster_url}")` : 'none',
)
const eyebrow = computed(() => {
  switch (props.data.anime.status) {
    case 'ongoing': return t('spotlight.featured.eyebrowOngoing')
    case 'announced': return t('spotlight.featured.eyebrowAnnounced')
    default: return t('spotlight.featured.eyebrowDefault')
  }
})
const primaryCta = computed(() => {
  switch (props.data.anime.status) {
    case 'ongoing': return t('spotlight.featured.watchEpisode', { n: (props.data.anime.episodes_aired || 0) + 1 })
    case 'announced': return t('spotlight.featured.remindCta')
    default: return t('spotlight.featured.watchCta')
  }
})
const watchTo = computed(() =>
  props.data.anime.status === 'announced'
    ? `/anime/${props.data.anime.id}`
    : `/anime/${props.data.anime.id}/watch`,
)
</script>

<style scoped>
/* Values copied verbatim from handoff styles.css (.hero* rules). */
.featured-hero { position: relative; width: 100%; height: 100%; overflow: hidden; }
.featured-bg { position: absolute; inset: 0; background-size: cover; background-position: center 30%; filter: saturate(105%); }
.featured-bg::after {
  content: ""; position: absolute; inset: 0;
  background:
    linear-gradient(90deg, rgba(8,8,15,.92) 0%, rgba(8,8,15,.72) 35%, rgba(8,8,15,.25) 65%, rgba(8,8,15,0) 100%),
    linear-gradient(180deg, rgba(8,8,15,0) 50%, rgba(8,8,15,.65) 100%);
}
.featured-content {
  position: absolute; inset: 0; z-index: 1;
  display: flex; flex-direction: column; justify-content: flex-end;
  padding: 40px 48px; gap: 20px; max-width: 720px;
}
.featured-eyebrow {
  display: inline-flex; align-items: center; gap: 10px;
  font-family: var(--f-mono); font-size: 11px; letter-spacing: .12em;
  text-transform: uppercase; color: var(--accent);
}
.featured-eyebrow .pulse { width: 6px; height: 6px; border-radius: 999px; background: var(--accent); box-shadow: 0 0 8px var(--accent); animation: featured-pulse 1.6s ease-in-out infinite; }
.featured-eyebrow .sep { opacity: .5; }
@keyframes featured-pulse { 0%,100% { opacity: 1; transform: scale(1); } 50% { opacity: .5; transform: scale(.8); } }
.featured-title { font-family: var(--f-display); font-weight: 800; font-size: clamp(36px, 4vw, 56px); line-height: 1.02; letter-spacing: -.025em; text-wrap: balance; }
.featured-title .jp { display: block; font-family: var(--f-jp); font-weight: 500; font-size: .42em; letter-spacing: .02em; color: var(--ink-3); margin-top: 8px; }
.featured-meta { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; color: var(--ink-3); font-size: 13px; }
.featured-meta .dot { width: 3px; height: 3px; border-radius: 999px; background: currentColor; opacity: .4; }
.featured-meta .score { display: inline-flex; align-items: center; gap: 6px; color: var(--color-warning); font-weight: 600; }
.featured-meta .chip-genre { padding: 4px 10px; border-radius: 999px; border: 1px solid var(--line-strong); font-size: 12px; color: var(--ink-2); }
.featured-desc { font-size: 15px; line-height: 1.6; color: var(--ink-2); max-width: 540px; text-wrap: pretty; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
.featured-actions { display: flex; gap: 10px; align-items: center; }
.btn-primary-hero { display: inline-flex; align-items: center; gap: 10px; padding: 14px 22px; background: var(--accent); color: #001218; border-radius: 12px; font-weight: 700; font-size: 14px; transition: filter .15s ease, box-shadow .15s ease; }
.btn-primary-hero:hover { filter: brightness(1.08); box-shadow: var(--accent-glow); }
.btn-secondary-hero { display: inline-flex; align-items: center; gap: 10px; padding: 14px 22px; background: rgba(255,255,255,.06); border: 1px solid var(--line-strong); border-radius: 12px; font-weight: 600; font-size: 14px; color: var(--ink); }
.btn-secondary-hero:hover { background: rgba(255,255,255,.1); }
@media (max-width: 640px) { .featured-content { padding: 24px; } }
</style>
```

- [ ] **Step 5: Run the spec to verify it passes**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx vitest run src/components/home/spotlight/cards/FeaturedCard.spec.ts
```
Expected: PASS (all 5 assertions).

- [ ] **Step 6: Type-check + regenerate snapshots**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit
bunx vitest run src/components/home/spotlight/ -u   # update snapshots for renamed card
grep -rn "anime_of_day\|AnimeOfDay\|animeOfDay" frontend/web/src/  # expect: zero
```
Expected: tsc clean; snapshots updated; grep returns nothing.

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/home/spotlight/cards/
git commit -m "feat(spotlight): transform featured card into full-bleed cinematic hero"
```

---

## Task 5: Carousel chrome + remaining 8-card restyle

**Files:**
- Modify: `frontend/web/src/components/home/spotlight/CarouselControls.vue`, `CarouselDots.vue`
- Modify: the other 8 card SFCs under `cards/` (notably `PlatformStatsCard.vue`)
- Reference: handoff `styles.css` `.arrow-btn`, `.dot-btn`, `.hero-stats`, `.stat-tile`.

- [ ] **Step 1: Restyle carousel arrows + dots**

In `CarouselControls.vue`, style the prev/next buttons to the handoff `.arrow-btn` (36×36 round, `rgba(8,8,15,0.7)` bg, `blur(8px)`, `--line` border, positioned 20px from sides, vertically centered). In `CarouselDots.vue`, style dots to `.dot-btn` (26×4 inactive `rgba(255,255,255,.16)`; active 36×4 `--accent` with `0 0 10px` glow), positioned bottom-right (24/28). Keep all existing props/emits/aria.

- [ ] **Step 2: Restyle PlatformStatsCard to the two-column `hero-stats` variant**

Match handoff `.hero-stats` (grid `1.1fr 1fr`, radial+linear bg) with left column ("Работает: ДА" h2 48px Manrope 800, uptime mono line, ASCII tree `<pre>`, cyan-left-border quote) and right 2×2 `.stat-tile` grid (mono label, 32px value, sub, top-right radial highlight). Keep the existing real data bindings (`working_ok`, `uptime_percent`, tiles) — only the presentation changes. Reuse i18n keys already present under `spotlight.platformStats`.

- [ ] **Step 3: Restyle the remaining 6 cards to the new tokens**

For `RandomTailCard`, `LatestNewsCard`, `PersonalPickCard`, `TelegramNewsCard`, `NowWatchingCard`, `NotTimeYetCard`: swap hardcoded grays/cyans for the semantic tokens (`--ink-*`, `--line`, `--accent*`, `--r-*`), match card padding (`p-4 md:p-6 lg:p-8`) and the spotlight frame height. Do not change data or props.

- [ ] **Step 4: Verify**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx vitest run src/components/home/spotlight/ && bunx tsc --noEmit
```
Expected: PASS (regenerate snapshots with `-u` if the restyle changed rendered class strings).
In-browser: cycle the carousel through all card types; arrows/dots match the mock; PlatformStats reads as the two-column stats panel.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/home/spotlight/
git commit -m "style(spotlight): restyle carousel chrome + all card types to Neon Tokyo tokens"
```

---

## Task 6: Continue Watching — promote above grid + 16:9 restyle

**Files:**
- Modify: `frontend/web/src/views/Home.vue` (move the `<ContinueWatchingRow />` tag)
- Modify: `frontend/web/src/components/home/ContinueWatchingRow.vue`
- Reference: handoff `styles.css` `.section-head`, `.cw-row`, `.cw-card`.

- [ ] **Step 1: Move ContinueWatchingRow above the 3-column grid**

In `Home.vue`: the current order is `HeroSpotlightBlock` → 3-column grid → `CollectionsRow` → `ContinueWatchingRow` → activity (grep anchors: `<ContinueWatchingRow />`, `<CollectionsRow />`, and the `<!-- Three Columns Layout -->` wrapper). Cut the `<ContinueWatchingRow />` block and paste it **immediately after `<HeroSpotlightBlock />`** and before the Three-Columns wrapper. Resulting order: Hero → Continue Watching → 3-col grid → Collections → Activity.

- [ ] **Step 2: Restyle the CW cards to 16:9 cinematic**

In `ContinueWatchingRow.vue`, match handoff `.cw-row` (horizontal scroll, `grid-auto-flow: column; grid-auto-columns: minmax(280px,1fr)`, snap) and `.cw-card` (16:9, rounded-16, image + bottom scrim, centered 52px cyan play button revealed on hover scale 1.06, episode label mono cyan, title single-line ellipsis, time-left, bottom 3px cyan progress bar with glow, hover lift -2px + accent border + glow). Keep existing routing (`/anime/:id?episode=N`), the `useContinueWatching` data, and the empty-state self-hide.

- [ ] **Step 3: Section header**

Match `.section-head` — h2 22px Manrope 700 "Продолжить просмотр" + mono count badge + right-aligned "Вся история →" link (route to history). Use existing i18n keys (`home.continueWatching`); add `home.continueWatchingAll` ("Вся история"/"All history") to both locales if missing.

- [ ] **Step 4: Verify**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/home/
```
In-browser (logged in as `ui_audit_bot` per CLAUDE.md): CW row sits directly under the hero; cards are 16:9 with working play-on-hover + progress bar.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/views/Home.vue frontend/web/src/components/home/ContinueWatchingRow.vue frontend/web/src/locales/ru.json frontend/web/src/locales/en.json
git commit -m "feat(home): promote Continue Watching above grid + 16:9 cinematic cards"
```

---

## Task 7: 3-column grid reframe (extract HomeColumn + ColumnItem)

**Files:**
- Create: `frontend/web/src/components/home/HomeColumn.vue`, `frontend/web/src/components/home/ColumnItem.vue`
- Modify: `frontend/web/src/views/Home.vue` (replace inline column markup)
- Modify: `frontend/web/src/locales/{ru,en}.json` (next-ep line key)
- Reference: handoff `styles.css` `.col`, `.col-head`, `.item`, `.chip*`, `.next-ep`, `.rank`.

- [ ] **Step 1: Create ColumnItem.vue**

A single row matching handoff `.item` (grid `56px 1fr`, gap 12, padding 10, rounded-12, hover bg + title→accent; poster 2:3 rounded-8). Props:
```ts
defineProps<{
  anime: HomeAnime          // from stores/home
  variant: 'ongoing' | 'top' | 'announced'
  rank?: number             // top only
}>()
```
Render rules by variant:
- `ongoing`: `● Выходит` success chip + optional score chip + **next-ep line** (`home.nextEpisodeLine` with `{n}` + formatted `next_episode_at`, mono cyan + clock icon) when `next_episode_at` set.
- `top`: score chip + giant `.rank` numeral (Manrope 800 56px, abs right-center, `rgba(255,255,255,.04)`; add `top-3` class → `rgba(0,212,255,.08)` when `rank <= 3`).
- `announced`: `Анонс` cyan chip + violet season chip.
Reuse `getLocalizedTitle` and the existing kebab/context-menu integration (carry over the `AnimeKebab` + `@open` handler currently inline in Home.vue so the context menu still works).

- [ ] **Step 2: Create HomeColumn.vue**

The column shell matching handoff `.col` + `.col-head`: gradient bg, `--line` border, rounded-22, padding 18; header = 36×36 semantic icon tile (`green`/`gold`/`blue`) + h3 17px + mono sub + right "Все" pill link. Props:
```ts
defineProps<{
  title: string
  sub?: string
  iconTone: 'green' | 'gold' | 'blue'
  seeAllTo: string
  loading: boolean
}>()
```
Slot the items; render the existing skeleton when `loading`.

- [ ] **Step 3: Replace inline column markup in Home.vue**

Swap the three inline column `<div>`s (grep anchor: `<!-- Three Columns Layout -->`) for three `<HomeColumn>` instances wrapping `v-for` `<ColumnItem>`. Keep the `useHomeStore` data (`ongoingAnime`, `topAnime`, `announcedAnime`, `siteRatings`, `loading*`) and pass `rank="index+1"` for the top column. Keep `AnimeContextMenu` mounted once at the bottom.

- [ ] **Step 4: Add the next-ep i18n key**

In both locales, under `home`, add:
```json
"nextEpisodeLine": "Серия {n} · {when}"   // ru
"nextEpisodeLine": "Episode {n} · {when}" // en
```
(`{when}` is produced by the existing `formatUpdatedAt`/schedule formatter already in Home.vue — reuse it; if a dedicated next-ep formatter is cleaner, add a small `formatNextEp` util.)

- [ ] **Step 5: Verify**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/home/
```
In-browser: Ongoing shows the cyan next-ep line; Top shows giant background rank numerals (1–3 tinted); Announcements shows violet season chips; kebab context menu still opens on each item.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/home/HomeColumn.vue frontend/web/src/components/home/ColumnItem.vue frontend/web/src/views/Home.vue frontend/web/src/locales/ru.json frontend/web/src/locales/en.json
git commit -m "refactor(home): extract HomeColumn/ColumnItem + reframe 3-column grid (next-ep, rank, season)"
```

---

## Task 8: Navbar refresh

**Files:**
- Create: `frontend/web/src/components/layout/BrandMark.vue`
- Modify: `frontend/web/src/components/layout/Navbar.vue`
- Reference: handoff `styles.css` `.brand`, `.nav-link`, `.nav-link.active::after`, `.lang-pill`, `.icon-btn`, `.avatar`.

- [ ] **Step 1: Create BrandMark.vue**

A 28×28 rounded-8 square with cyan→pink linear gradient, a 4px inset cutout showing `--color-base`, "AE" centered (cyan 11px Manrope 800), accent glow — matching handoff `.brand .mark`. Pure presentational, no props.

- [ ] **Step 2: Wire BrandMark + wordmark into Navbar**

Replace the current text-only logo (grep anchor: the `"Anime"` + `"Enigma"` spans) with `<BrandMark />` + the wordmark (`Anime` cyan / `Enigma` white, Manrope 800 18px).

- [ ] **Step 3: Animated active-link underline**

Style nav links to handoff `.nav-link` (14px Inter 500, inactive `--ink-3`, hover `--ink`); active link gets the 2px cyan underline pill 16px below with `0 0 10px` glow and a `0.2s ease` transition. Keep the existing `router-link active-class` mechanism — just change the visual.

- [ ] **Step 4: Right-side tools**

Match handoff: search icon button → lang pill (`RU ▾`, reuse the existing language dropdown) → bell with 6px pink notification dot (wire the dot to the real unread count if the notifications store is available; otherwise render the bell without the dot — do not fake a count) → 36×36 avatar with success online dot. Icon buttons 36×36 rounded-10, hover `rgba(255,255,255,.05)`. Preserve theater-mode hide and existing mobile/hamburger behavior.

- [ ] **Step 5: Verify**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/layout/ 2>/dev/null || true
```
In-browser: brand mark renders with glow; active route shows the animated cyan underline; tools row matches the mock; mobile hamburger still works.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/layout/
git commit -m "feat(navbar): brand mark + animated active underline + Neon Tokyo tools row"
```

---

## Task 9: Search row, Activity + Collections restyle, full-page smoke

**Files:**
- Modify: `frontend/web/src/views/Home.vue` (search row)
- Modify: `frontend/web/src/components/home/ActivityFeed.vue`, `LastUpdates.vue`, `CollectionsRow.vue`
- Reference: handoff `styles.css` `.search-row`, `.search`, `.btn-ghost-accent`, `.activity`, `.feed-item`, `.update-row`.

- [ ] **Step 1: Restyle the search row**

In `Home.vue`, match handoff `.search-row` (grid `1fr auto`, gap 12): the `SearchAutocomplete` wrapper to `.search` (56px, rounded-16, `⌘K` kbd, focus→`--accent-line`), and the Расписание link to `.btn-ghost-accent` (56px, `--accent-soft`/`--accent-line`/`--accent`). Keep the component's existing behavior.

- [ ] **Step 2: Restyle ActivityFeed + LastUpdates**

Match handoff `.activity` shell + `.feed-item` (28px avatar, `@user` bold + action + accent title + optional italic excerpt + mono time) and `.update-row` (36×48 thumb + title/sub + right mono "when", hover bg). Data bindings unchanged.

- [ ] **Step 3: Restyle CollectionsRow**

Apply the new card tokens (`--line`, `--r-*`, surface) to the collection cards; keep cover-fallback gradient, title overlay, item-count meta, and empty-state self-hide.

- [ ] **Step 4: Full-page in-browser smoke (the real acceptance gate)**

Start dev, log in as `ui_audit_bot`, open `/`. Verify against the spec:
- Fonts loaded (Network: woff2 200s); no FOIT flash beyond `swap`.
- Section order: Search → Hero(featured default) → Continue Watching → 3-col grid → Collections → Activity.
- Featured hero renders full-bleed with status-aware eyebrow + CTA; carousel cycles (7s) and pauses on hover.
- 3-column: next-ep line / rank numerals / season chips correct.
- **No raw i18n key strings** anywhere (i18n smoke per `feedback_smoke_verify_i18n.md`). Toggle RU/EN and re-check.
- **Cascade-trap check** (`reference_tailwind_v4_css_cascade.md`): confirm `md:hidden`/responsive utilities still win where expected and no custom class is bleeding over a utility. Resize to 1280 / 960 / 720 widths.
- Console: no errors. `grep -rn "anime_of_day" frontend/ services/` returns zero.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/views/Home.vue frontend/web/src/components/home/ActivityFeed.vue frontend/web/src/components/home/LastUpdates.vue frontend/web/src/components/home/CollectionsRow.vue
git commit -m "style(home): restyle search row, activity feed, last updates, collections"
```

- [ ] **Step 6: Deploy-time migration note (record, do not run here)**

When the user later deploys: redeploy `catalog` + `web`, then **flush stale spotlight cache** so no `anime_of_day`-keyed JSON lingers:
```bash
docker compose -f docker/docker-compose.yml exec -T redis redis-cli --scan --pattern 'spotlight:*' | xargs -r docker compose -f docker/docker-compose.yml exec -T redis redis-cli DEL
```
Then runtime-smoke `GET /api/home/spotlight` and confirm a `"type":"featured"` card appears. (This lives in the `/animeenigma-after-update` pass the user triggers — not part of plan execution.)

---

## Self-Review

**Spec coverage:**
- §4 tokens/fonts → Task 1 ✓
- §5.1 navbar → Task 8 ✓
- §5.2 search row → Task 9 Step 1 ✓
- §5.3 featured rename + transform + carousel/8-card restyle → Tasks 2,3,4,5 ✓
- §5.4 CW promote+restyle → Task 6 ✓
- §5.5 3-column reframe → Task 7 ✓
- §5.6 activity → Task 9 Step 2 ✓
- §5.7 collections → Task 9 Step 3 ✓
- §5.8 reorder → Task 6 Step 1 ✓
- §6 i18n → Tasks 3,6,7 ✓
- §7 backend resolver+pin → Task 2 ✓
- §8 testing → per-task verify steps + Task 9 Step 4 ✓
- §9 deferred side panel → not built (correct; out of plan) ✓
- §4.4 cascade guard → Task 9 Step 4 ✓
- Migration (flush spotlight:*) → Task 9 Step 6 ✓

**Type consistency:** `FeaturedData` (Go + TS), `FeaturedResolver`, `NewFeaturedResolver`, `Type()="featured"`, union member `{ type: 'featured' }`, `tokens.featured`, `spotlight.featured.*` — consistent across Tasks 2–4. Component named `FeaturedCard` everywhere.

**Open implementation risks to watch (flagged, not blocking):**
- Task 2 Step 5: `SearchFilters.SortPriorityMin` may not exist — step includes the fallback to add it.
- Task 4 Step 2: the spec's i18n test import must match the sibling specs' pattern — step says to mirror `RandomTailCard.spec.ts`.
- Task 1 Step 1: fontsource CDN URLs may need version pinning — step says resolve the exact URL if a path 404s.
