<template>
  <SpotlightCardShell
    accent="pink"
    icon="play"
    :kicker="t('spotlight.continueWatchingNew.title')"
    backdrop="poster-blur"
    :poster-url="data.anime.poster_url"
  >
    <!-- Pink secondary overlay — the "new episode just dropped" wash
         (brand triad: live/personal = pink). -->
    <template #background-extra>
      <div
        aria-hidden="true"
        class="absolute inset-0 bg-gradient-to-r from-pink-500/25 via-transparent to-transparent"
      />
    </template>

    <div class="flex-1 min-h-0 flex flex-col md:flex-row gap-4 md:gap-6 md:items-center">
      <!-- Poster + hero ribbon -->
      <router-link
        :to="watchUrl"
        class="relative flex-shrink-0 self-center w-24 md:w-32 lg:w-40 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-pink-500/30"
        >
          <img
            :src="cardPosterUrl(data.anime.poster_url, 256)"
            :alt="title"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
            decoding="async"
          />
          <!-- Hero ribbon ACROSS the top of the poster -->
          <div
            class="absolute inset-x-0 top-0 px-3 py-1.5 bg-gradient-to-r from-pink-500 to-pink-400 text-white text-xs font-semibold uppercase tracking-wider shadow-lg flex items-center justify-center gap-1.5"
          >
            <Play class="w-3.5 h-3.5" fill="currentColor" aria-hidden="true" />
            {{
              t('spotlight.continueWatchingNew.newEpisodeBadge', {
                n: data.new_episode_number,
              })
            }}
          </div>
        </div>
      </router-link>

      <!-- Two-row episode meta with hierarchy -->
      <div class="flex-1 min-w-0">
        <h3
          class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2 mb-3"
        >
          {{ title }}
        </h3>

        <!-- Subdued: where you stopped -->
        <p class="text-xs text-muted-foreground font-medium">
          {{
            t('spotlight.continueWatchingNew.lastWatched', {
              n: data.last_watched_episode,
            })
          }}
        </p>
        <!-- Accent: what's new -->
        <p
          class="mt-1 text-lg text-pink-400 font-semibold tabular-nums"
        >
          {{
            t('spotlight.continueWatchingNew.newEpisodeLine', {
              n: data.new_episode_number,
            })
          }}
        </p>
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
// Workstream hero-spotlight — DS alignment 2026-06-10: SpotlightCardShell
// anatomy (pink kicker per brand triad, CTA bottom-left via Button default
// variant), pink ribbon/accents replacing the old violet/fuchsia mix.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Play } from 'lucide-vue-next'
import { buttonVariants } from '@/components/ui/button-variants'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import { getLocalizedTitle } from '@/utils/title'
import type { ContinueWatchingNewData } from '@/types/spotlight'
import { cardPosterUrl } from '@/composables/useImageProxy'

const props = defineProps<{ data: ContinueWatchingNewData }>()
const { t } = useI18n()

const title = computed<string>(() =>
  getLocalizedTitle(
    props.data.anime.name,
    props.data.anime.name_ru,
    props.data.anime.name_jp,
  ),
)

// Canonical deep-link contract honored by Anime.vue's `queryEpisode` computed
// (and used by the sibling ContinueWatchingRow). `/anime/:id/watch` is only a
// redirect alias, so link straight to `/anime/:id?episode=N` to skip the hop.
const watchUrl = computed<string>(
  () => `/anime/${props.data.anime.id}?episode=${props.data.new_episode_number}`,
)
</script>
