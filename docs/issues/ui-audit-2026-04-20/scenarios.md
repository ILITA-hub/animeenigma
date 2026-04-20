# Scenario probes — mobile — 2026-04-20

## Scenario N3 — hamburger nav switching

Flow: Home → tap hamburger → verify each destination.

### Pass
- Hamburger opens/closes (click toggle works)
- Nav links resolve to: `/` (logo), `/` (Главная), `/browse`, `/themes`, `/game`, `/profile`
- Language toggle (Русский / 日本語 / English) rendered in the drawer panel
- Close X button swaps the hamburger's aria-label from "Open menu" → "Close menu"

### Fail

### [UA-053] Hamburger button has no `aria-expanded` — Severity 2 (major) — accessibility

**Evidence:**
- `<button aria-label="Open menu">` has `aria-expanded=null` before and after the click
- Screen-reader users can't tell whether the menu is open, and can't use the standard expanded/collapsed announcement

**Citations:**
- `frontend/web/src/components/layout/Navbar.vue — found via grep "mobileMenuOpen"`

**Proposed fix:** Add `:aria-expanded="mobileMenuOpen"` and `aria-controls="mobile-nav-menu"` to the hamburger button. One-line template change.

### [UA-054] Mobile menu panel has no `role="dialog"` + `aria-modal` — Severity 1 (minor) — accessibility

**Evidence:**
- Panel is plain `<div v-if="mobileMenuOpen" class="md:hidden py-4 border-t border-white/10 glass-nav rounded-b-2xl">` — no dialog role, no aria-label, no aria-modal
- SR users can't understand this is a modal overlay; focus isn't trapped

**Citations:**
- `frontend/web/src/components/layout/Navbar.vue — found via grep "mobileMenuOpen"`

**Proposed fix:** Add `role="dialog" aria-modal="true" :aria-label="$t('nav.mobileMenu')" id="mobile-nav-menu"` on the panel, plus tabindex=-1 + focus-trap on open.

### [UA-055] Schedule destination missing from mobile menu — Severity 2 (major) — UX

**Evidence:**
- `header a` pathname enumeration in open mobile menu: `/, /, /browse, /themes, /game, /profile` — **no `/schedule`**
- Schedule link exists on Home as the cyan icon shortcut (but that has its own UA-042 a11y bug on mobile)
- On any other view (Browse, Profile, Themes, Anime), mobile users have **no navigation path to Schedule at all**

**Why it matters:** Schedule is a first-class destination (ongoing episode schedule) — it's broken on every mobile view except Home, and even Home only exposes it via an unlabeled icon.

**Citations:**
- `frontend/web/src/components/layout/Navbar.vue — found via grep "/browse\|/themes\|/game"` (the current mobile menu link block)

**Proposed fix:** Add a `<router-link to="/schedule">{{ $t('nav.schedule') }}</router-link>` item to the mobile menu between OP/ED and Комнаты. Single-line addition.

### [UA-056] Mobile menu panel renders BELOW Home search wrapper (z-index stack) — Severity 2 (major) — layout regression

**Evidence:**
- Screenshot: open the mobile menu from `/`. The menu panel renders with "Поиск аниме…" placeholder visible bleeding through the "Главная"/"Каталог" menu text, and the Schedule icon button stays fully painted on top of the panel
- Stack audit: `header` = `fixed z-50`, mobile menu panel = `z-index: auto, position: static` (inherits from header z-50 parent, but enters static flow for painting). Home's search wrapper = `relative z-[60]` — its stacking context **beats** the header's z-50 because z-60 > z-50
- Root cause = the recent Firefox-fix commit `581a5b5` moved `relative z-[60]` to the inner row, but it still competes with the mobile drawer. Also affects pre-fix state — this is a latent bug, not a net-new regression

**Why it matters:** Looks broken. Visual overlap renders the menu hard to read and makes Schedule shortcut tappable through the open menu.

**Citations:**
- `frontend/web/src/components/layout/Navbar.vue — found via grep "mobileMenuOpen"`
- `frontend/web/src/views/Home.vue — found via grep "relative z-\[60\]"`

**Proposed fix:** Promote the mobile menu panel to `position: fixed top-16 inset-x-0 z-[70]` (or at minimum `z-[70]` on the inline panel and an explicit position). Belt-and-braces: when `mobileMenuOpen`, temporarily neuter Home's z-[60] (conditional class) — or, cheaper: just make the panel win with z-[70].

## Scenario L1 — add to watchlist on mobile

Flow: anime detail (logged in) → tap status "Смотрю" → pick status → verify aria-checked.

### Pass
- Status button opens menu on click (`aria-expanded` toggles false→true)
- 5 `[role="menuitemradio"]` + 1 `[role="menuitem"]` ("Удалить из списка")
- Current status "Смотрю" carries `aria-checked="true"`, others `aria-checked="false"`
- No hover dead-ends; touch works first-try

### Fail
- None. L1 flow is cleanly implemented (UA-015 shipped in Batch A is verified live on mobile)

## Scenario W1 — anonymous player load on mobile

Flow: logout → `/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623` (Frieren) → tap "Смотреть".

### Pass
- Anime page loads for anon users (h1 = full title)
- Status button correctly absent (logged-out state)
- Player does NOT auto-load on mount (UA-014 shipped holds up)
- Tapping "Смотреть" CTA spawns Kodik iframe (`https://kodikplayer.com/seria/...`) after ~1.5s — lazy mount works on mobile touch
- No JS errors, no auth wall, no crash

### Fail
- None. W1 is green end-to-end on mobile.
