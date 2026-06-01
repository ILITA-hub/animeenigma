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
 *
 * Plan 05.5 added the `auth-expired` branch which renders a blocking
 * modal (vs Phase 2's immediate router.push) — gives the user a moment
 * to read before navigating, and avoids redirecting users who close the
 * tab without ever consenting to /auth.
 */
type ErrorState = 'gone' | 'capacity' | 'auth-expired' | null
const loading = ref(true)
const errorState = ref<ErrorState>(null)

// Cached anime id from the snapshot — used by the back button in the
// empty / capacity states + the mid-session room:closed redirect. We
// also persist it to sessionStorage so a WS-only room:closed event has
// a redirect target even after the composable's `room` ref has been
// cleared (or on a page reload where the REST snapshot races the WS).
//
// Key is scoped per-room so two simultaneously-mounted views don't
// stomp each other (defensive — the router only ever mounts one view
// at a time, but the cost of scoping is zero).
const LAST_ANIME_ID_STORAGE_KEY = `wt-last-anime-id-${roomId}`
const lastAnimeId = ref<string | null>(null)
try {
  const cached = sessionStorage.getItem(LAST_ANIME_ID_STORAGE_KEY)
  if (cached) lastAnimeId.value = cached
} catch {
  // Privacy modes can throw on sessionStorage access; silent failure.
}
// Static modal heading id — referenced by aria-labelledby on the dialog.
const modalTitleId = 'wt-auth-expired-modal-title'

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
  // Plan 05.5: AUTH_EXPIRED handled via the dedicated onAuthExpired
  // subscription below — keeps the modal branch decoupled from the
  // catch-all onError dispatcher. The ERR_AUTH_EXPIRED constant is still
  // imported as a compile-time guard against drift.
  void ERR_AUTH_EXPIRED
})

roomHandle.onAuthExpired(() => {
  // Plan 05.5 (WT-POLISH-06): present a blocking modal instead of an
  // immediate router.push. The user clicks Login to navigate to /auth
  // (preserving the resume URL); if they navigate away without clicking,
  // they don't end up at an auth page they didn't ask for.
  errorState.value = 'auth-expired'
})

roomHandle.onRoomClosed(() => {
  // Plan 05.5 (WT-POLISH-05): mid-session room:closed event arrives via
  // WS while the user is actively watching. Redirect to the anime watch
  // page with a toast so the user lands somewhere useful — vs the REST
  // 410 path (bootstrap catch block) which still shows the empty
  // "room ended" landing page because the user typed a stale URL.
  toast.push(t('watch_together.room_ended_redirect_toast'), 'info')
  const target = lastAnimeId.value ? `/anime/${lastAnimeId.value}/watch` : '/'
  try {
    sessionStorage.removeItem(LAST_ANIME_ID_STORAGE_KEY)
  } catch {
    // Privacy modes can throw; silent failure is OK.
  }
  router.push(target)
})

// Pre-fetch REST snapshot to short-circuit 410-Gone before opening the
// WebSocket. On success, hand off to the composable's connect() which
// will replay an authoritative `room:snapshot` over WS.
async function bootstrap() {
  loading.value = true
  try {
    const snap = await getRoom(roomId)
    lastAnimeId.value = snap.room.anime_id
    // Plan 05.5: persist for WS-only room:closed events that may arrive
    // after the composable resets its `room` ref. Privacy modes throw on
    // sessionStorage; silent failure is OK — the in-memory ref still
    // holds the value for the duration of THIS mount.
    try {
      sessionStorage.setItem(LAST_ANIME_ID_STORAGE_KEY, snap.room.anime_id)
    } catch {
      // ignore
    }
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
  // Tear down the test hook to avoid leaking handles across SPA
  // navigations within the same Playwright page object.
  if (typeof window !== 'undefined') {
    delete (window as unknown as { __wtTestRoom?: unknown }).__wtTestRoom
  }
})

// Workstream watch-together — install the test hook that the existing
// e2e/watch-together-state-switching.spec.ts already expects:
// `window.__wtTestRoom.emitChange{Episode,Player,Translation}`.
// Exposes the SAME roomHandle the in-page UI already uses, scoped to
// the user's current room (no privilege gain — anything callable via
// the hook is also callable via the visible PlayerTabBar / Chat /
// Reaction palette buttons). Kept unconditional so the Docker
// production-mode build that we use for headless e2e gets it. The
// risk surface is negligible: the room handle has no admin powers,
// only the WS emits this user could trigger anyway.
if (typeof window !== 'undefined') {
  (window as unknown as { __wtTestRoom?: unknown }).__wtTestRoom = roomHandle
}

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

// Plan 05.5 — invoked from the auth-expired modal's Login button. Writes
// returnUrl (consumed by the router's beforeEach guard) AND passes the
// explicit `next=` query so the auth view's own resume logic also sees
// it (belt-and-suspenders — mirrors the Phase 2 redirect pattern).
function onAuthExpiredLoginClick() {
  try {
    sessionStorage.setItem('returnUrl', route.fullPath)
  } catch {
    // Privacy modes can throw; silent failure is OK.
  }
  router.push({ path: '/auth', query: { next: route.fullPath } })
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

  <!-- Auth-expired — composable.onAuthExpired fired (Plan 05.5,
       WT-POLISH-06). Blocking modal; the user clicks Login to navigate
       to /auth with the resume URL preserved. -->
  <div
    v-else-if="errorState === 'auth-expired'"
    class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm p-4"
    role="dialog"
    aria-modal="true"
    :aria-labelledby="modalTitleId"
  >
    <div
      class="max-w-md w-full p-6 rounded-lg bg-background border border-foreground/20 flex flex-col gap-4 shadow-lg"
    >
      <h2
        :id="modalTitleId"
        class="text-xl font-semibold"
      >
        {{ t('watch_together.auth_expired_modal_title') }}
      </h2>
      <p class="text-foreground/80 font-medium">
        {{ t('watch_together.auth_expired_modal_body') }}
      </p>
      <button
        type="button"
        data-testid="wt-auth-expired-login"
        class="px-4 py-2 rounded-md bg-primary text-primary-foreground font-medium hover:bg-primary/90 transition-colors self-end"
        @click="onAuthExpiredLoginClick"
      >
        {{ t('watch_together.auth_expired_modal_login_button') }}
      </button>
    </div>
  </div>

  <!-- Live room layout — player column + sidebar column, burst overlay
       absolute over the player. -->
  <div
    v-else
    class="flex flex-col lg:flex-row min-h-screen w-full pt-16"
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
