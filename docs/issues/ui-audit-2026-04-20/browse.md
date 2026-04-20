# Browse mobile audit — 2026-04-20 — viewport 500x723

## State verification

- `<h1>Каталог</h1>` present ✓
- `document.title` = "Каталог - AnimeEnigma" ✓ (Batch F UA-040 landed)
- **UA-039 verified** — pagination buttons all have `aria-label`: "Страница 1/2/190", "Предыдущая страница", "Следующая страница", "Страница 1" has `aria-current="page"`. Fix landed cleanly.
- SearchAutocomplete on Browse shares the same combobox-on-wrapper bug as Home — see UA-044 in home.md

## axe-core (mobile 500x723)

- 38 passes, **2 violations** — down from 2 (desktop 2026-04-17 also = 2). Delta = +0.
- `color-contrast` serious × 1 node — `.text-white/30` placeholder on Genre filter trigger (2.7:1)
- `heading-order` moderate × 1 node — h3 card title under h1 with no h2

## NEW findings

### [UA-046] Browse — Genre filter placeholder `.text-white/30` fails 4.5:1 contrast — Severity 2 (major) — accessibility

**View:** `/browse` — visible every page load until Genre is chosen
**Heuristic:** WCAG 2.1 SC 1.4.3
**Evidence:**
- axe `color-contrast` serious, 1 node. fg `#626166`, bg `#1e1e24`, ratio **2.7:1** (need 4.5:1)
- DOM: `<button class="bg-white/5 border text-white ...">` → `<span class="text-white/30 truncate">Жанр</span>`
- Other three filter dropdowns on the same row ("Статус", "Год", "Популярные") use regular `text-white` (full opacity) for their placeholders and are readable
- Only the Genre filter uses the dim variant

**Why it matters:** Users with low-contrast sensitivity or bright-ambient conditions cannot see the Genre filter label. Since it's the first filter in the row, it's also a first-impression issue.

**Citations:**
- `frontend/web/src/components/ui/GenreFilterPopup.vue — found via grep "text-white/30"`

**Proposed fix:** Bump placeholder span to `text-white/60` (matches the ThemeCard fix in Batch D). Single class swap.

### [UA-047] Browse — Genre filter trigger missing `aria-haspopup` + `aria-expanded` — Severity 1 (minor) — accessibility

**View:** `/browse`
**Heuristic:** WCAG 4.1.2 + ARIA combobox pattern
**Evidence:**
- Genre filter `<button>Жанр</button>` has `aria-expanded=null`, `aria-haspopup=null`
- Sibling filters on the same view (Status/Year/Popularity) all have `aria-haspopup="listbox"` + `aria-expanded="false|true"` — inconsistency within a single view
- The popup opens an actual list, so the attrs are meaningful

**Citations:**
- `frontend/web/src/components/ui/GenreFilterPopup.vue — found via grep "<button"`

**Proposed fix:** Add `aria-haspopup="dialog"` (or `"listbox"`) and `:aria-expanded="isOpen"` on the trigger `<button>`.

### [UA-048] Browse — h1 → h3 heading-order jump (same pattern as Themes/Profile UA-041) — Severity 1 (minor) — accessibility

**View:** `/browse`
**Heuristic:** WCAG 1.3.1
**Evidence:**
- axe `heading-order` moderate, 1 node
- h1 "Каталог" then all card titles are h3 — no h2 in between
- Matches the UA-041 pattern that was fixed only on Themes + Profile (Batch F); Browse was not in the Batch F scope

**Citations:**
- `frontend/web/src/views/Browse.vue — found via grep "AnimeCard\|<h3"` (card component)
- Likely fix point: `frontend/web/src/components/anime/AnimeCard.vue`

**Proposed fix:** Either demote AnimeCard title to h3 → h2 (breaks many views), OR inject a sr-only `<h2>` between the h1 and the results grid on Browse. The sr-only route is the smaller blast radius — `<h2 class="sr-only">{{ $t('browse.results') }}</h2>` above the results grid.

### [UA-049] Browse — Home-only hamburger aria-label still in English here — same as UA-043

Not duplicated — see home.md. Applies to every view.
