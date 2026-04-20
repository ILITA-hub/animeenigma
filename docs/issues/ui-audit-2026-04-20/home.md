# Home mobile audit — 2026-04-20 — viewport 500x723

## State verification (verifies prior batches landed)

- `<html lang>` = `ru` — UA-005 ✓ shipped via Batch B
- `document.title` = "Главная - AnimeEnigma" — UA-040 ✓ shipped via Batch F
- `<h1>` exists (sr-only, text "AnimeEnigma") — UA-006 ✓ shipped via Batch B
- Home search dropdown background: `oklab(0.13 ... / 0.95)` + `backdrop-blur-xl` — bd45b4a ✓ shipped
- Search input wraps in `<div role="combobox" aria-expanded aria-autocomplete aria-controls>` — Batch C wired
- Navbar z=50, transform=none on Home at scrollY=0 — no Firefox hover regressions observed

## axe-core (mobile 500x723)

- 40 passes, **1 violation**, dramatic improvement over desktop (prior audits had 3 on Home)
- violations:
  - `link-name` serious 1 node — target `.bg-cyan-500/10` (see UA-042)

## NEW findings

### [UA-042] Home mobile — /schedule shortcut link has no accessible name when icon-only — Severity 2 (major) — accessibility

**View:** `/` (mobile widths < Tailwind `sm` = 640px)
**Heuristic:** WCAG 4.1.2 (Name, Role, Value)
**Evidence:**
- axe `link-name` serious, 1 node
- DOM: `<a href="/schedule" class="... bg-cyan-500/10 ...">` with child `<svg>` and `<span class="hidden sm:inline">` wrapping "Расписание"
- On mobile (<640px) the span is `display:none` leaving only the icon with no textual accessible name
- On desktop the span makes the text visible, so this never triggered in prior desktop audits

**Why it matters:** Screen-reader users hear "link" with no label — cannot tell this shortcut goes to Schedule. Keyboard users Tab into it with no cue.

**Citations:**
- `frontend/web/src/views/Home.vue — found via grep "bg-cyan-500/10 border border-cyan-500/20"`

**Proposed fix:** Add `:aria-label="$t('nav.schedule')"` on the `<router-link to="/schedule">`. One-line change; the i18n key already exists (`nav.schedule`).

### [UA-043] Hamburger `aria-label="Open menu"` — English string on Russian locale — Severity 1 (minor) — i18n

**View:** global navbar — visible on every view at mobile widths
**Heuristic:** Consistency + SC 3.1.2 (Language of Parts)
**Evidence:**
- DOM: `<button aria-label="Open menu">` in `<header>`
- `<html lang="ru">`, UI strings all Russian, but SR would announce the English "Open menu"
- String is hard-coded, not piped through `$t(...)`

**Citations:**
- `frontend/web/src/components/layout/Navbar.vue — found via grep "Open menu"`

**Proposed fix:** Replace the literal with `:aria-label="$t('nav.openMenu')"` and add `nav.openMenu` / `nav.closeMenu` entries to `ru.json` / `en.json` / `ja.json`.

### [UA-044] SearchAutocomplete ARIA attrs land on wrapper `<div>` instead of `<input>` — Severity 2 (major) — accessibility

**View:** `/` (and `/browse` — same shared component)
**Heuristic:** WCAG 4.1.2 + ARIA authoring pattern for combobox
**Evidence:**
- SearchAutocomplete.vue passes `role="combobox"`, `aria-autocomplete="list"`, `aria-controls`, `aria-expanded`, `aria-activedescendant` as props to `<Input>` component.
- `Input.vue` does NOT declare these as props, does NOT `v-bind="$attrs"` on the inner `<input>`, and has `inheritAttrs` at Vue's default (true). Extra attrs end up on the root `<div class="w-full">`.
- DOM probe on Home search with dropdown open:
  - `<div role="combobox" aria-expanded="true" aria-controls="home-search" aria-autocomplete="list">` (the wrapper)
  - `<input type="search">` — no role, no aria-expanded, no aria-controls
- Screen readers announce the focused `<input>`, not the outer div. The combobox role is effectively lost.

**Why it matters:** ARIA authoring-pattern specifically requires the combobox role be on the focusable text input (or the textbox be labeled by a combobox element through a pattern like 1.1). Landing it on a sibling div breaks SR announcement of "combobox, expanded" and of activedescendant highlight.

**Citations:**
- `frontend/web/src/components/ui/SearchAutocomplete.vue — found via grep "role=\"combobox\""`
- `frontend/web/src/components/ui/Input.vue — found via grep "<input"`

**Proposed fix:** In `Input.vue`:
1. Add `defineOptions({ inheritAttrs: false })`
2. On the inner `<input>`, add `v-bind="$attrs"` alongside the existing bindings
Two-line change; fixes both Home and Browse at once.

### [UA-045] Hamburger button 40×40 — below WCAG 2.5.5 target size — Severity 1 (minor) — accessibility

**View:** global navbar at mobile
**Heuristic:** WCAG 2.5.5 (Target Size, AAA at 44, AA at 24; and Apple HIG / Material 48 guidance)
**Evidence:**
- `header button[aria-label="Open menu"]` bounding box = 40×40 px
- Thumb taps on ~44×44 reach 95th-percentile accuracy; 40×40 measurably worse

**Citations:**
- `frontend/web/src/components/layout/Navbar.vue — found via grep "Open menu"`

**Proposed fix:** Promote `w-10 h-10` → `w-11 h-11` (or add `p-2` / larger padding). Single class swap.

## Deferred on Home mobile

- "Continue Watching" section not observed on Home mobile for `ui_audit_bot`; may require more varied seed data to test the mobile layout of that row
- Didn't drive keyboard-nav of the dropdown with a soft keyboard — mobile keyboard-nav is inherently limited, and axe covers the attr surface
