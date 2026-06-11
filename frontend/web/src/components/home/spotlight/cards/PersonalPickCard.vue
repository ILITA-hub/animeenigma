<template>
  <SpotlightCardShell
    accent="cyan"
    icon="sparkles"
    :kicker="featured ? t('spotlight.personalPick.kicker') : ''"
    backdrop="poster-blur"
    :poster-url="featured?.anime.poster_url ?? ''"
  >
  <!--
    Workstream hero-spotlight — v4 C-2 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Refined two-zone
    layout: the personalized title moved from the kicker into the body,
    secondary picks gained rank numbers + ★ scores and live in a
    SCROLLABLE column (desktop, up to 6 items — fade mask + thin cyan
    scrollbar) or a horizontal poster swipe-row (mobile). «Все
    рекомендации» links to /browse?sort=recommended (the old /recs route
    never existed — it 404'd; recs-service support for the sort is a
    recorded TODO).
  -->
    <template v-if="featured">
      <div class="flex-1 min-h-0 grid md:grid-cols-[3fr_2fr] gap-4 md:gap-7">
        <!-- ── Featured pick ───────────────────────────────────────────── -->
        <router-link
          :to="`/anime/${featured.anime.id}`"
          :aria-label="featuredTitle"
          class="flex gap-4 md:gap-5 group min-h-0 min-w-0"
        >
          <SpotlightPoster
            :poster-url="featured.anime.poster_url"
            :alt="featuredTitle"
            width-class="w-24 md:w-36 self-center md:self-start"
            glow="cyan"
            :proxy-width="256"
          />
          <div class="flex-1 flex flex-col gap-2 min-w-0">
            <p class="text-[13px] text-muted-foreground font-medium">{{ title }}</p>
            <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
              {{ featuredTitle }}
            </h3>
            <div class="flex flex-wrap items-center gap-2">
              <Badge v-if="featured.anime.score" variant="warning" size="sm" overlay>
                <template #icon>
                  <Star class="size-3" fill="currentColor" aria-hidden="true" />
                </template>
                {{ featured.anime.score.toFixed(1) }}
              </Badge>
              <Badge
                v-if="featured.reason_i18n_key"
                variant="primary"
                size="sm"
                overlay
                class="min-w-0"
              >
                <template #icon>
                  <SpotlightIcon name="sparkles" class="w-3 h-3" />
                </template>
                {{ t(featured.reason_i18n_key) }}
              </Badge>
            </div>
          </div>
        </router-link>

        <!-- ── Secondary picks: desktop scrollable ranked column ───────── -->
        <ul
          v-if="secondary.length"
          class="rec-scroll hidden md:flex flex-col gap-2.5 min-h-0"
          data-testid="rec-list"
        >
          <SpotlightTile
            v-for="(item, i) in secondary"
            :key="item.anime.id"
            as="li"
            interactive
            class="flex-shrink-0"
          >
            <router-link
              :to="`/anime/${item.anime.id}`"
              class="flex items-center gap-3 p-2 min-w-0"
            >
              <span class="font-mono text-[11px] text-muted-foreground w-3 text-center">{{
                i + 2
              }}</span>
              <SpotlightPoster
                :poster-url="item.anime.poster_url"
                :alt="secondaryTitle(item)"
                width-class="w-10 rounded-md"
                :proxy-width="128"
              />
              <div class="flex-1 flex flex-col gap-0.5 min-w-0">
                <h4 class="text-sm font-semibold text-white truncate">
                  {{ secondaryTitle(item) }}
                </h4>
                <p class="text-xs font-medium text-cyan-400/80 truncate">
                  <template v-if="item.anime.score">★ {{ item.anime.score.toFixed(1) }}</template>
                  <template v-if="item.anime.score && item.reason_i18n_key"> · </template>
                  <template v-if="item.reason_i18n_key">{{ t(item.reason_i18n_key) }}</template>
                </p>
              </div>
            </router-link>
          </SpotlightTile>
        </ul>
      </div>

      <!-- ── Mobile: horizontal poster swipe-row ─────────────────────── -->
      <div v-if="secondary.length" class="md:hidden min-w-0">
        <p class="font-mono text-[10px] uppercase tracking-[0.12em] text-muted-foreground mb-2">
          {{ t('spotlight.personalPick.swipeLabel') }}
        </p>
        <div class="flex gap-2.5 overflow-x-auto pb-1.5 swipe-row" data-testid="rec-swipe">
          <router-link
            v-for="item in secondary"
            :key="`m-${item.anime.id}`"
            :to="`/anime/${item.anime.id}`"
            class="w-[72px] flex-shrink-0"
          >
            <SpotlightPoster
              :poster-url="item.anime.poster_url"
              :alt="secondaryTitle(item)"
              width-class="w-[72px] rounded-lg"
              :proxy-width="128"
            />
            <p class="text-[10px] font-medium text-white truncate mt-1">
              {{ secondaryTitle(item) }}
            </p>
          </router-link>
        </div>
      </div>

    </template>

    <template #cta>
      <router-link
        v-if="featured"
        :to="`/anime/${featured.anime.id}`"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        {{ t('spotlight.personalPick.watchCta') }}
      </router-link>
      <router-link
        :to="allRecsTo"
        :class="[buttonVariants({ variant: 'link', size: 'sm' }), 'text-sm']"
      >
        {{ t('spotlight.personalPick.allRecsCta') }} →
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Star } from 'lucide-vue-next'
import { getLocalizedTitle } from '@/utils/title'
import { useAuthStore } from '@/stores/auth'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import SpotlightTile from '../ui/SpotlightTile.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import type { PersonalPickData, PersonalPickItem } from '@/types/spotlight'

const props = defineProps<{ data: PersonalPickData }>()

const { t } = useI18n()
const auth = useAuthStore()

// Featured = first item (recs arrive ranked). Secondary = the rest, up
// to 5 — the backend caps the payload at 6 total (v4 C-2).
const featured = computed(() => props.data.items[0])
const secondary = computed(() => props.data.items.slice(1, 6))

const featuredTitle = computed(() =>
  featured.value
    ? getLocalizedTitle(
        featured.value.anime.name,
        featured.value.anime.name_ru,
        featured.value.anime.name_jp,
      )
    : '',
)

function secondaryTitle(item: PersonalPickItem): string {
  return getLocalizedTitle(item.anime.name, item.anime.name_ru, item.anime.name_jp)
}

// Personalized line shown above the featured title (was the kicker).
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

// /recs never existed (only /admin/recs/:user_id) — the old mobile
// "+N more" link 404'd. Until a real recs page ships, browse with the
// recommended sort is the landing surface (recs-service TODO recorded
// in the v4 spec).
const allRecsTo = computed(() =>
  props.data.source === 'trending' ? '/browse?sort=trending' : '/browse?sort=recommended',
)
</script>

<style scoped>
/* Desktop secondary column: scrolls when >3 rows; bottom fade signals
   "there's more"; thin cyan scrollbar matches the card accent. */
.rec-scroll {
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: rgba(0, 212, 255, 0.35) transparent;
  padding-right: 4px;
  mask-image: linear-gradient(180deg, black 84%, transparent);
}
.rec-scroll::-webkit-scrollbar {
  width: 5px;
}
.rec-scroll::-webkit-scrollbar-thumb {
  background: rgba(0, 212, 255, 0.3);
  border-radius: 999px;
}
/* Mobile swipe-row: hide the scrollbar, keep the swipe. */
.swipe-row {
  scrollbar-width: none;
}
.swipe-row::-webkit-scrollbar {
  display: none;
}
</style>
