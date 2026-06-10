<template>
  <SpotlightCardShell
    accent="pink"
    :kicker="t('spotlight.nowWatching.title')"
  >
    <template #kicker-lead>
      <SpotlightIcon
        name="pulse"
        class="w-4 h-4 animate-pulse flex-shrink-0"
      />
    </template>

    <ul class="flex-1 flex flex-col gap-2 min-h-0">
        <li
          v-for="s in data.sessions.slice(0, 3)"
          :key="`${s.public_id}:${s.anime_id}:${s.episode_number}`"
          class="min-w-0"
        >
          <router-link
            :to="`/anime/${s.anime_id}`"
            class="flex items-center gap-3 p-3 rounded-xl bg-white/5 hover:bg-white/10 backdrop-blur-sm transition group min-w-0"
          >
            <!-- Avatar circle (hashed deterministic color via fallbackClass) -->
            <Avatar
              :name="s.username"
              size="md"
              :fallback-class="`${avatarBgClass(s.username)} text-white`"
            >
              <!-- Pulsing LIVE indicator dot (visually hidden text for a11y) -->
              <span
                aria-hidden="true"
                class="absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full bg-success ring-2 ring-background animate-pulse"
              />
              <span class="sr-only">{{
                t('spotlight.nowWatching.liveBadge')
              }}</span>
            </Avatar>

            <!-- Bigger anime poster (56x84) -->
            <img
              v-if="s.poster_url"
              :src="cardPosterUrl(s.poster_url, 128)"
              alt=""
              class="w-14 object-cover rounded-md flex-shrink-0"
              style="height: 84px"
              loading="lazy"
              decoding="async"
            />

            <!-- Text -->
            <div class="flex-1 flex flex-col min-w-0">
              <p
                class="text-sm font-semibold text-white truncate"
              >
                {{ s.username }}
              </p>
              <p class="text-xs font-medium text-muted-foreground truncate">
                {{ getLocalizedTitle(s.anime_name, s.anime_name_ru) }} · ep
                {{ s.episode_number }}
              </p>
            </div>
          </router-link>
        </li>
    </ul>
  </SpotlightCardShell>
</template>

<script setup lang="ts">
/**
 * Workstream hero-spotlight — v1.1-polish Phase 05 (HSB-V11-NW-01..04).
 *
 * Refactor goal: make NowWatchingCard feel alive. Bigger poster thumbs
 * (56×84, 3.5× the previous 32×44), deterministic hashed avatar circles
 * per user, animated cyan→green mesh backdrop via SpotlightBackdrop, and
 * a pulsing LIVE micro-element (green dot at bottom-right of avatar) in
 * place of the previous "LIVE" text label on the right edge.
 *
 * The original "LIVE" string is preserved as `sr-only` text inside the
 * avatar circle so screen readers still announce the live indicator and
 * the existing `spotlight-full.spec.ts` e2e check (`text=LIVE`) keeps
 * matching via toBeAttached (DOM presence, not visual visibility).
 *
 * CRITICAL — single-element root (Phase 04 footgun): the <template> block
 * MUST have exactly one root node (<article>) with NO top-level v-if, NO
 * leading template comments, and NO sibling nodes. HeroSpotlightBlock
 * wraps each card in `<transition mode="out-in">`. If the root ever
 * resolves to a comment node — multi-root template OR top-level v-if
 * false — Vue logs "non-element root node that cannot be animated" and
 * the cross-fade silently wedges: the NEXT card's mount never fires, the
 * carousel stays blank after navigation. Conditional content lives
 * INSIDE <article>, never around it. (Phase 04 e2e regression.)
 */
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle } from '@/utils/title'
import Avatar from '@/components/ui/Avatar.vue'
import SpotlightCardShell from '../SpotlightCardShell.vue'
import SpotlightIcon from '../SpotlightIcon.vue'
import type { NowWatchingData } from '@/types/spotlight'
import { cardPosterUrl } from '@/composables/useImageProxy'

defineProps<{ data: NowWatchingData }>()
const { t } = useI18n()

// Avatar color palette: 8 Tailwind backgrounds covering a wide hue range
// so adjacent usernames feel visibly distinct. Order is stable across
// builds; `avatarBgClass` hashes the username into a palette index so the
// same username always renders the same color across mounts and across
// page reloads (no flicker on data refresh).
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

/**
 * Deterministic username → palette index. Uses the classic 31-multiplier
 * polynomial rolling hash, then `Math.abs(...) % PALETTE.length`. Pure
 * function — exported via the script's binding scope so the spec can
 * exercise determinism (same input → same class across calls) and the
 * hash distribution (different inputs MAY hit different palette slots).
 */
function avatarBgClass(username: string): string {
  let hash = 0
  for (const ch of username) {
    hash = (hash * 31 + ch.charCodeAt(0)) | 0
  }
  return PALETTE[Math.abs(hash) % PALETTE.length]
}
</script>
