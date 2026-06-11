<template>
  <SpotlightCardShell
    accent="pink"
    :kicker="t('spotlight.nowWatching.title')"
  >
  <!--
    Workstream hero-spotlight — v4 F-2 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). "N в эфире": a big
    pink live counter on the left (works even with a single session —
    the old row list looked half-empty), compact session tiles on the
    right with a pulsing success dot per row. Mobile stacks
    counter-then-list.

    TODO (recorded in spec, future session): Watch Together integration —
    a "join" badge + CTA on rows whose viewer has an invite-open WT room
    (needs wt_room_id? on NowWatching sessions).
  -->
    <template #kicker-lead>
      <SpotlightIcon
        name="pulse"
        class="w-4 h-4 animate-pulse flex-shrink-0"
      />
    </template>

    <div
      class="flex-1 min-h-0 flex flex-col md:grid md:grid-cols-[1fr_1.2fr] gap-4 md:gap-7 md:items-center"
    >
      <!-- ── Live counter ───────────────────────────────────────────── -->
      <div class="text-center md:text-left" data-testid="live-counter">
        <p class="font-display font-semibold leading-none">
          <span class="text-4xl md:text-5xl text-pink-400">{{ count }}</span>
        </p>
        <p class="font-display font-semibold text-lg md:text-xl text-white mt-1.5">
          {{ countLine }}
        </p>
        <p class="text-[13px] text-muted-foreground font-medium mt-2">
          {{ t('spotlight.nowWatching.joinLine') }}
        </p>
      </div>

      <!-- ── Session tiles ──────────────────────────────────────────── -->
      <ul class="flex flex-col gap-2.5 min-h-0 justify-center">
        <SpotlightTile
          v-for="s in data.sessions.slice(0, 3)"
          :key="`${s.public_id}:${s.anime_id}:${s.episode_number}`"
          as="li"
          interactive
        >
          <router-link
            :to="`/anime/${s.anime_id}`"
            class="flex items-center gap-3 p-2.5 min-w-0"
          >
            <Avatar
              :name="s.username"
              size="sm"
              :fallback-class="`${avatarBgClass(s.username)} text-white`"
            />
            <SpotlightPoster
              v-if="s.poster_url"
              :poster-url="s.poster_url"
              alt=""
              width-class="w-9 rounded-md"
              :proxy-width="128"
            />
            <div class="flex-1 flex flex-col min-w-0">
              <p class="text-[13px] font-semibold text-white truncate">
                {{ s.username }}
              </p>
              <p class="text-xs font-medium text-muted-foreground truncate">
                {{ getLocalizedTitle(s.anime_name, s.anime_name_ru) }} · ep
                {{ s.episode_number }}
              </p>
            </div>
            <!-- Pulsing LIVE dot (sr-only text keeps the e2e `text=LIVE`
                 attached-check and SR announcement). -->
            <span
              aria-hidden="true"
              class="w-2.5 h-2.5 rounded-full bg-success animate-pulse flex-shrink-0 live-dot"
            />
            <span class="sr-only">{{ t('spotlight.nowWatching.liveBadge') }}</span>
          </router-link>
        </SpotlightTile>
      </ul>
    </div>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import Avatar from '@/components/ui/Avatar.vue'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import SpotlightTile from '../ui/SpotlightTile.vue'
import SpotlightPoster from '../ui/SpotlightPoster.vue'
import type { NowWatchingData } from '@/types/spotlight'

const props = defineProps<{ data: NowWatchingData }>()
const { t } = useI18n()

const count = computed(() => props.data.sessions.length)

// Russian needs 3 plural forms (1 человек / 2-4 человека / 5+ человек);
// picking the i18n key in code avoids configuring vue-i18n's custom
// pluralization rules for one line. EN/JA map few→many.
const countLine = computed<string>(() => {
  const n = count.value
  const mod10 = n % 10
  const mod100 = n % 100
  if (mod10 === 1 && mod100 !== 11) return t('spotlight.nowWatching.countOne')
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) {
    return t('spotlight.nowWatching.countFew')
  }
  return t('spotlight.nowWatching.countMany')
})

// Avatar color palette: 8 backgrounds covering a wide hue range so
// adjacent usernames feel visibly distinct. Stable order; the hash keeps
// the same username on the same color across mounts and reloads.
const PALETTE = [
  'bg-destructive',
  'bg-orange-500',
  'bg-warning',
  'bg-success',
  'bg-cyan-500',
  'bg-info',
  'bg-brand-violet',
  'bg-pink-500',
] as const

function avatarBgClass(username: string): string {
  let hash = 0
  for (const ch of username) {
    hash = (hash * 31 + ch.charCodeAt(0)) | 0
  }
  return PALETTE[Math.abs(hash) % PALETTE.length]
}
</script>

<style scoped>
/* Soft success glow behind the live dot — composed shadow, no token pair. */
.live-dot {
  box-shadow: 0 0 6px var(--success);
}
</style>
