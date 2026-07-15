<template>
  <!-- Phase 11 / UX-22 — sticky Quick-Nav for the Anime detail view.
       Desktop (md+): floating right-side sticky pill column.
       Mobile (<md): sticky horizontal pill row below the hero.
       Theater Mode (UX-23) does NOT hide this: theater keeps the navbar
       and page content visible and only widens the player section. -->

  <!-- Desktop: floating right sticky pill column -->
  <nav
    class="hidden md:flex md:flex-col md:fixed md:top-24 md:right-4 md:z-30 gap-2"
    :aria-label="$t('anime.nav.heading')"
  >
    <a
      v-for="s in sections"
      :key="s.id"
      :href="`#${s.id}`"
      class="px-3 py-1.5 rounded-full text-xs font-medium transition-colors backdrop-blur-sm bg-white/5 border border-white/10"
      :class="active === s.id
        ? 'text-cyan-400 border-cyan-400/40 bg-cyan-500/10'
        : 'text-white/70 hover:text-white'"
      @click="scrollTo(s.id, $event)"
    >
      {{ $t(s.labelKey) }}
    </a>
  </nav>

  <!-- Mobile: sticky horizontal pill row -->
  <nav
    class="md:hidden sticky top-16 z-30 -mx-4 px-4 py-2 bg-background/80 backdrop-blur-md border-b border-white/5 overflow-x-auto scrollbar-hide quicknav-safe"
    :aria-label="$t('anime.nav.heading')"
  >
    <div class="flex gap-2 whitespace-nowrap">
      <a
        v-for="s in sections"
        :key="s.id"
        :href="`#${s.id}`"
        class="px-3 py-1.5 rounded-full text-xs font-medium flex-shrink-0"
        :class="active === s.id ? 'text-cyan-400 bg-cyan-500/10' : 'text-white/70'"
        @click="scrollTo(s.id, $event)"
      >
        {{ $t(s.labelKey) }}
      </a>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'

// The four section IDs are stable contract surface for /anime/:id deep
// links (e.g. /anime/123#section-episodes). Locked in Phase 11 / CONTEXT.md
// D-02. Do not rename without updating Anime.vue section IDs in lockstep.
const sections = [
  { id: 'section-overview', labelKey: 'anime.nav.overview' },
  { id: 'section-episodes', labelKey: 'anime.nav.episodes' },
  { id: 'section-similar',  labelKey: 'anime.nav.similar'  },
  { id: 'section-comments', labelKey: 'anime.nav.comments' },
]

const active = ref<string>('section-overview')
let observer: IntersectionObserver | null = null

function scrollTo(id: string, ev: Event) {
  ev.preventDefault()
  const el = document.getElementById(id)
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'start' })
  // history.replaceState avoids the default jump that <a href="#…"> would
  // trigger; we already did the smooth scroll above.
  history.replaceState(null, '', `#${id}`)
}

onMounted(() => {
  // Observe each section. rootMargin shifts the trigger line down past
  // the sticky header so the active pill flips when the section header
  // crosses the visible area, not when its bottom edge touches the
  // viewport edge.
  observer = new IntersectionObserver(
    (entries) => {
      const visible = entries
        .filter((e) => e.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
      if (visible[0]) {
        active.value = visible[0].target.id
      }
    },
    { rootMargin: '-80px 0px -60% 0px', threshold: 0 },
  )
  for (const s of sections) {
    const el = document.getElementById(s.id)
    if (el) observer.observe(el)
  }
})

onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
</script>

<style scoped>
/* Sticks below the navbar capsule; on notch/Dynamic-Island phones
   (viewport-fit=cover) the capsule sits --safe-top lower, so the sticky
   offset must too. 0px everywhere else — identical to plain top-16. */
.quicknav-safe {
  top: calc(var(--safe-top) + 4rem);
}
</style>
