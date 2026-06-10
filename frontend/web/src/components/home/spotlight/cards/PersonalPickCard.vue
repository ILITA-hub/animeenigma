<template>
  <SpotlightCardShell
    accent="cyan"
    icon="sparkles"
    :kicker="featured ? title : ''"
    backdrop="poster-blur"
    :poster-url="featured?.anime.poster_url ?? ''"
  >
    <template v-if="featured">
      <div class="flex-1 grid md:grid-cols-[3fr_2fr] gap-4 md:gap-6 min-h-0">
        <!-- ── Featured pick ───────────────────────────────────────────── -->
        <router-link
          :to="`/anime/${featured.anime.id}`"
          :aria-label="featuredAriaLabel"
          class="flex flex-col md:flex-row gap-4 group min-h-0"
        >
          <div class="flex-shrink-0 w-32 md:w-40 lg:w-48 self-center md:self-start">
            <div
              class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-cyan-500/20 group-hover:shadow-cyan-500/40 transition-shadow"
            >
              <img
                :src="cardPosterUrl(featured.anime.poster_url, 256)"
                :alt="featuredTitle"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                loading="lazy"
                decoding="async"
              />
            </div>
          </div>

          <div class="flex-1 flex flex-col gap-2 min-w-0">
            <h3
              class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2"
            >
              {{ featuredTitle }}
            </h3>
            <Badge
              v-if="featured.reason_i18n_key"
              variant="primary"
              size="sm"
              overlay
              class="self-start"
            >
              <template #icon>
                <SpotlightIcon name="sparkles" class="w-3 h-3" />
              </template>
              {{ t(featured.reason_i18n_key) }}
            </Badge>
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
                  :src="cardPosterUrl(item.anime.poster_url, 128)"
                  :alt="
                    getLocalizedTitle(
                      item.anime.name,
                      item.anime.name_ru,
                      item.anime.name_jp,
                    )
                  "
                  class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                  loading="lazy"
                  decoding="async"
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
                  class="text-xs font-medium text-cyan-400/80 truncate"
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
        :class="[
          buttonVariants({ variant: 'ghost', size: 'sm' }),
          'absolute bottom-3 inset-x-3 justify-center md:hidden z-20',
        ]"
      >
        {{
          t('spotlight.personalPick.moreLink', {
            n: data.items.length - 1,
          })
        }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
/**
 * Workstream hero-spotlight — v1.1-polish Phase 04 (HSB-V11-PP-01..04);
 * DS alignment 2026-06-10 (SpotlightCardShell + Badge/Button primitives).
 *
 * Two-zone layout:
 *   - Featured pick (60% desktop / full mobile): large poster + title +
 *     reason chip (overlay Badge), backed by the featured anime's blurred
 *     poster.
 *   - Secondary picks (40% desktop, hidden on mobile): up to 2 stacked
 *     rows with small posters + titles + reason lines.
 *
 * Mobile keeps only the featured pick and surfaces a full-width
 * ghost-Button "+ N more →" footer.
 *
 * Title personalization: when `data.source === 'personal'` and the auth
 * store has a `user.username`, render `titleWithName` with the username
 * interpolated; falls back to `title` (logged-in, no name) or
 * `titleAnon` (anonymous / source='trending').
 *
 * CRITICAL — single-element root: SpotlightCardShell's root <article> is
 * the only root node; conditional content lives INSIDE it, never around
 * it (Transition mode="out-in" safety — Phase 04 e2e regression).
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import { useAuthStore } from '@/stores/auth'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import type { PersonalPickData } from '@/types/spotlight'
import { cardPosterUrl } from '@/composables/useImageProxy'

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
