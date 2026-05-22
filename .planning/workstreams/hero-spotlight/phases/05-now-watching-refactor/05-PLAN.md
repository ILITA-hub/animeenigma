---
phase: 05-now-watching-refactor
plan: 05
workstream: hero-spotlight
milestone: v1.1-polish
requirements: [HSB-V11-NW-01, HSB-V11-NW-02, HSB-V11-NW-03, HSB-V11-NW-04]
blocked_by: [01]
status: ready
---

# Plan 05: NowWatchingCard refactor — social live identity

## Goal

Make NowWatchingCard feel alive: bigger poster thumbs, hashed avatar
circles per user, animated cyan→green gradient backdrop, and a pulsing
LIVE micro-element next to each avatar (not text on the right).

## Tasks

### Task 1 — Animated mesh backdrop

```vue
<article class="relative w-full h-full overflow-hidden">
  <SpotlightBackdrop variant="gradient-mesh" accent="green" />
  <div class="relative z-10 w-full h-full flex flex-col gap-3 p-4 md:p-6 lg:p-8">
    <header>
      <div class="flex items-center gap-2">
        <SpotlightIcon name="pulse" class="w-4 h-4 text-green-300 animate-pulse" />
        <h3 class="text-lg md:text-xl font-semibold text-white">
          {{ t('spotlight.nowWatching.title') }}
        </h3>
      </div>
    </header>
    <ul class="flex-1 flex flex-col gap-2 min-h-0">
      <!-- rows -->
    </ul>
  </div>
</article>
```

Mesh backdrop variant in `SpotlightBackdrop.vue` for `accent="green"`:

```css
.mesh-green {
  background:
    radial-gradient(at 20% 20%, rgba(34, 197, 94, 0.25) 0%, transparent 50%),
    radial-gradient(at 80% 60%, rgba(6, 182, 212, 0.25) 0%, transparent 50%),
    radial-gradient(at 50% 90%, rgba(168, 85, 247, 0.15) 0%, transparent 50%);
  animation: mesh-drift 20s ease-in-out infinite;
}
@keyframes mesh-drift {
  0%, 100% { background-position: 0% 0%, 100% 100%, 50% 50%; }
  50%      { background-position: 10% 5%, 95% 95%, 45% 55%; }
}
```

### Task 2 — Bigger poster + avatar circle per row

```vue
<router-link
  :to="`/anime/${s.anime_id}`"
  class="flex items-center gap-3 p-3 rounded-xl bg-white/5 hover:bg-white/10 backdrop-blur-sm transition group min-w-0"
>
  <!-- Avatar circle (hashed color) -->
  <div
    class="relative flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center text-sm font-semibold text-white"
    :class="avatarBgClass(s.username)"
  >
    {{ s.username.slice(0, 1).toUpperCase() }}
    <!-- Pulsing LIVE indicator -->
    <span
      aria-hidden="true"
      class="absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full bg-green-400 ring-2 ring-[#0a0e1a] animate-pulse"
    />
    <span class="sr-only">{{ t('spotlight.nowWatching.liveBadge') }}</span>
  </div>

  <!-- Bigger anime poster (56x84) -->
  <img
    v-if="s.poster_url"
    :src="s.poster_url"
    alt=""
    class="w-14 h-21 object-cover rounded-md flex-shrink-0"
    loading="lazy"
  />

  <!-- Text -->
  <div class="flex-1 min-w-0">
    <p class="text-sm font-semibold text-white truncate">{{ s.username }}</p>
    <p class="text-xs text-gray-300 truncate">
      {{ getLocalizedTitle(s.anime_name, s.anime_name_ru) }} · ep {{ s.episode_number }}
    </p>
  </div>
</router-link>
```

### Task 3 — Deterministic avatar color

```ts
const PALETTE = [
  'bg-red-500', 'bg-orange-500', 'bg-amber-500', 'bg-emerald-500',
  'bg-cyan-500', 'bg-sky-500', 'bg-violet-500', 'bg-pink-500'
] as const

function avatarBgClass(username: string): string {
  let hash = 0
  for (const ch of username) hash = (hash * 31 + ch.charCodeAt(0)) | 0
  return PALETTE[Math.abs(hash) % PALETTE.length]
}
```

### Task 4 — Spec updates

`NowWatchingCard.spec.ts`:

- Each row has avatar circle with text content matching `username[0].toUpperCase()`.
- Avatar class is deterministic — same username → same class across mounts.
- Poster img has `w-14 h-21` (56×84).
- Pulsing LIVE indicator dot rendered next to avatar (not text "LIVE" on right).
- `SpotlightBackdrop` rendered with `variant="gradient-mesh"` `accent="green"`.

## Verification

- `bunx vitest run src/components/home/spotlight/cards/NowWatchingCard.spec.ts` — green.
- `bunx tsc --noEmit` — clean.
- `bunx playwright test spotlight-full` — green.
- Visual smoke: requires another concurrent user watching (data eligibility);
  if no rows visible, manually triggered via seeding `watch_progress` rows
  in the last 5 minutes.

## Metrics

`UXΔ = +3 (Better) · CDI = 0.04 * 8 · MVQ = Phoenix 80%/75%`
