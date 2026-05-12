# Themes / Schedule / Game / Navbar — per-view findings

**Audit mode:** Code-based (browser disconnected after first 5 views; carry-over verified via source greps).
**Source pass date:** 2026-05-12

## Themes.vue (`frontend/web/src/views/Themes.vue` — 314 lines)

| ID | Title | Severity |
|---|---|---|
| **UA-074** | "No themes" empty-state text uses `text-white/40` (contrast ~3.16:1) | 1 minor (a11y) |
| **UA-075** | Type-filter toggle group lacks `role="group"` + `aria-pressed` on buttons | 2 major (a11y) |

**Evidence:**
- UA-074: `frontend/web/src/views/Themes.vue — found via grep "themes.noThemes"` — `<p class="text-white/40">{{ $t('themes.noThemes') }}</p>`
- UA-075: `frontend/web/src/views/Themes.vue — found via grep "typeFilter"` — type-filter buttons are siblings with `:class="typeFilter === opt.value"` styling but no ARIA state. Same root cause as UA-062 / UA-063 on Anime.

**Holds:**
- UA-034 (ThemeCard contrast — was 82 nodes pre-fix): no `text-white/30` instances on ThemeCard markup. ✓ Fixed.
- UA-035 (Sort + Season selects unlabeled): selects have `aria-label`. ✓ Holds.
- UA-041 (h1 present): ✓ `<h1>{{ $t('themes.title') }}</h1>` present.

## Schedule.vue (`frontend/web/src/views/Schedule.vue` — 192 lines)

| ID | Title | Severity |
|---|---|---|
| **UA-076** | Schedule hint text uses `text-white/40` (contrast ~3.16:1) | 1 minor (a11y) |

**Evidence:**
- UA-076: `frontend/web/src/views/Schedule.vue — found via grep "schedule.hint"` — `<p class="text-white/40 text-sm mt-2">{{ $t('schedule.hint') }}</p>`

**Holds:** h1, navigation, i18n all clean. Cleanest of the four views audited in this pass.

## Game.vue (`frontend/web/src/views/Game.vue` — 442 lines)

| ID | Title | Severity |
|---|---|---|
| **UA-077** | Leaderboard rank cells use `text-white/40` for ranks ≥ 2 (~3.16:1) | 1 minor (a11y, mostly cosmetic on leaderboard) |
| **UA-078** | Answer-option buttons form a radio-like group but lack `role="radiogroup"` / `aria-pressed` | 2 major (a11y — interactive quiz answers) |
| **UA-079** | Room cards (`<button>`) use card layout; depend on inner text for accessible name (acceptable but verify focus ring) | 1 minor (a11y) |

**Evidence:**
- UA-077: `Game.vue — found via grep "text-white/40"` (line ~117) — leaderboard rank styling.
- UA-078: `Game.vue — found via grep "selectedAnswer === index"` — bare `<button>` siblings; the visual "selected" state needs `aria-pressed="true"` per ARIA Toggle pattern. Same root cause as UA-062 / UA-063 / UA-075.

**Holds:** h1 present, i18n clean.

## Navbar.vue (`frontend/web/src/components/layout/Navbar.vue` — 423 lines)

| ID | Title | Severity |
|---|---|---|
| **UA-080** | Mobile-menu close-button still hardcoded English: `:aria-label="mobileMenuOpen ? 'Close menu' : 'Open menu'"` (line ~172) | 2 major (a11y / i18n) — **same as UA-043 still open** |
| **UA-081** | Navbar search-close icon button has no `aria-label` / `title` | 2 major (a11y) — **same pattern as UA-065 on Profile** |
| **UA-082** | Mobile language-toggle buttons (RU/EN/JA) lack `role="group"` + `aria-pressed` | 2 major (a11y) — **same root cause** |
| **UA-083** | Hamburger button has no `aria-expanded` state on the drawer (UA-053 still open) | 2 major (a11y) |
| **UA-084** | Mobile drawer is a plain `<div>` — no `role="dialog"` / `aria-modal="true"` (UA-054 still open) | 2 major (a11y) |
| **UA-085** | Mobile drawer doesn't include a Schedule link (UA-055 still open) | 1 minor (discovery) |
| **UA-086** | Search subtitle / icon use `text-white/40` in 3 places — contrast fail | 1 minor (a11y) |

**Evidence:**
- All anchors per the subagent's code audit; verbatim grep targets: `'Close menu'`, `'Open menu'`, `text-white/40`, `mobile menu`, `role="dialog"`, `aria-expanded`, `aria-modal`.

**Note on the avatar fix (commit `84fb943`):** the avatar `<img>` is now rendered in header (good); no a11y regression introduced. The new `<img>` should still verify it has `alt` (likely empty alt is acceptable since it's inside a labeled button — verify in browser pass).

## Mobile findings (500×723) — code-based supplement

Browser sweep is blocked, but the following mobile-specific findings are confirmed open via source inspection (see carry-over verification table):

- **UA-042** Home schedule shortcut icon-only on sm: + no aria-label — STILL OPEN.
- **UA-043** Hamburger English aria-label — STILL OPEN (now also numbered UA-080).
- **UA-044** Input.vue lacks `v-bind="$attrs"` — STILL OPEN (no aria-* pass-through).
- **UA-045** Hamburger touch target size — needs viewport measurement; defer to next browser session.
- **UA-046** Genre placeholder `text-white/30` — STILL OPEN.
- **UA-047** GenreFilterPopup `aria-haspopup` — STILL OPEN.
- **UA-048** Browse heading-order — STILL OPEN.
- **UA-050** "Failed to fetch anime" English literal — STILL OPEN (in `composables/useAnime.ts`).
- **UA-051** Anime detail dynamic title — STILL OPEN.
- **UA-052** Anime detail `text-white/40` × 5 — STILL OPEN.
- **UA-053**…**UA-055** Drawer a11y — STILL OPEN.
- **UA-056** Drawer z-index — ✓ Fixed (only one carry-over actually fixed).
