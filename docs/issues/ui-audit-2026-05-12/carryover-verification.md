# UA-042…UA-056 Carryover Verification (2026-05-12)

| ID | Status | Evidence (file + grep anchor) | Notes |
|---|---|---|---|
| UA-042 | ✗ Open | `/data/animeenigma/frontend/web/src/views/Home.vue:21` `<span class="hidden sm:inline">{{ $t('nav.schedule') }}</span>` | Schedule icon link at sm: breakpoint is icon-only on mobile. No aria-label on parent router-link. |
| UA-043 | ✗ Open | `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue:172` `:aria-label="mobileMenuOpen ? 'Close menu' : 'Open menu'"` | Literal English strings, not using i18n `$t()`. Should be translated key. |
| UA-044 | ✗ Open | `/data/animeenigma/frontend/web/src/components/ui/Input.vue:10-20` No `v-bind="$attrs"` on `<input>` element | Input component doesn't forward parent attributes to inner input. Missing aria-* pass-through. |
| UA-045 | ? Uncertain | `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue:169-172` Button at px-2 py-2 | Could be 40×40 but visual size unclear from source; need viewport measurement. |
| UA-046 | ✗ Open | `/data/animeenigma/frontend/web/src/components/ui/GenreFilterPopup.vue:11` `'text-white/30'` | Placeholder text uses text-white/30 (3.16:1 contrast). Should be text-white/60 for 4.5:1. |
| UA-047 | ✗ Open | `/data/animeenigma/frontend/web/src/components/ui/GenreFilterPopup.vue:4-8` No `aria-haspopup` or `aria-expanded` | Trigger button missing popup accessibility attributes. |
| UA-048 | ✗ Open | `/data/animeenigma/frontend/web/src/views/Browse.vue:6,79,147` No sr-only `<h2>` between `<h1>` and grid `<h3>` | h1 at line 6, h2 "Recent searches" at line 79 is visible but not always shown (only if !searchQuery). Card grid (h3 inferred) jumps heading. |
| UA-050 | ✗ Open | `/data/animeenigma/frontend/web/src/composables/useAnime.ts` `'Failed to fetch anime'` | Literal English error string in catch block. Should be localized i18n key. |
| UA-051 | ✗ Open | `/data/animeenigma/frontend/web/src/views/Anime.vue` No `document.title` or `useHead()` call found | Anime detail page does not update dynamic `<title>` with anime name. |
| UA-052 | ✗ Open | `/data/animeenigma/frontend/web/src/views/Anime.vue:88,99,597,694,727` 5 instances of `text-white/40` | Anime detail page has residual text-white/40 nodes (contrast ~3.16:1). Should sweep to text-white/60. |
| UA-053 | ✗ Open | `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue:172` No `aria-expanded` on hamburger button | Mobile menu button missing aria-expanded state attribute. |
| UA-054 | ✗ Open | `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue:185` No `role="dialog"` or `aria-modal` on drawer | Mobile menu panel (.md:hidden div) missing dialog semantics. |
| UA-055 | ✗ Open | `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue:187-232` No "/schedule" route link in mobile menu | Mobile drawer links loop over navLinks. No explicit Schedule link present in drawer content. |
| UA-056 | ✓ Fixed | `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue:4` `z-50` on header | Header is z-50. Mobile menu at z-50 renders in stacking order. Home search wrapper at z-[60] still wins. |

