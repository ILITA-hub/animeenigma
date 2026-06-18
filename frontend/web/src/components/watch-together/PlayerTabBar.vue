<!--
  Workstream watch-together — Phase 04 (state-switching) Plan 04.4 Task 1.

  PlayerTabBar.vue — a small horizontal 6-tab switcher mounted inside
  WatchTogetherView so users can request a player swap (kodik → animelib
  etc.) from inside the room. The Anime.vue tabs aren't mounted under
  `/watch/room/:roomId`, so without this component there is no in-room
  way to drive `state:change_player`.

  Behavior contract (locked in Plan 04.4 Task 1):
    - Renders 6 tabs, one per PlayerKind, labels via i18n
      (watch_together.player_tab_<kind>).
    - The tab whose data-player matches `activePlayer` carries
      aria-selected="true" and an active-state class set.
    - Clicking an inactive tab emits `select-player` with the kind.
    - Clicking the active tab is a no-op (defense in depth — the
      view-level handler also guards against this).
    - `disabled=true` paints aria-disabled on every tab and suppresses
      all emits (used while the WS is disconnected, for example).

  UI-SPEC contract (CLAUDE.md):
    - Tailwind utility classes only.
    - Font weights: font-medium / font-semibold only (no bolder weights).
    - Active = bg-primary text-primary-foreground;
      inactive = bg-muted/40 text-foreground/70 hover:bg-muted/60.
-->

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlayerKind } from '@/api/watch-together'

const props = withDefaults(
  defineProps<{
    /** The room's current player kind (drives the active-state styling). */
    activePlayer: PlayerKind | null
    /** When true: aria-disabled on every tab, no emits. */
    disabled?: boolean
    /**
     * Player kinds to omit from the bar entirely. Defaults to none, preserving
     * the original 5-tab contract. WatchTogetherView passes `['animelib']`
     * while the AniLib provider is hidden (see Anime.vue animeLibEnabled).
     */
    hiddenKinds?: readonly PlayerKind[]
  }>(),
  {
    disabled: false,
    hiddenKinds: () => [],
  },
)

const emit = defineEmits<{
  (e: 'select-player', player: PlayerKind): void
}>()

const { t } = useI18n()

/**
 * Stable iteration order — matches the 6-way dispatch order in
 * WatchTogetherView.vue and the PlayerKind union order in types/.
 * Keep in sync with the PlayerKind union in src/types/watch-together.ts.
 */
const ALL_PLAYERS: readonly PlayerKind[] = ['aeplayer', 'kodik', 'kodik-adfree', 'animelib', 'ourenglish', 'hanime', 'raw'] as const
const PLAYERS = computed(() => ALL_PLAYERS.filter((p) => !props.hiddenKinds.includes(p)))

function onTabClick(kind: PlayerKind) {
  if (props.disabled) return
  if (kind === props.activePlayer) return
  emit('select-player', kind)
}
</script>

<template>
  <div
    role="tablist"
    aria-label="Player switcher"
    class="flex flex-wrap items-center gap-1.5 p-1.5 rounded-md bg-background/80 backdrop-blur-sm"
  >
    <button
      v-for="kind in PLAYERS"
      :key="kind"
      type="button"
      role="tab"
      :data-player="kind"
      :aria-selected="kind === activePlayer ? 'true' : 'false'"
      :aria-disabled="disabled ? 'true' : 'false'"
      :class="[
        'px-3 py-1.5 rounded-md text-sm font-medium transition-colors',
        kind === activePlayer
          ? 'bg-primary text-primary-foreground font-semibold'
          : 'bg-muted/40 text-foreground/70 hover:bg-muted/60',
        disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
      ]"
      @click="onTabClick(kind)"
    >
      {{ t(`watch_together.player_tab_${kind}`) }}
    </button>
  </div>
</template>
