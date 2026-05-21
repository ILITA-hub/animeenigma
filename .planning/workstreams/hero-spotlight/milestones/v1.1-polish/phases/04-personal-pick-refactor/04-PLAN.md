---
phase: 04-personal-pick-refactor
plan: 04
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-PP-01, HSB-V11-PP-02, HSB-V11-PP-03, HSB-V11-PP-04]
blocked_by: [01]
status: ready
---

# Plan 04: PersonalPickCard refactor — featured-plus-secondary layout

## Goal

Replace the 3-equal-posters grid with a featured-pick (60% width) + 2
secondary picks (40% width, stacked) layout. Fix the truncated-title bug,
add per-item reason copy, surface the username in the personalized title,
and make the mobile "+ N more" link a proper full-width footer button.

## Tasks

### Task 1 — Featured + secondary layout (desktop)

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop
    variant="poster-blur"
    :poster-url="featured.anime.poster_url"
  />
  <div class="relative z-10 w-full h-full grid md:grid-cols-[3fr_2fr] gap-4 md:gap-6 p-4 md:p-6 lg:p-8">
    <!-- Featured pick -->
    <router-link
      :to="`/anime/${featured.anime.id}`"
      class="flex flex-col md:flex-row gap-4 group min-h-0"
    >
      <!-- poster 280x400 -->
      <!-- title + reason chip + cta-hero -->
    </router-link>
    <!-- Secondary picks (md+ only) -->
    <ul class="hidden md:flex flex-col gap-2 min-h-0">
      <li v-for="item in secondary" :key="item.anime.id">
        <!-- small 96x144 card with title + reason -->
      </li>
    </ul>
  </div>
  <!-- Mobile footer "+ N more" -->
  <router-link
    v-if="data.items.length > 1"
    :to="moreLinkTo"
    class="md:hidden block w-full text-center py-3 cta-card mt-3"
  >
    {{ t('spotlight.personalPick.moreLink', { n: data.items.length - 1 }) }}
  </router-link>
</article>
```

### Task 2 — Personalized title with username

The backend's `personal_pick` resolver passes `data.source === 'personal'`
for logged-in users. Username flows through JWT context. The frontend can
read it from the auth store:

```ts
import { useAuthStore } from '@/stores/auth'
const auth = useAuthStore()

const title = computed(() => {
  if (data.source === 'personal' && auth.user?.username) {
    return t('spotlight.personalPick.titleWithName', { name: auth.user.username })
  }
  if (data.source === 'personal') return t('spotlight.personalPick.title')
  return t('spotlight.personalPick.titleAnon')
})
```

Add `personalPick.titleWithName` i18n key:
- EN: `"For you, {name}"`
- RU: `"Для вас, {name}"`
- JA: `"{name}さんへのおすすめ"`

### Task 3 — Per-item reason chip

Each item already carries `reason_i18n_key`. Render as a chip:

```vue
<span
  v-if="item.reason_i18n_key"
  class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium bg-cyan-500/20 text-cyan-200"
>
  <SpotlightIcon name="sparkles" class="w-3 h-3" />
  {{ t(item.reason_i18n_key) }}
</span>
```

### Task 4 — Fix title truncation

Current bug: `flex-1` on the grid eats all height, leaving no room for the
poster + title underneath. New layout uses `grid-cols-[3fr_2fr]` and
explicitly sizes the featured poster (`flex-shrink-0`) so the title block
flows naturally below/beside.

### Task 5 — Spec updates

`PersonalPickCard.spec.ts`:

- Featured-pick container has `aria-label` referencing the featured anime's title.
- Secondary picks count = `data.items.length - 1` (max 2).
- Username appears in title when `data.source === 'personal'` and store has a user.
- Mobile (<768px via `useMediaQuery` mock): "+ N more" button is `block w-full`.
- Reason chips render `reason_i18n_key` for items that have one.

## Verification

- `bunx vitest run src/components/home/spotlight/cards/PersonalPickCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: load logged in as `ui_audit_bot`, confirm "Для вас, ui_audit_bot" title appears.

## Metrics

`UXΔ = +4 (Better) · CDI = 0.05 * 13 · MVQ = Kraken 88%/85%`
