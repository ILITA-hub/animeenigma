# Phase 2 Summary: Tier B — Quick-wins batch

**Completed:** 2026-05-13
**Plan:** 02-PLAN.md
**Outcome:** All four sub-batches shipped. ~13 audit findings closed in one PR.

## Changes shipped

### B1 — i18n leaks (UX-03)

- `nav.openMenu`, `nav.closeMenu`, `nav.closeSearch`, `nav.scheduleLink` added to all three locale files (`en.json`, `ru.json`, `ja.json`).
- `errors.fetchAnime`, `errors.fetchAnimeList` added as a new top-level `errors` namespace in all three locales.
- `Navbar.vue:173` — `:aria-label` switched from literal `'Close menu' : 'Open menu'` to `$t('nav.closeMenu') : $t('nav.openMenu')`.
- `useAnime.ts` — added `import { useI18n } from 'vue-i18n'`; `t()` resolved at the top of `useAnime()`; replaced `'Failed to fetch anime'` and `'Failed to fetch anime list'` literals at L116/L133 with `t('errors.fetchAnime')` / `t('errors.fetchAnimeList')`.

### B2 — Dynamic titles (UX-04)

- `Anime.vue` — added a `watch(() => anime.value?.title, ...)` that runs `document.title = '${title} — AnimeEnigma'` whenever the loaded anime's title resolves. `anime.value.title` is already locale-resolved by the transform in `useAnime.ts`, so RU/EN/JA users each see the title in their locale.
- `Profile.vue` — added a `watch(() => profileUser.value?.username, ...)` that runs `document.title = '${username} — AnimeEnigma'` whenever the loaded profile populates.

### B3 — Aria-label batch (UX-05)

- `Home.vue:16` — Schedule icon link gains `:aria-label="$t('nav.scheduleLink')"`. Closes UA-042 (icon-only on mobile, no SR name).
- `Auth.vue` — new `<h1 class="sr-only">{{ $t('auth.heading') }}</h1>` inserted above the existing `<h2>`. SEO + a11y top-level heading. Closes UA-070.
- `Auth.vue:44` — QR canvas gains `role="img"` + `:aria-label="$t('auth.qrAlt')"`. Closes UA-071.
- `Navbar.vue:107` — search-close icon button gains `:aria-label="$t('nav.closeSearch')"`. Closes UA-081.
- `AdminRecs.vue:25` — recompute button gains a non-changing `:aria-label="$t('admin.recs.forceRecompute')"` so SR announcement is consistent across the busy/idle visible-text toggle. Closes UA-099.

### B4 — Tier-A-adjacent quick wins (UX-06)

- `Navbar.vue:268-274` — `navLinks` gains `{ to: '/schedule', label: 'nav.schedule' }` between catalog and themes. The mobile drawer now surfaces /schedule (UA-055).
- `Home.vue` rec card `<img>` switched from `:alt="getLocalizedTitle(...)"` to `alt=""` — decorative image; the visible title text directly below the image already announces the anime name to screen readers. Closes UA-059.
- All three locales updated:
  - `profile.import.malPlaceholder` → mentions "or profile URL" / "или ссылку на профиль" / "またはプロフィールURL".
  - `profile.import.shikimoriPlaceholder` → same pattern.
  - `malDescription` / `shikimoriDescription` reworded to reflect URL acceptance (was: "Enter your username, not a URL"; now: "Username or profile URL both work"). Closes UA-067.

## Verification

See `02-VERIFICATION.md` for the success-criteria scorecard.

## Files touched

```
frontend/web/src/locales/en.json                       # +6 keys, 4 reworded
frontend/web/src/locales/ru.json                       # +6 keys, 4 reworded
frontend/web/src/locales/ja.json                       # +6 keys, 4 reworded
frontend/web/src/composables/useAnime.ts                # +1 import, 1 ref, 2 literal-replaces
frontend/web/src/components/layout/Navbar.vue           # 2 aria-label edits, 1 navLinks entry
frontend/web/src/views/Auth.vue                         # +1 h1.sr-only, 1 canvas a11y
frontend/web/src/views/Home.vue                         # 1 aria-label, 1 alt=""
frontend/web/src/views/Anime.vue                        # +6 lines (title watcher)
frontend/web/src/views/Profile.vue                      # +9 lines (title watcher)
frontend/web/src/views/admin/AdminRecs.vue              # 1 aria-label
.planning/workstreams/ui-ux-audit/phases/02-tier-b-quick-wins/
  02-CONTEXT.md      (new)
  02-PLAN.md         (new)
  02-SUMMARY.md      (this file)
  02-VERIFICATION.md (new)
```

## Notes for downstream phases

- The new `errors.*` top-level namespace is the right home for any future composable-side error strings (composables can't reach `$t()` from templates).
- `nav.scheduleLink` is for icon-link aria; `nav.schedule` remains the visible link text. Keep these distinct downstream.
- The `<h1 class="sr-only">` pattern added to Auth.vue is the reference for any future a11y h1-promotion (e.g., a similar pattern could land in NotFound.vue or other utility views).
