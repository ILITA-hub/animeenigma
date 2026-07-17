<template>
  <SpotlightCardShell
    accent="cyan"
    icon="clock"
    :kicker="t('spotlight.upcomingForYou.title')"
    backdrop="poster-blur"
    :poster-url="current?.anime.poster_url || ''"
  >
    <!--
      Workstream announcement-recs-spotlight — upcoming_for_you card
      (spec 2026-07-17). Login-only announcement matches: the shell is the
      ONLY root (single-root invariant for the parent <Transition
      mode="out-in">); the exhausted "done" state is a body variant, not a
      second root. `match_score` can legitimately be 0 for thin pools and is
      NEVER rendered numerically — the reason line is qualitative only.
    -->
    <!-- Cyan wash — content-core accent. -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-cyan-500/20 via-transparent to-transparent"
      />
    </template>

    <div
      v-if="current"
      class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-8 md:items-center"
    >
      <router-link :to="animeUrl" class="flex-shrink-0 self-center group">
        <SpotlightPoster
          :poster-url="current.anime.poster_url"
          :alt="title"
          width-class="w-24 md:w-40"
          glow="cyan"
          :proxy-width="256"
          img-class="group-hover:scale-105 transition-transform duration-300"
        />
      </router-link>

      <div class="flex-1 min-w-0 max-w-[600px]">
        <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
          {{ title }}
        </h3>

        <div class="mt-2 flex flex-wrap items-center gap-2">
          <Badge variant="success" size="sm" overlay>
            {{ t('spotlight.upcomingForYou.announcedBadge') }}
          </Badge>
          <span
            v-if="current.anime.year"
            class="text-[13px] text-muted-foreground font-medium"
          >
            {{ current.anime.year }}
          </span>
          <span
            v-if="current.anime.kind"
            class="text-[13px] text-muted-foreground font-medium uppercase"
          >
            {{ current.anime.kind }}
          </span>
        </div>

        <p class="mt-2.5 text-[13px] leading-relaxed text-white/70 line-clamp-2" data-testid="ufy-reason">
          {{ reasonLine }}
        </p>
      </div>
    </div>

    <!-- Exhausted state — user acted on every match. -->
    <div v-else class="flex-1 min-h-0 flex items-center">
      <p class="text-[14px] text-white/60" data-testid="ufy-done">
        {{ t('spotlight.upcomingForYou.done') }}
      </p>
    </div>

    <template #cta>
      <div v-if="current" class="flex items-center gap-2">
        <button
          type="button"
          :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
          :disabled="busy"
          data-testid="ufy-add"
          @click="addToPlan"
        >
          <BookmarkPlus class="w-4 h-4" aria-hidden="true" />
          {{ t('spotlight.upcomingForYou.addCta') }}
        </button>
        <button
          type="button"
          :class="[buttonVariants({ variant: 'ghost', size: 'md' }), 'text-sm']"
          :disabled="busy"
          data-testid="ufy-dismiss"
          @click="dismiss"
        >
          <X class="w-4 h-4" aria-hidden="true" />
          {{ t('spotlight.upcomingForYou.dismissCta') }}
        </button>
      </div>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookmarkPlus, X } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import { getLocalizedTitle } from '@/utils/title'
import { apiClient } from '@/api/client'
import { useWatchlistStore } from '@/stores/watchlist'
import type { UpcomingForYouData } from '@/types/spotlight'

const props = defineProps<{ data: UpcomingForYouData }>()
const { t, locale: i18nLocale } = useI18n()
const watchlist = useWatchlistStore()

const idx = ref(0)
const busy = ref(false)

const current = computed(() => props.data.items[idx.value] ?? null)

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const title = computed<string>(() =>
  current.value
    ? getLocalizedTitle(
        current.value.anime.name,
        current.value.anime.name_ru,
        current.value.anime.name_jp,
      )
    : '',
)

const reasonLine = computed<string>(() => {
  const c = current.value
  if (!c) return ''
  if (c.reason.kind === 'franchise' && c.reason.seed_anime_name) {
    const seed =
      locale.value === 'ru'
        ? c.reason.seed_anime_name_ru || c.reason.seed_anime_name
        : c.reason.seed_anime_name
    return t('spotlight.upcomingForYou.reasonFranchise', {
      name: seed,
      score: c.reason.user_score ?? '?',
    })
  }
  return t('spotlight.upcomingForYou.reasonTaste')
})

const animeUrl = computed<string>(
  () => (current.value ? `/anime/${current.value.anime.id}` : '/'),
)

function advance(): void {
  idx.value += 1
}

async function addToPlan(): Promise<void> {
  const c = current.value
  if (!c || busy.value) return
  busy.value = true
  try {
    await watchlist.setStatusOptimistic(c.anime.id, 'plan_to_watch')
    advance()
  } catch (e) {
    // Optimistic store already rolled back; keep the item visible.
    console.warn('[spotlight] upcoming add-to-plan failed', e)
  } finally {
    busy.value = false
  }
}

async function dismiss(): Promise<void> {
  const c = current.value
  if (!c || busy.value) return
  busy.value = true
  try {
    await apiClient.post('/users/recs/upcoming/dismiss', { anime_id: c.anime.id })
    advance()
  } catch (e) {
    console.warn('[spotlight] upcoming dismiss failed', e)
  } finally {
    busy.value = false
  }
}
</script>
