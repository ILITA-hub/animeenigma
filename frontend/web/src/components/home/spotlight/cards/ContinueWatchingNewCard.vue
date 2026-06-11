<template>
  <SpotlightCardShell
    accent="pink"
    icon="play"
    :kicker="t('spotlight.continueWatchingNew.title')"
    backdrop="poster-blur"
    :poster-url="data.anime.poster_url"
  >
  <!--
    Workstream hero-spotlight — v4 H-4 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Stepper + episode
    context: the poster ribbon is gone (artwork stays clean, pink glow
    shadow instead), the story is told by the chip chain
    «эп. 4 ✓ → эп. 5 ✓ → эп. 6 · NEW» (SpotlightStepper), backed by the
    status/genre pills, a clamp-2 description and a thin season-progress
    line (SpotlightProgress) when episodes_count is known.
  -->
    <!-- Pink secondary overlay — the "new episode just dropped" wash
         (brand triad: live/personal = pink). -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-pink-500/25 via-transparent to-transparent"
      />
    </template>

    <div class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-8 md:items-center">
      <router-link
        :to="watchUrl"
        class="flex-shrink-0 self-center group"
      >
        <SpotlightPoster
          :poster-url="data.anime.poster_url"
          :alt="title"
          width-class="w-24 md:w-40"
          glow="pink"
          :proxy-width="256"
          img-class="group-hover:scale-105 transition-transform duration-300"
        />
      </router-link>

      <div class="flex-1 min-w-0 max-w-[600px]">
        <h3 class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2">
          {{ title }}
        </h3>

        <div class="mt-2 flex flex-wrap items-center gap-2">
          <Badge v-if="data.anime.status === 'ongoing'" variant="success" size="sm" overlay>
            {{ t('spotlight.randomTail.statusOngoing') }}
          </Badge>
          <Badge
            v-for="g in (data.anime.genres || []).slice(0, 2)"
            :key="g.id"
            size="sm"
            overlay
          >
            {{ locale === 'ru' ? g.russian || g.name : g.name || g.russian }}
          </Badge>
          <span class="text-[13px] text-muted-foreground font-medium">
            {{
              t('spotlight.continueWatchingNew.newEpisodeLine', {
                n: data.new_episode_number,
              })
            }}
          </span>
        </div>

        <p
          v-if="data.anime.description"
          class="mt-2.5 text-[13px] leading-relaxed text-white/70 line-clamp-2"
          data-testid="cwn-desc"
        >
          {{ plainDescription }}
        </p>

        <SpotlightStepper
          class="mt-3"
          :last-watched="data.last_watched_episode"
          :new-episode="data.new_episode_number"
        />

        <SpotlightProgress
          v-if="seasonPercent !== null"
          class="mt-3 max-w-[380px]"
          :percent="seasonPercent"
          accent="pink"
          :label="
            t('spotlight.continueWatchingNew.seasonProgress', {
              cur: data.last_watched_episode,
              total: data.anime.episodes_count,
            })
          "
        />
      </div>
    </div>

    <!-- Deep-link CTA — jump straight to the new episode in the player. -->
    <template #cta>
      <router-link
        :to="watchUrl"
        :class="[buttonVariants({ variant: 'default', size: 'md' }), 'text-sm']"
      >
        <Play class="w-4 h-4" fill="currentColor" aria-hidden="true" />
        {{
          t('spotlight.continueWatchingNew.resumeCtaWithEp', {
            n: data.new_episode_number,
          })
        }}
      </router-link>
    </template>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Play } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import SpotlightStepper from '../ui/SpotlightStepper.vue'
import SpotlightProgress from '../ui/SpotlightProgress.vue'
import { getLocalizedTitle } from '@/utils/title'
import { parseDescription } from '@/utils/description-parser'
import type { ContinueWatchingNewData } from '@/types/spotlight'

const props = defineProps<{ data: ContinueWatchingNewData }>()
const { t, locale: i18nLocale } = useI18n()

const locale = computed(() => {
  const v = (i18nLocale as { value?: unknown }).value
  return typeof v === 'string' ? v : 'ru'
})

const title = computed<string>(() =>
  getLocalizedTitle(
    props.data.anime.name,
    props.data.anime.name_ru,
    props.data.anime.name_jp,
  ),
)

const plainDescription = computed(() => {
  if (!props.data.anime.description) return ''
  const el = document.createElement('div')
  el.innerHTML = parseDescription(props.data.anime.description)
  return el.textContent || ''
})

// Season progress: watched / total. null (no bar) when the total is
// unknown or nonsensical — defensive against episodes_count=0 ongoing.
const seasonPercent = computed<number | null>(() => {
  const total = props.data.anime.episodes_count
  if (!total || total <= 0) return null
  return (props.data.last_watched_episode / total) * 100
})

// Canonical deep-link contract honored by Anime.vue's `queryEpisode` computed
// (and used by the sibling ContinueWatchingRow). `/anime/:id/watch` is only a
// redirect alias, so link straight to `/anime/:id?episode=N` to skip the hop.
const watchUrl = computed<string>(
  () => `/anime/${props.data.anime.id}?episode=${props.data.new_episode_number}`,
)
</script>
