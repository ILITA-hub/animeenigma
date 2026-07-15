import { computed, watch, type Ref } from 'vue'
import { wtCreateSeed, type WtCreateSeed } from '@/composables/aePlayer/wtCreateSeed'
import { useWatchTogetherLaunch } from '@/composables/watch-together/useWatchTogetherLaunch'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { useAuthStore } from '@/stores/auth'

// ── Parent-facing sync: WT create seed + shareable-URL state + launch button ──

export interface WtSeedUrlSyncDeps {
  getRoom: () => WatchTogetherRoomHandle | null | undefined
  state: PlayerState
  selectedEpisode: Ref<EpisodeOption | null>
  providerAutoSelected: Ref<boolean>
  preferenceSettled: Ref<boolean>
  auth: ReturnType<typeof useAuthStore>
  getAnimeId: () => string
  emitComboChange: (seed: WtCreateSeed | null) => void
  emitUrlSync: (s: { provider: string; team: string; episode: number }) => void
}

export function useWtSeedUrlSync(deps: WtSeedUrlSyncDeps) {
  const { getRoom, state, selectedEpisode, providerAutoSelected, preferenceSettled, auth } = deps

  // ─── Watch-Together create seed ────────────────────────────────────────────
  // The live combo + current episode as a create-room seed (null until a usable
  // source resolves). Drives BOTH the in-player launch button (below) and the
  // anime-page Invite button (via the combo-change emit).
  const currentWtSeed = computed<WtCreateSeed | null>(() =>
    wtCreateSeed(state.combo.value, selectedEpisode.value?.number ?? 0),
  )

  // Outside a room, surface the seed to the parent (Anime.vue) so the Invite
  // button can CREATE the room AS aeplayer seeded with exactly this source.
  // Suppressed in a room — there the room is authoritative and the broadcast
  // watcher already syncs the combo. immediate so the parent has the
  // latest seed (or null) before the user can click Invite.
  watch(
    currentWtSeed,
    (seed) => {
      if (getRoom()) return
      deps.emitComboChange(seed)
    },
    { immediate: true },
  )

  // ─── Shareable-URL sync ────────────────────────────────────────────────────
  // Reflect the live source/team/episode so the parent view can mirror them into
  // `?provider/?team/?episode` (shareable + bookmarkable links). DONE RIGHT after
  // the 2026-06-21 revert: provider/team are emitted ONLY for a USER-PINNED source
  // (manual pick or `?provider=` deep-link → providerAutoSelected=false). For an
  // auto/smart-default selection they are emitted EMPTY, so the parent strips them
  // and a plain reload re-runs the deterministic BEST default (the product rule a
  // previously-watched source must not override). Suppressed inside a WT room.
  const urlSyncState = computed(() => {
    const ep = selectedEpisode.value?.number ?? 0
    const pinned = !providerAutoSelected.value && !!state.combo.value.provider
    return {
      provider: pinned ? state.combo.value.provider : '',
      team: pinned ? (state.combo.value.team ?? '') : '',
      episode: ep > 0 ? ep : 0,
    }
  })
  // Gate on preferenceSettled: applyInitialProvider() (which CONSUMES the
  // `?provider=` deep-link from props) runs in resolvePreference().finally() right
  // before preferenceSettled flips true. Emitting earlier would let the initial
  // empty/auto state strip the deep-link param from the URL before the read path
  // reads it. Once settled, every later source/team/episode change syncs through.
  watch(
    [preferenceSettled, urlSyncState],
    ([settled, s]) => {
      if (getRoom()) return
      if (!settled) return
      deps.emitUrlSync(s as { provider: string; team: string; episode: number })
    },
    { immediate: true },
  )

  // ─── In-player Watch-Together launch button ────────────────────────────────
  // Shown only standalone (not inside a room) and only to authenticated users —
  // creating a room requires a JWT, and inside a room the RoomSidebar owns
  // invites. Disabled until a usable source resolves; clicking creates the room
  // from the live combo and routes to it (same flow as the anime-page Invite).
  const { launching: wtLaunching, launch: launchWt } = useWatchTogetherLaunch()
  const showWtLaunch = computed(() => !getRoom() && auth.isAuthenticated)

  async function onLaunchWt(): Promise<void> {
    const seed = currentWtSeed.value
    if (!seed) return
    await launchWt({
      animeId: deps.getAnimeId(),
      episodeId: seed.episode_id,
      player: seed.player,
      translationId: seed.translation_id,
    })
  }

  return { currentWtSeed, wtLaunching, showWtLaunch, onLaunchWt }
}
