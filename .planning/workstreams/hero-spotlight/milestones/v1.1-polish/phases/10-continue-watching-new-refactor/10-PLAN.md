---
phase: 10-continue-watching-new-refactor
plan: 10
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-CWN-01, HSB-V11-CWN-02, HSB-V11-CWN-03]
blocked_by: [01]
status: ready
---

# Plan 10: ContinueWatchingNewCard refactor — hero ribbon + deep-link CTA

## Goal

Transform the tiny "New episode N!" corner badge into a hero ribbon
across the top of the poster, stack the episode meta (last watched +
new) with visual hierarchy, and deep-link the CTA into the new episode
directly (not the detail page).

## Tasks

### Task 1 — Backdrop + purple overlay

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop variant="poster-blur" :poster-url="data.anime.poster_url" />
  <div aria-hidden="true" class="absolute inset-0 bg-gradient-to-r from-purple-500/30 via-transparent to-transparent" />
  <div class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-6 lg:p-8 md:items-center">
    <!-- Poster + hero ribbon -->
    <!-- Meta column -->
  </div>
</article>
```

### Task 2 — Hero ribbon

```vue
<router-link
  :to="watchUrl"
  class="relative flex-shrink-0 self-center md:self-start w-32 md:w-40 lg:w-52 group"
>
  <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-purple-500/30">
    <img
      :src="data.anime.poster_url || '/placeholder.svg'"
      :alt="title"
      class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
      loading="lazy"
    />
    <!-- Hero ribbon ACROSS the top -->
    <div
      class="absolute inset-x-0 top-0 px-3 py-1.5 bg-gradient-to-r from-purple-600 to-fuchsia-500 text-white text-xs font-bold uppercase tracking-wider shadow-lg flex items-center justify-center gap-1.5"
    >
      <SpotlightIcon name="play" class="w-3.5 h-3.5" />
      {{ t('spotlight.continueWatchingNew.newEpisodeBadge', { n: data.new_episode_number }) }}
    </div>
  </div>
</router-link>
```

### Task 3 — Two-row episode meta with hierarchy

```vue
<div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
  <div>
    <div class="flex items-center gap-2 mb-3">
      <SpotlightIcon name="play" class="w-5 h-5 text-purple-300" />
      <p class="text-purple-200 text-sm font-semibold uppercase tracking-[0.15em]">
        {{ t('spotlight.continueWatchingNew.title') }}
      </p>
    </div>

    <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2 mb-3">
      {{ title }}
    </h3>

    <!-- Subdued: where you stopped -->
    <p class="text-xs text-gray-400 font-medium">
      {{ t('spotlight.continueWatchingNew.lastWatched', { n: data.last_watched_episode }) }}
    </p>
    <!-- Accent: what's new -->
    <p class="mt-1 text-lg text-purple-200 font-semibold tabular-nums">
      {{ t('spotlight.continueWatchingNew.newEpisodeLine', { n: data.new_episode_number }) }}
    </p>
  </div>

  <router-link :to="watchUrl" class="cta-hero" data-accent="purple">
    {{ t('spotlight.continueWatchingNew.resumeCtaWithEp', { n: data.new_episode_number }) }}
    <SpotlightIcon name="play" class="w-4 h-4" />
  </router-link>
</div>
```

i18n keys added (EN/RU/JA):

- `lastWatched`: "Вы посмотрели до серии {n}" / "Watched up to ep {n}" / "{n}話まで視聴済"
- `newEpisodeLine`: "Новая серия {n}" / "New episode {n}" / "新しい{n}話"
- `resumeCtaWithEp`: "Смотреть серию {n} →" / "Watch episode {n} →" / "{n}話を見る →"

### Task 4 — Deep-link CTA

```ts
const watchUrl = computed(
  () => `/anime/${data.anime.id}/watch?episode=${data.new_episode_number}`,
)
```

> **Verify upstream consumer.** The Watch view (`frontend/web/src/views/Watch.vue`)
> must honor the `?episode=N` query param at mount. Grep `services/.../watch`
> + `Watch.vue` to confirm. If absent, add a one-line `route.query.episode`
> handler — this is a pre-flight check, not a separate task.

### Task 5 — Spec updates

`ContinueWatchingNewCard.spec.ts`:

- Hero ribbon `<div>` rendered with `inset-x-0 top-0` (spans the full top).
- Ribbon text contains `data.new_episode_number` via interpolation.
- "Last watched" line + "New episode" line BOTH render (count = 2).
- CTA href ends in `/watch?episode={n}` where n === `data.new_episode_number`.
- Backdrop secondary overlay class contains `from-purple-500/30`.

## Verification

- `bunx vitest run src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: log in as a user with a watch_progress row where
  `episodes_aired > last_watched_episode + 1` (seed if needed), cycle to
  ContinueWatchingNew, click CTA, confirm player opens episode N.

## Metrics

`UXΔ = +4 (Better) · CDI = 0.04 * 8 · MVQ = Phoenix 86%/82%`
