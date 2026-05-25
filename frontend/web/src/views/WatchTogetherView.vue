<!--
  Workstream watch-together — Phase 02 (frontend-shell) Plan 02.8.

  WatchTogetherView.vue — the route view at `/watch/room/:roomId`. This is
  the glue layer that:

    1. Pre-fetches the room snapshot via REST so we can branch on 410-Gone
       BEFORE burning a WebSocket upgrade (Plan 02.1 contract). On success
       we mount the composable; on RoomGoneError we render the "room ended"
       empty state with a back-to-anime button.

    2. Instantiates `useWatchTogetherRoom(roomId)` and calls connect().
       The composable owns the WS lifecycle, snapshot replay, reconnect
       backoff, and reactive room/members/messages/reactions state.

    3. Dispatches to one of the 5 existing `<*Player>` components based on
       `room.player`. Player imports go through `defineAsyncComponent` so
       the player chunks stay independent and do NOT inflate this view's
       chunk (WT-NF-04: <30KB gz target). Players accept `:room` but
       ignore it in Phase 2 — sync wiring lands in Phase 3.

    4. Mounts the RoomSidebar (right rail / below on mobile) and the
       ReactionBurstOverlay (absolute over the player). Both bind directly
       to the composable's reactive refs.

    5. Handles three exceptional states via composable subscriptions:
         - CAPACITY_FULL → "Room is full" page with back button
         - AUTH_EXPIRED  → router.push to /auth, preserving returnUrl
         - onRoomClosed  → transitions to the "room ended" branch (the
                           same one we'd hit on REST 410)

  Layout (desktop, >= lg):
    grid grid-cols-[1fr_360px] gap-4 → player column + sidebar column.
    The ReactionBurstOverlay is a sibling of the player inside a
    `relative` wrapper so its `absolute inset-0` covers the video.

  Layout (mobile, <lg):
    Single column, player on top, sidebar below. Final mobile polish
    (bottom-sheet tabs) is deferred to Phase 5 per CONTEXT.md §deferred.

  UI-SPEC contract (CLAUDE.md):
    - Tailwind utility classes only.
    - Font weights: font-medium / font-semibold only (NO font-bold).
    - Padding: p-4 md:p-6 lg:p-8 on the outer container.

  Deviation noted in SUMMARY:
    The existing router beforeEach guard already redirects unauthenticated
    users via `sessionStorage.setItem('returnUrl', to.fullPath)` → /auth.
    On AUTH_EXPIRED (mid-session), we do BOTH: write returnUrl to session
    storage (so the existing guard hook will resume on next-mount) AND
    push to /auth with an explicit `next=` query param (so the URL itself
    carries the resume target for users that bookmark or share the auth
    URL). This is belt-and-suspenders, matching CONTEXT.md §"Capacity /
    errors" intent.
-->

<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'

import {
  getRoom,
  RoomGoneError,
  ERR_CAPACITY_FULL,
  ERR_AUTH_EXPIRED,
  ERR_EPISODE_UNAVAILABLE,
  ERR_PLAYER_UNAVAILABLE,
  ERR_TRANSLATION_UNAVAILABLE,
  type PlayerKind,
} from '@/api/watch-together'
import { useWatchTogetherRoom } from '@/composables/useWatchTogetherRoom'
import { useToast } from '@/composables/useToast'
import RoomSidebar from '@/components/watch-together/RoomSidebar.vue'
import ReactionBurstOverlay from '@/components/watch-together/ReactionBurstOverlay.vue'
import SyncToastStack from '@/components/watch-together/SyncToastStack.vue'
import ConnectionStatusOverlay from '@/components/watch-together/ConnectionStatusOverlay.vue'
import PlayerTabBar from '@/components/watch-together/PlayerTabBar.vue'

// Lazy-load each player so the WatchTogetherView chunk stays under the
// 30KB gz budget (WT-NF-04). Mirrors Anime.vue's defineAsyncComponent
// pattern; Vite emits one chunk per dynamic import → players load only
// when their branch is rendered.
const KodikPlayer = defineAsyncComponent(() => import('@/components/player/KodikPlayer.vue'))
const AnimeLibPlayer = defineAsyncComponent(() => import('@/components/player/AnimeLibPlayer.vue'))
const OurEnglishPlayer = defineAsyncComponent(() => import('@/components/player/OurEnglishPlayer.vue'))
const HanimePlayer = defineAsyncComponent(() => import('@/components/player/HanimePlayer.vue'))
const RawPlayer = defineAsyncComponent(() => import('@/components/player/RawPlayer.vue'))

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

// roomId is treated as a string; the router parameter is guaranteed by
// the route definition (`/watch/room/:roomId`). encodeURIComponent in the
// API client makes the SUT robust against pathological IDs.
const roomId = String(route.params.roomId)

/**
 * View states. Mutually exclusive — the template renders exactly one
 * top-level branch. `errorState` overrides `loading` and the live view;
 * `loading` overrides the live view.
 */
type ErrorState = 'gone' | 'capacity' | null
const loading = ref(true)
const errorState = ref<ErrorState>(null)

// Cached anime id from the snapshot — used by the back button in the
// empty / capacity states. Captured from the REST snapshot since the
// composable's `room` ref might already be null by the time we render
// the empty state (room:closed clears it).
const lastAnimeId = ref<string | null>(null)

// Instantiate the composable once. The composable's internal state lives
// across the view's lifetime; auto-disconnect on unmount is wired both by
// the composable's onUnmounted hook AND our own explicit onBeforeUnmount
// (defense in depth — explicit disconnect is cheap and idempotent).
const roomHandle = useWatchTogetherRoom(roomId)

// Plan 04.4 — toast composable for sender-only state-error surfacing.
const toast = useToast()

// Subscribe to terminal events from the composable. Both unsubscribers
// are eaten — the handle's auto-disconnect cleans up on unmount.
roomHandle.onError((e) => {
  // Plan 04.4 state-switching: sender-only error codes from
  // services/watch-together's validated change_{episode,player,translation}
  // handlers. Surface as toast, do NOT mutate local state — the broadcast
  // for a successful change would arrive separately as room:state_changed.
  if (e.code === ERR_EPISODE_UNAVAILABLE) {
    toast.push(t('watch_together.state_change_episode_unavailable'), 'error')
    return
  }
  if (e.code === ERR_PLAYER_UNAVAILABLE) {
    toast.push(t('watch_together.state_change_player_unavailable'), 'error')
    return
  }
  if (e.code === ERR_TRANSLATION_UNAVAILABLE) {
    toast.push(t('watch_together.state_change_translation_unavailable'), 'error')
    return
  }
  if (e.code === ERR_CAPACITY_FULL) {
    errorState.value = 'capacity'
    return
  }
  if (e.code === ERR_AUTH_EXPIRED) {
    // Belt-and-suspenders: write returnUrl + explicit ?next= query so the
    // resume target is preserved across BOTH redirect paths.
    try {
      sessionStorage.setItem('returnUrl', route.fullPath)
    } catch {
      // Privacy modes can throw; silent failure is OK.
    }
    router.push({ path: '/auth', query: { next: route.fullPath } })
    return
  }
})

roomHandle.onRoomClosed(() => {
  errorState.value = 'gone'
})

// Pre-fetch REST snapshot to short-circuit 410-Gone before opening the
// WebSocket. On success, hand off to the composable's connect() which
// will replay an authoritative `room:snapshot` over WS.
async function bootstrap() {
  loading.value = true
  try {
    const snap = await getRoom(roomId)
    lastAnimeId.value = snap.room.anime_id
    // The composable's connect() will refetch internally too — that's
    // fine; the snapshot is idempotent and the second call comes from
    // the SAME network round-trip path. (We could pre-pass it, but the
    // composable's API doesn't expose a seed-from-snapshot variant.)
    await roomHandle.connect()
  } catch (err) {
    if (err instanceof RoomGoneError) {
      errorState.value = 'gone'
      return
    }
    // Any other failure surfaces as a generic "ended" state. The
    // composable's own onError will fire for protocol-level errors;
    // REST-level failures (network blip, 500, etc.) land here. We
    // intentionally show the same "ended" UX so users get one back
    // button rather than three flavors of "something went wrong".
    errorState.value = 'gone'
  } finally {
    loading.value = false
  }
}

// Fire the bootstrap immediately — there's no `await` needed at the
// top level; Vue will render `loading` until the promise settles.
bootstrap()

// Explicit disconnect on unmount. Idempotent — the composable also
// auto-disconnects via its own onUnmounted hook, but doing it here
// guarantees the WS closes even if the composable was instantiated
// outside a setup context (defensive — the current path always IS
// inside setup, but the test surface and future refactors benefit).
onBeforeUnmount(() => {
  roomHandle.disconnect()
})

// Plan 04.4 — PlayerTabBar @select-player handler. Routes ALL user-driven
// player changes through the room handle so the backend validates +
// broadcasts; the composable's auto-mutation of room.value.player then
// flips the :key on the active player branch and Vue re-mounts cleanly.
// Defense in depth: PlayerTabBar already guards against same-kind clicks,
// but the view re-checks here so direct programmatic emit calls (tests,
// future shortcuts) can't accidentally fire spurious change_player.
function onSelectPlayer(player: PlayerKind) {
  if (player === livePlayer.value) return
  roomHandle.emitChangePlayer(player)
}

// Navigation back to the anime page from empty/capacity states. Falls
// back to home if we don't have an anime id (e.g. the REST pre-fetch
// itself 410'd before we could read the snapshot).
function goBackToAnime() {
  if (lastAnimeId.value) {
    router.push(`/anime/${lastAnimeId.value}`)
    return
  }
  router.push('/')
}

// Computed accessors that unwrap the composable's refs for template
// consumption. We pass these (not the refs themselves) to RoomSidebar
// since its props are typed as the unwrapped shapes (Plan 02.6).
const liveRoom = computed(() => roomHandle.room.value)
const liveMembers = computed(() => roomHandle.members.value)
const liveMessages = computed(() => roomHandle.messages.value)
const liveReactions = computed(() => roomHandle.reactions.value)
const liveConnectionStatus = computed(() => roomHandle.connectionStatus.value)
const livePlayer = computed(() => roomHandle.room.value?.player ?? null)

// Episode id passed to each player. Phase 2 keeps the string-as-is
// (per CONTEXT.md §"Player mounting in WatchTogetherView" — players
// already handle the string). Conversion to a numeric `initialEpisode`
// where required by the player is a Phase 3 concern.
const initialEpisode = computed(() => {
  const ep = roomHandle.room.value?.episode_id
  if (!ep) return undefined
  const n = Number.parseInt(ep, 10)
  return Number.isFinite(n) && n > 0 ? n : undefined
})

const animeId = computed(() => roomHandle.room.value?.anime_id ?? lastAnimeId.value ?? '')
</script>

<template>
  <!-- Loading state — REST pre-fetch in flight. -->
  <div
    v-if="loading"
    class="flex items-center justify-center min-h-screen p-4 md:p-6 lg:p-8 text-foreground/80 text-base font-medium"
    role="status"
    aria-live="polite"
  >
    {{ t('watch_together.loading') }}
  </div>

  <!-- Room ended — REST 410 OR composable.onRoomClosed fired. -->
  <div
    v-else-if="errorState === 'gone'"
    class="flex flex-col items-center justify-center min-h-screen p-4 md:p-6 lg:p-8 text-center gap-4"
  >
    <h2 class="text-2xl font-semibold">{{ t('watch_together.room_ended_title') }}</h2>
    <button
      type="button"
      class="px-4 py-2 rounded-md bg-primary text-primary-foreground font-medium hover:bg-primary/90 transition-colors"
      @click="goBackToAnime"
    >
      {{ t('watch_together.room_ended_back_button') }}
    </button>
  </div>

  <!-- Capacity full — composable.onError fired with CAPACITY_FULL. -->
  <div
    v-else-if="errorState === 'capacity'"
    class="flex flex-col items-center justify-center min-h-screen p-4 md:p-6 lg:p-8 text-center gap-4"
  >
    <h2 class="text-2xl font-semibold">{{ t('watch_together.capacity_full_title') }}</h2>
    <button
      type="button"
      class="px-4 py-2 rounded-md bg-primary text-primary-foreground font-medium hover:bg-primary/90 transition-colors"
      @click="goBackToAnime"
    >
      {{ t('watch_together.capacity_full_back_button') }}
    </button>
  </div>

  <!-- Live room layout — player column + sidebar column, burst overlay
       absolute over the player. -->
  <div
    v-else
    class="flex flex-col lg:flex-row min-h-screen w-full"
  >
    <!-- Player column — relative so the burst overlay can `absolute inset-0` -->
    <div class="relative flex-1 min-w-0 bg-black">
      <!-- Plan 04.4 — PlayerTabBar overlays the top-left of the player.
           Routes user-driven player switches through roomHandle.emitChangePlayer;
           the broadcast's room:state_changed event flips livePlayer which
           tears down the old player (via :key) and mounts the new one. -->
      <PlayerTabBar
        class="absolute top-2 left-2 z-20"
        :active-player="livePlayer"
        @select-player="onSelectPlayer"
      />

      <KodikPlayer
        v-if="livePlayer === 'kodik'"
        :key="`player-${livePlayer}`"
        :anime-id="animeId"
        :initial-episode="initialEpisode"
        :room="roomHandle"
      />
      <AnimeLibPlayer
        v-else-if="livePlayer === 'animelib'"
        :key="`player-${livePlayer}`"
        :anime-id="animeId"
        :initial-episode="initialEpisode"
        :room="roomHandle"
      />
      <OurEnglishPlayer
        v-else-if="livePlayer === 'ourenglish'"
        :key="`player-${livePlayer}`"
        :anime-id="animeId"
        :initial-episode="initialEpisode"
        :room="roomHandle"
      />
      <HanimePlayer
        v-else-if="livePlayer === 'hanime'"
        :key="`player-${livePlayer}`"
        :anime-id="animeId"
        :initial-episode="initialEpisode"
        :room="roomHandle"
      />
      <RawPlayer
        v-else-if="livePlayer === 'raw'"
        :key="`player-${livePlayer}`"
        :anime-id="animeId"
        :room="roomHandle"
      />
      <!-- Forward-compat: unknown player kind → empty state, NOT a crash.
           The protocol may add a 6th player; this guards against the WS
           snapshot delivering an unrecognized PlayerKind. -->
      <div
        v-else
        class="flex items-center justify-center w-full h-full text-foreground/60 font-medium"
      >
        {{ t('watch_together.loading') }}
      </div>

      <ConnectionStatusOverlay :status="liveConnectionStatus" />
      <SyncToastStack :room="roomHandle" />
      <ReactionBurstOverlay :reactions="liveReactions" />
    </div>

    <!-- Sidebar column — desktop right rail, mobile below. -->
    <RoomSidebar
      :room="liveRoom"
      :members="liveMembers"
      :messages="liveMessages"
      :send-chat="roomHandle.sendChat"
      :send-reaction="roomHandle.sendReaction"
      :connection-status="liveConnectionStatus"
    />
  </div>
</template>
