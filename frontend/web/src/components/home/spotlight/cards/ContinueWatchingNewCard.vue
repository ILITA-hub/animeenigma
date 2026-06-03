<template>
  <article class="relative w-full h-full overflow-hidden">
    <SpotlightBackdrop
      variant="poster-blur"
      accent="purple"
      :poster-url="data.anime.poster_url"
    />
    <!-- Purple secondary overlay — the "new episode just dropped" wash that
         distinguishes this card from NotTimeYet's amber. -->
    <div
      aria-hidden="true"
      class="absolute inset-0 bg-gradient-to-r from-brand-violet/30 via-transparent to-transparent"
    />

    <div
      class="relative z-10 w-full h-full flex flex-col md:flex-row gap-4 md:gap-6 p-4 md:p-6 lg:p-8 md:items-center"
    >
      <!-- Poster + hero ribbon -->
      <router-link
        :to="watchUrl"
        class="relative flex-shrink-0 self-center md:self-start w-32 md:w-40 lg:w-52 group"
      >
        <div
          class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] shadow-2xl shadow-brand-violet/30"
        >
          <img
            :src="data.anime.poster_url || '/placeholder.svg'"
            :alt="title"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
          <!-- Hero ribbon ACROSS the top of the poster -->
          <div
            class="absolute inset-x-0 top-0 px-3 py-1.5 bg-gradient-to-r from-brand-violet to-fuchsia-500 text-white text-xs font-semibold uppercase tracking-wider shadow-lg flex items-center justify-center gap-1.5"
          >
            <SpotlightIcon
              name="play"
              class="w-3.5 h-3.5"
            />
            {{
              t('spotlight.continueWatchingNew.newEpisodeBadge', {
                n: data.new_episode_number,
              })
            }}
          </div>
        </div>
      </router-link>

      <!-- Two-row episode meta with hierarchy -->
      <div class="flex-1 flex flex-col justify-between gap-3 min-w-0">
        <div>
          <div class="flex items-center gap-2 mb-3">
            <SpotlightIcon
              name="play"
              class="w-5 h-5 text-brand-violet"
            />
            <p
              class="text-brand-violet text-sm font-semibold uppercase tracking-[0.15em]"
            >
              {{ t('spotlight.continueWatchingNew.title') }}
            </p>
          </div>

          <h3
            class="text-2xl md:text-3xl font-semibold text-white leading-tight line-clamp-2 mb-3"
          >
            {{ title }}
          </h3>

          <!-- Subdued: where you stopped -->
          <p class="text-xs cwn-muted font-medium">
            {{
              t('spotlight.continueWatchingNew.lastWatched', {
                n: data.last_watched_episode,
              })
            }}
          </p>
          <!-- Accent: what's new -->
          <p
            class="mt-1 text-lg text-brand-violet font-semibold tabular-nums"
          >
            {{
              t('spotlight.continueWatchingNew.newEpisodeLine', {
                n: data.new_episode_number,
              })
            }}
          </p>
        </div>

        <!-- Deep-link CTA — jump straight to the new episode in the player. -->
        <router-link
          :to="watchUrl"
          class="cta-hero"
          data-accent="purple"
        >
          {{
            t('spotlight.continueWatchingNew.resumeCtaWithEp', {
              n: data.new_episode_number,
            })
          }}
          <SpotlightIcon
            name="play"
            class="w-4 h-4"
          />
        </router-link>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SpotlightBackdrop from '../SpotlightBackdrop.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import { getLocalizedTitle } from '@/utils/title'
import type { ContinueWatchingNewData } from '@/types/spotlight'

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

<style scoped>
/* Neon Tokyo token replacements (feat/homepage-neon-tokyo-redesign). */
.cwn-muted { color: var(--muted-foreground); }
</style>
