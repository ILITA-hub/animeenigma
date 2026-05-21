---
phase: 02-anime-of-day-refactor
plan: 02
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-AOD-01, HSB-V11-AOD-02, HSB-V11-AOD-03, HSB-V11-AOD-04]
blocked_by: [01]
status: ready
---

# Plan 02: AnimeOfDayCard refactor — cinematic backdrop + single hero CTA

## Goal

Give AnimeOfDayCard a cinematic feel: blurred poster backdrop, larger
foreground poster, single oversized CTA (drop the dead disabled "Add to
list" button), and color-coded genre tags.

## Tasks

### Task 1 — Add backdrop slot to AnimeOfDayCard

Replace the current bare `<article>` with a `relative` container that
hosts `<SpotlightBackdrop>` then the existing content layered above.

```vue
<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop
      variant="poster-blur"
      :poster-url="data.anime.poster_url"
    />
    <div class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-6 lg:p-8 md:items-center">
      <!-- existing poster + meta blocks below, unchanged in structure -->
    </div>
  </article>
</template>
```

### Task 2 — Bigger poster, drop disabled CTA, accent kicker

- Foreground poster `flex-shrink-0` widens to `w-32 md:w-44 lg:w-56`
  (currently `w-28 md:w-32 lg:w-44`).
- Replace the `<button disabled>` with nothing — just one cyan `cta-hero`:

```vue
<router-link
  :to="`/anime/${data.anime.id}/watch`"
  class="cta-hero"
>
  {{ t('spotlight.animeOfDay.watchCta') }}
  <SpotlightIcon name="play" class="w-4 h-4" />
</router-link>
```

- Kicker styling: `text-cyan-300 text-[10px] uppercase tracking-[0.18em] font-semibold`.
- Score badge: drop from poster overlay (un-obstructs the art); render as a
  meta-row pill below the title alongside episodes-count.

### Task 3 — Genre tag color coding

Add `genreColorClass` helper that maps `genre.id` → Tailwind bg class. Map
lives in `tokens.ts` under `cardTokens.anime_of_day.genreColors` (id-keyed
fallback to `bg-white/10` for unmapped).

```ts
genreColors: {
  '1':  'bg-red-500/20 text-red-200',     // Action
  '4':  'bg-yellow-500/20 text-yellow-200', // Comedy
  '8':  'bg-pink-500/20 text-pink-200',   // Drama
  '10': 'bg-purple-500/20 text-purple-200', // Fantasy
  // ... fill remaining 8-10 common genres; default to bg-white/10
} satisfies Record<string, string>
```

### Task 4 — Spec updates

`AnimeOfDayCard.spec.ts`:

```ts
it('renders SpotlightBackdrop with poster-blur variant', () => {
  const wrapper = mount(AnimeOfDayCard, { props: { data: fixture } })
  const backdrop = wrapper.findComponent({ name: 'SpotlightBackdrop' })
  expect(backdrop.exists()).toBe(true)
  expect(backdrop.props('variant')).toBe('poster-blur')
  expect(backdrop.props('posterUrl')).toBe(fixture.anime.poster_url)
})

it('does not render any disabled CTA', () => {
  const wrapper = mount(AnimeOfDayCard, { props: { data: fixture } })
  expect(wrapper.find('[aria-disabled="true"]').exists()).toBe(false)
  expect(wrapper.find('button[disabled]').exists()).toBe(false)
})

it('renders exactly one cta-hero CTA', () => {
  const wrapper = mount(AnimeOfDayCard, { props: { data: fixture } })
  expect(wrapper.findAll('.cta-hero')).toHaveLength(1)
})

it('applies genre color classes from cardTokens map', () => {
  const wrapper = mount(AnimeOfDayCard, { props: { data: { ...fixture, anime: { ...fixture.anime, genres: [{ id: '1', name: 'Action' }] } } } })
  expect(wrapper.html()).toContain('bg-red-500/20')
})
```

## Verification

- `cd frontend/web && bunx vitest run src/components/home/spotlight/cards/AnimeOfDayCard.spec.ts` — green.
- `cd frontend/web && bunx tsc --noEmit` — clean.
- `cd frontend/web && bunx playwright test spotlight-full` — green.
- Visual smoke (post-deploy): load `https://animeenigma.ru/`, cycle to
  AnimeOfDay card, confirm blurred backdrop + bigger poster + single CTA.

## Metrics

`UXΔ = +3 (Better) · CDI = 0.03 * 8 · MVQ = Phoenix 82%/80%`
