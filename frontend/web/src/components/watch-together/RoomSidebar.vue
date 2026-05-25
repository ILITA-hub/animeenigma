<!--
  Workstream watch-together — Phase 02 (frontend-shell) Plan 02.6 Task 1.

  RoomSidebar.vue — the parent shell that composes the three sidebar leaves
  (MemberList + ChatPanel + ReactionPalette) into the locked vertical
  layout from CONTEXT.md §"Component layout". This component is the bridge
  between the `useWatchTogetherRoom()` composable (owned by
  WatchTogetherView in Plan 02.8) and the leaf presentational components
  shipped in Plans 02.4 / 02.5.

  Layout (desktop, >= lg):
    - Outer <aside> as a vertical flex column with `border-l` so it sits
      flush against the player wrapper. Width-locked at lg:w-96 to match
      the design doc's "right rail" dimension; on smaller viewports the
      sidebar collapses to full-width and stacks below the player. Final
      mobile polish (bottom-sheet, tab UI) lands in Phase 5 per
      CONTEXT.md §deferred.
    - MemberList: small section at the top. We do NOT cap its height
      here — MemberList's own <ul> handles overflow internally; a parent
      max-h would interfere with auto-expand when a 10th member joins.
    - ChatPanel: takes the remaining vertical space (`flex-1 min-h-0`).
      The `min-h-0` is mandatory in nested flex containers so the inner
      `overflow-y-auto` can compute a finite height (otherwise the chat
      list grows unbounded and the page scrolls instead of the list).
    - ReactionPalette: pinned to the bottom (`shrink-0`).

  Reconnecting indicator:
    - Subtle amber banner at the very top of the column whenever the
      composable reports a transient connection-loss state. We only
      surface 'reconnecting' here — 'failed' / 'closed' get a full
      empty-state page in WatchTogetherView (Plan 02.8), not a banner.
    - `aria-live="polite"` so screen readers announce the state change
      without yanking focus.

  Why props (not a single `room: WatchTogetherRoomHandle` blob): the leaf
  components only need the destructured fields. Passing the whole handle
  would (a) couple this shell to the composable's full surface (emit/
  subscribe methods we don't use here) and (b) make testing harder by
  forcing every test to construct the full UseWatchTogetherRoomReturn
  shape. The plan's <behavior> contract is the canonical signature.

  UI-SPEC contract (CLAUDE.md):
    - Tailwind utility classes only
    - Font weights: font-medium / font-semibold only (no font-bold etc.)
    - Padding: p-4 md:p-6 where appropriate
-->

<script setup lang="ts">
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
// MemberList.vue (which independently reads its `(you)` target from the
// store). Cheap and avoids forcing the parent to plumb it twice.
const authStore = useAuthStore()

function onChatSend(body: string): void {
  props.sendChat(body)
}
</script>

<template>
  <aside
    class="flex flex-col h-full w-full lg:w-96 border-l border-foreground/10 bg-background/95"
    :aria-label="t('watch_together.title')"
  >
    <div
      v-if="connectionStatus === 'reconnecting'"
      role="status"
      aria-live="polite"
      class="shrink-0 px-4 py-2 text-xs font-medium text-amber-300 bg-amber-500/10 border-b border-amber-500/20 flex items-center gap-2"
    >
      <span
        aria-hidden="true"
        class="inline-block w-2 h-2 rounded-full bg-amber-300 animate-pulse"
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
</template>
