<!--
  Workstream watch-together — Phase 04 (state-switching) Plan 04.4 Task 1.

  PlayerTabBar.vue — a small horizontal tab switcher mounted inside
  WatchTogetherView so users can request a player swap (aePlayer ↔ Kodik)
  from inside the room. The Anime.vue tabs aren't mounted under
  `/watch/room/:roomId`, so without this component there is no in-room
  way to drive `state:change_player`. Legacy players were retired
  2026-06-17 — only aePlayer + Classic Kodik survive.

  Behavior contract (locked in Plan 04.4 Task 1):
    - Renders one tab per surviving PlayerKind, labels via i18n
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
     * Player kinds to omit from the bar entirely. Defaults to none.
     * WatchTogetherView passes the retired legacy kinds here so the in-room
     * switch only ever offers the surviving aePlayer + Kodik tabs.
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
 * Stable iteration order — matches the dispatch order in WatchTogetherView.vue.
 * Legacy players (kodik-adfree / animelib / ourenglish / hanime / raw) retired
 * 2026-06-17; only the first-party aePlayer + Classic Kodik survive, so the
 * in-room switch offers just those two (aePlayer leads). The PlayerKind union
 * still carries the retired kinds so a pre-deploy room snapshot naming one is
 * recognized (and routed to the view's forward-compat empty state).
 */
const ALL_PLAYERS: readonly PlayerKind[] = ['aeplayer', 'kodik'] as const
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
