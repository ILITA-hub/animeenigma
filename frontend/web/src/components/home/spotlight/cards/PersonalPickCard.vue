<template>
  <article class="relative w-full h-full overflow-hidden">
    <template v-if="featured">
      <SpotlightBackdrop
        variant="poster-blur"
        :poster-url="featured.anime.poster_url"
        accent="cyan"
      />

      <div
        class="relative z-10 w-full h-full grid md:grid-cols-[3fr_2fr] gap-4 md:gap-6 p-4 md:p-6 lg:p-8 min-h-0"
      >
        <!-- ── Featured pick ───────────────────────────────────────────── -->
        <router-link
          :to="`/anime/${featured.anime.id}`"
          :aria-label="featuredAriaLabel"
          class="flex flex-col md:flex-row gap-4 group min-h-0"
        >
          <div class="flex-shrink-0 w-32 md:w-44 lg:w-56 self-center md:self-start">
            <div
              class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-cyan-500/20 group-hover:shadow-cyan-500/40 transition-shadow"
            >
              <img
                :src="featured.anime.poster_url || '/placeholder.svg'"
                :alt="featuredTitle"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                loading="lazy"
              />
            </div>
          </div>

          <div class="flex-1 flex flex-col gap-2 min-w-0">
            <div class="flex items-center gap-2">
              <SpotlightIcon
                name="sparkles"
                class="w-4 h-4 text-cyan-300 flex-shrink-0"
              />
              <p
                class="text-cyan-300 text-[10px] uppercase tracking-[0.18em] font-semibold truncate"
              >
                {{ title }}
              </p>
            </div>
            <h3
              class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2"
            >
              {{ featuredTitle }}
            </h3>
            <span
              v-if="featured.reason_i18n_key"
              class="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium bg-cyan-500/20 text-cyan-200 self-start"
            >
              <SpotlightIcon name="sparkles" class="w-3 h-3" />
              {{ t(featured.reason_i18n_key) }}
            </span>
          </div>
        </router-link>

        <!-- ── Secondary picks (md+ only) ──────────────────────────────── -->
        <ul
          v-if="secondary.length"
          class="hidden md:flex flex-col gap-2 min-h-0 overflow-hidden"
        >
          <li v-for="item in secondary" :key="item.anime.id" class="min-h-0">
            <router-link
              :to="`/anime/${item.anime.id}`"
              class="flex items-center gap-3 p-2 rounded-lg hover:bg-white/10 transition-colors group"
            >
              <div
                class="flex-shrink-0 w-16 h-24 rounded-md overflow-hidden bg-white/5"
              >
                <img
                  :src="item.anime.poster_url || '/placeholder.svg'"
                  :alt="
                    getLocalizedTitle(
                      item.anime.name,
                      item.anime.name_ru,
                      item.anime.name_jp,
                    )
                  "
                  class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                  loading="lazy"
                />
              </div>
              <div class="flex-1 flex flex-col gap-1 min-w-0">
                <h4 class="text-sm font-semibold text-white truncate">
                  {{
                    getLocalizedTitle(
                      item.anime.name,
                      item.anime.name_ru,
                      item.anime.name_jp,
                    )
                  }}
                </h4>
                <p
                  v-if="item.reason_i18n_key"
                  class="text-xs font-medium text-cyan-300/80 truncate"
                >
                  {{ t(item.reason_i18n_key) }}
                </p>
              </div>
            </router-link>
          </li>
        </ul>
      </div>

      <!-- ── Mobile "+ N more →" footer button (mobile-only) ─────────── -->
      <router-link
        v-if="data.items.length > 1"
        :to="moreLinkTo"
        class="absolute bottom-3 inset-x-3 cta-card text-center justify-center md:hidden z-20"
      >
        {{
          t('spotlight.personalPick.moreLink', {
            n: data.items.length - 1,
          })
        }}
      </router-link>
    </template>
  </article>
</template>

<script setup lang="ts">
/**
 * Workstream hero-spotlight — v1.1-polish Phase 04 (HSB-V11-PP-01..04).
 *
 * Refactor: replace the 3-equal-poster grid with a two-zone layout:
 *   - Featured pick (60% desktop / full mobile): large poster + title +
 *     reason chip, backed by the featured anime's blurred poster.
 *   - Secondary picks (40% desktop, hidden on mobile): up to 2 stacked
 *     rows with small posters + titles + reason chips.
 *
 * Mobile keeps only the featured pick and surfaces a full-width
 * cta-card "+ N more →" footer button.
 *
 * Title personalization: when `data.source === 'personal'` and the auth
 * store has a `user.username`, render `titleWithName` with the username
 * interpolated; falls back to `title` (logged-in, no name) or
 * `titleAnon` (anonymous / source='trending').
 *
 * CRITICAL — single-element root: the <template> block intentionally
 * has ONLY one root node (<article>), no top-level v-if, no sibling
 * comment nodes. HeroSpotlightBlock wraps each card in
 * `<transition mode="out-in">`. If the root ever resolves to a comment
 * node — multi-root template OR top-level v-if false — Vue logs
 * "non-element root node that cannot be animated" and the cross-fade
 * silently wedges: the NEXT card's mount never fires, the carousel
 * stays blank after navigation. Conditional content lives INSIDE
 * <article>, never around it. (Phase 04 e2e regression — fixed once.)
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import { useAuthStore } from '@/stores/auth'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import type { PersonalPickData } from '@/types/spotlight'

const props = defineProps<{ data: PersonalPickData }>()

const { t } = useI18n()
const auth = useAuthStore()

// Featured = first item. Secondary = items 1..2 (max 2 more for the
// desktop right column). Mobile shows only the featured pick and a
// "+ N more →" footer that links to the appropriate list view.
const featured = computed(() => props.data.items[0])
const secondary = computed(() => props.data.items.slice(1, 3))

const featuredTitle = computed(() =>
  featured.value
    ? getLocalizedTitle(
        featured.value.anime.name,
        featured.value.anime.name_ru,
        featured.value.anime.name_jp,
      )
    : '',
)

const featuredAriaLabel = computed(() => featuredTitle.value)

// Title precedence:
//   1. Anonymous (source='trending') → titleAnon
//   2. Personal with username      → titleWithName (interpolated)
//   3. Personal without username   → title (generic personalized)
//
// The auth store may be null/unhydrated in SSR/test contexts —
// `auth.user?.username` short-circuits safely.
const title = computed(() => {
  if (props.data.source !== 'personal') {
    return t('spotlight.personalPick.titleAnon')
  }
  const username = auth.user?.username
  if (username) {
    return t('spotlight.personalPick.titleWithName', { name: username })
  }
  return t('spotlight.personalPick.title')
})

const moreLinkTo = computed(() =>
  props.data.source === 'trending' ? '/browse?sort=trending' : '/recs',
)
</script>
