<!--
  Workstream watch-together — Phase 02 (frontend-shell) Plan 02.6 Task 1
                            + Phase 05 (polish)  Plan 05.4 Task 2 (WT-POLISH-03).

  RoomSidebar.vue — dual-mode parent shell that composes MemberList +
  ChatPanel + ReactionPalette into:

  Desktop layout (>= lg, 1024px+):
    - Outer <aside> as a vertical flex column with `border-l` so it sits
      flush against the player wrapper. Width-locked at lg:w-96 to match
      the design doc's "right rail" dimension.
    - MemberList → ChatPanel (flex-1 min-h-0) → ReactionPalette stack.
    - Reconnecting banner pinned to the top.
    - Behavior identical to Phase 2 — wrapped now in `hidden lg:flex` so
      CSS gating on viewport keeps it desktop-only.

  Mobile layout (< lg) — Plan 05.4 WT-POLISH-03:
    - Bottom-anchored sheet via `fixed inset-x-0 bottom-0 z-30`. Sits
      OVER the player area at the bottom of the viewport so the player
      stays visible at the top.
    - 2-tab bar (Chat | Reactions) per WT-POLISH-03. Members are NOT a
      separate tab — instead the tab bar shows a compact "n/10" count
      on the right (keeps the tab count at 2 as the spec requires).
    - Collapsed state: 80px (just the tab bar + count visible).
    - Expanded state: 60vh (active tab body visible above the tab bar).
    - Tap the active tab again → collapses (toggle); tap a different tab →
      switches active without collapsing.
    - Touch drag-up (>50px) expands; drag-down (>50px) collapses. Smaller
      jitter is ignored. CSS transition fallback if touch isn't available.

  Both branches are rendered in the DOM tree at all times; Tailwind's
  `lg:hidden` / `hidden lg:flex` controls visibility. This is simpler than
  a JS-side viewport guard and lets jsdom-based tests assert both branches
  without simulating window.matchMedia.

  Reconnecting indicator:
    - Subtle amber banner at the top of BOTH branches. Surfaces only
      'reconnecting' — 'failed' / 'closed' get a full empty-state page in
      WatchTogetherView, not a banner. `aria-live="polite"` so screen
      readers announce without yanking focus.

  WatchTogetherView outer:
    - Needs `pb-20 lg:pb-0` on its `<lg` flex column so the player has
      80px of clearance below it for the collapsed mobile sheet.

  UI-SPEC contract (CLAUDE.md):
    - Tailwind utility classes only
    - Font weights: font-medium / font-semibold only (no font-bold etc.)
    - Padding: p-4 md:p-6 where appropriate
-->

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'

import MemberList from './MemberList.vue'
import ChatPanel from './ChatPanel.vue'
import ReactionPalette from './ReactionPalette.vue'

import { useAuthStore } from '@/stores/auth'
import type { Room, Member, ChatMessage } from '@/api/watch-together'
import type { ConnectionStatus } from '@/composables/useWatchTogetherRoom'

const props = defineProps<{
  /** Authoritative room snapshot from the composable. `null` while the
   *  REST pre-fetch is in flight (composable's initial state).
   */
  room: Room | null
  /** Roster of members in the room (composable's `members.value`). */
  members: Member[]
  /** Chat backlog (composable's `messages.value`). */
  messages: ChatMessage[]
  /** Bridge to the composable's `sendChat(body)` method. */
  sendChat: (body: string) => void
  /** Bridge to the composable's `sendReaction(emoji)` method. */
  sendReaction: (emoji: string) => void
  /** Connection lifecycle state for the reconnecting banner. */
  connectionStatus: ConnectionStatus
}>()

const { t } = useI18n()

// The composable owns the local user id, but ChatPanel needs it to bubble
// own-vs-other styling. Read once from the auth store — same pattern as
// MemberList.vue. Cheap and avoids forcing the parent to plumb it twice.
const authStore = useAuthStore()

function onChatSend(body: string): void {
  props.sendChat(body)
}

// ── Mobile bottom-sheet state (Plan 05.4) ────────────────────────────────
//
// Two tabs only per WT-POLISH-03; members are shown as a count strip in the
// tab bar header rather than as a separate tab.

type SheetTab = 'chat' | 'reactions'

const tabs: ReadonlyArray<{ key: SheetTab; i18nKey: string }> = [
  { key: 'chat', i18nKey: 'watch_together.bottom_sheet_tab_chat' },
  { key: 'reactions', i18nKey: 'watch_together.bottom_sheet_tab_reactions' },
]

const activeTab = ref<SheetTab>('chat')
const sheetExpanded = ref<boolean>(false)
const sheetHeight = computed(() => (sheetExpanded.value ? '60vh' : '80px'))

// Project-wide hard cap from Phase 1 (rooms service enforces 10 max members).
const MAX_MEMBERS = 10

function onTabClick(key: SheetTab): void {
  if (key === activeTab.value && sheetExpanded.value) {
    // Toggle collapse when re-tapping the active tab.
    sheetExpanded.value = false
    return
  }
  activeTab.value = key
  sheetExpanded.value = true
}

// Touch-drag gesture: capture clientY at touchstart, accumulate dy on
// touchmove, and decide at touchend. A 50px threshold filters jitter.
let touchStartY = 0
let lastDeltaY = 0

function onTouchStart(e: TouchEvent): void {
  const t0 = e.touches[0]
  if (!t0) return
  touchStartY = t0.clientY
  lastDeltaY = 0
}

function onTouchMove(e: TouchEvent): void {
  const t0 = e.touches[0]
  if (!t0) return
  lastDeltaY = t0.clientY - touchStartY
}

function onTouchEnd(): void {
  if (lastDeltaY < -50) {
    // Dragged UP at least 50px → expand.
    sheetExpanded.value = true
  } else if (lastDeltaY > 50) {
    // Dragged DOWN at least 50px → collapse.
    sheetExpanded.value = false
  }
  // else: small jitter, no-op
  lastDeltaY = 0
}
</script>

<template>
  <!-- Desktop right-rail (>= lg) — Phase 2 layout, gated CSS-only. -->
  <aside
    class="hidden lg:flex flex-col h-full w-full lg:w-96 border-l border-foreground/10 bg-background/95"
    :aria-label="t('watch_together.title')"
  >
    <div
      v-if="connectionStatus === 'reconnecting'"
      role="status"
      aria-live="polite"
      class="shrink-0 px-4 py-2 text-xs font-medium text-warning bg-warning/10 border-b border-warning/20 flex items-center gap-2"
    >
      <span
        aria-hidden="true"
        class="inline-block w-2 h-2 rounded-full bg-warning animate-pulse"
      />
      {{ t('watch_together.reconnecting_indicator') }}
    </div>

    <div class="shrink-0 border-b border-foreground/10">
      <MemberList
        :members="members"
        :host-user-id="room?.host_user_id ?? ''"
      />
    </div>

    <div class="flex-1 min-h-0">
      <ChatPanel
        :messages="messages"
        :current-user-id="authStore.user?.id ?? ''"
        @send="onChatSend"
      />
    </div>

    <div class="shrink-0 border-t border-foreground/10">
      <ReactionPalette :send-reaction="sendReaction" />
    </div>
  </aside>

  <!-- Mobile bottom-sheet (< lg) — Plan 05.4 WT-POLISH-03. -->
  <aside
    class="lg:hidden fixed inset-x-0 bottom-0 z-30 flex flex-col bg-background/95 border-t border-foreground/10 transition-[height] duration-200 ease-out"
    :style="{ height: sheetHeight }"
    :aria-expanded="sheetExpanded"
    :aria-label="t('watch_together.title')"
    @touchstart.passive="onTouchStart"
    @touchmove.passive="onTouchMove"
    @touchend.passive="onTouchEnd"
  >
    <div
      v-if="connectionStatus === 'reconnecting'"
      role="status"
      aria-live="polite"
      class="shrink-0 px-4 py-1 text-xs font-medium text-warning bg-warning/10 border-b border-warning/20 flex items-center gap-2"
    >
      <span
        aria-hidden="true"
        class="inline-block w-2 h-2 rounded-full bg-warning animate-pulse"
      />
      {{ t('watch_together.reconnecting_indicator') }}
    </div>

    <!-- Tab bar header (always visible when sheet is collapsed OR expanded). -->
    <div
      class="shrink-0 flex items-center justify-between px-4 py-2 border-b border-foreground/10"
      role="tablist"
      :aria-label="t('watch_together.title')"
    >
      <div class="flex gap-2">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.key"
          class="px-3 py-1.5 rounded-md font-medium text-sm transition-colors"
          :class="
            activeTab === tab.key
              ? 'bg-primary text-primary-foreground'
              : 'text-foreground/70 hover:text-foreground'
          "
          @click="onTabClick(tab.key)"
        >
          {{ t(tab.i18nKey) }}
        </button>
      </div>
      <div
        class="text-xs text-foreground/60 font-medium"
        :aria-label="t('watch_together.members_heading')"
      >
        {{ members.length }}/{{ MAX_MEMBERS }}
      </div>
    </div>

    <!-- Active tab body — only rendered when sheet is expanded so the
         collapsed 80px footprint doesn't leak through child layout. -->
    <div v-show="sheetExpanded" class="flex-1 min-h-0">
      <div v-if="activeTab === 'chat'" class="flex flex-col h-full">
        <ChatPanel
          :messages="messages"
          :current-user-id="authStore.user?.id ?? ''"
          @send="onChatSend"
        />
      </div>
      <div
        v-else-if="activeTab === 'reactions'"
        class="h-full overflow-y-auto"
      >
        <ReactionPalette :send-reaction="sendReaction" />
      </div>
    </div>
  </aside>
</template>
