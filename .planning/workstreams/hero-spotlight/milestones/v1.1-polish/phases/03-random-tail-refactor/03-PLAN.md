---
phase: 03-random-tail-refactor
plan: 03
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-RT-01, HSB-V11-RT-02, HSB-V11-RT-03, HSB-V11-RT-04]
blocked_by: [01]
status: ready
---

# Plan 03: RandomTailCard refactor — purple discovery identity

## Goal

Make RandomTailCard distinct from AnimeOfDayCard (currently a clone) by
giving it a purple "discovery" accent, a shuffle icon, rotating taglines,
and a mount-time shuffle-deck animation gated on reduced-motion.

## Tasks

### Task 1 — Backdrop with purple overlay

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop
    variant="poster-blur"
    :poster-url="data.anime.poster_url"
  />
  <!-- Purple-tinted secondary overlay -->
  <div
    aria-hidden="true"
    class="absolute inset-0 bg-gradient-to-r from-purple-500/30 via-transparent to-transparent"
  />
  <div class="relative z-10 ...">
    <!-- content -->
  </div>
</article>
```

### Task 2 — Shuffle icon + promoted kicker

```vue
<div class="flex items-center gap-2">
  <SpotlightIcon name="shuffle" class="w-4 h-4 text-purple-300" />
  <p class="text-purple-200 text-xs uppercase tracking-[0.2em] font-semibold">
    {{ t('spotlight.randomTail.title') }}
  </p>
</div>
```

### Task 3 — Rotating taglines

Add 4 taglines to i18n (`spotlight.randomTail.taglines[]`) per locale.
Component picks one randomly at mount:

```ts
import { ref, onMounted } from 'vue'

const tagline = ref('')
onMounted(() => {
  const candidates = t('spotlight.randomTail.taglines', null, { returnObjects: true }) as string[]
  tagline.value = candidates[Math.floor(Math.random() * candidates.length)]
})
```

i18n excerpts:

```json
"taglines": [
  "Откройте что-то новое",
  "А вы это смотрели?",
  "Случайный шедевр со склада",
  "Если повезёт — будет любовь"
]
```

(EN + JA add their own variants.)

### Task 4 — Shuffle-deck mount animation

```vue
<div
  v-if="!reducedMotion && showShuffle"
  class="absolute inset-0 z-20 flex items-center justify-center pointer-events-none"
>
  <div class="shuffle-deck">
    <div v-for="n in 5" :key="n" class="shuffle-card" :style="`--delay: ${n * 60}ms`" />
  </div>
</div>
```

CSS keyframes added to `main.css`:

```css
.shuffle-deck { position: relative; width: 144px; height: 200px; }
.shuffle-card {
  position: absolute; inset: 0;
  background: linear-gradient(135deg, #8b5cf6, #06b6d4);
  border-radius: 12px;
  opacity: 0;
  animation: shuffle 800ms cubic-bezier(.4,0,.2,1) var(--delay) forwards;
}
@keyframes shuffle {
  0%   { opacity: 0; transform: translateY(-100px) rotate(-15deg); }
  60%  { opacity: 1; transform: translateY(0) rotate(0); }
  100% { opacity: 0; transform: translateY(20px) rotate(5deg); }
}
```

`showShuffle` flips to false 1000ms post-mount via `setTimeout`.

### Task 5 — Purple CTA + spec updates

CTA uses `data-accent="purple"`:

```vue
<router-link :to="`/anime/${data.anime.id}`" class="cta-hero" data-accent="purple">
  {{ t('spotlight.randomTail.discoverCta') }}
</router-link>
```

`RandomTailCard.spec.ts`:

- Asserts `SpotlightIcon` with `name="shuffle"`.
- Asserts CTA has `data-accent="purple"`.
- Mocks `prefers-reduced-motion: reduce`, mounts, asserts shuffle-deck does NOT render.
- Asserts tagline is one of the 4 candidates.

## Verification

- `bunx vitest run src/components/home/spotlight/cards/RandomTailCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: cycle to RandomTail, confirm purple accent + shuffle icon + animation runs once.

## Metrics

`UXΔ = +2 (Better) · CDI = 0.03 * 5 · MVQ = Sprite 78%/82%`
