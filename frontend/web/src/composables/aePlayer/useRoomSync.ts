import { computed, nextTick, watch, type Ref } from 'vue'
import { tokenToCombo, comboToToken } from '@/composables/aePlayer/comboMapping'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// ── Watch-Together (room sync) ────────────────────────────────────────────────
// When mounted inside a WT room the room's combo is authoritative: this
// composable adopts the room's combo/episode on join and on every remote
// change, and broadcasts genuine LOCAL changes back — with echo guards in both
// directions. When the room is null/undefined none of the watchers are
// registered and the player behaves exactly as standalone.

export interface RoomSyncDeps {
  getRoom: () => WatchTogetherRoomHandle | null | undefined
  state: PlayerState
  episodes: Ref<EpisodeOption[]>
  selectedEpisode: Ref<EpisodeOption | null>
  /** Late-bound: the stream-resolution cluster's episode-select entry point. */
  onSelectEpisode: (ep: EpisodeOption) => void
  recordDecision: (reason: string) => void
}

export function useRoomSync(deps: RoomSyncDeps) {
  const { getRoom, state, episodes, selectedEpisode } = deps

  // True iff mounted inside a WT room — the room's combo is authoritative, so
  // AePlayer's own auto-source-selection (Stage 1a smart default + Stage 1b
  // saved-combo restore) is suppressed.
  const roomPinned = computed(() => !!getRoom())

  // Whether the room currently pins a usable aePlayer combo. True only when the
  // room's translation_id parses to a real combo token. When false (a token-less
  // room, or an in-flight LEGACY room whose translation_id is a kodik id, not an
  // aePlayer combo) there is nothing to adopt, so we let the normal smart-default
  // resolution run — the player picks the BEST source and the broadcast watcher
  // publishes it to the room. Once a combo lands this flips true and pins.
  const roomHasCombo = computed(
    () => roomPinned.value && !!tokenToCombo(getRoom()?.room.value?.translation_id ?? ''),
  )

  // Guard so the combo-broadcast watcher does not echo a combo we applied FROM
  // the room back TO the room. Set true around every applyRoomCombo, cleared on
  // the next tick once the deep combo watcher has observed the change.
  let applyingRoomCombo = false

  // Guard so a room-driven episode switch does not re-emit the episode change to
  // the room (which would loop forever). Mirrors OurEnglishPlayer's fromRoomSync.
  let applyingRoomEpisode = false

  // Parse a WT room token (carried in `translation_id`) and apply it wholesale to
  // the player's combo. The room is the single source of truth for the source, so
  // every field (audio/lang/team/provider/server) is set from the token.
  function applyRoomCombo(token: string | undefined | null) {
    if (!token) return
    const fields = tokenToCombo(token)
    if (!fields) return
    applyingRoomCombo = true
    state.combo.value = {
      audio: fields.audio,
      lang: fields.lang,
      team: fields.team,
      provider: fields.provider,
      server: fields.server,
    }
    deps.recordDecision('pinned by the Watch-Together room')
    // Release after Vue flushes the deep combo watcher for this change so the
    // broadcast watcher sees applyingRoomCombo===true and skips the echo.
    void nextTick(() => {
      applyingRoomCombo = false
    })
  }

  // Apply the room's combo on mount and on every remote source switch. immediate
  // so a joiner adopts the room's source before its own (non-immediate) broadcast
  // watcher can fire — guaranteeing no echo of the room's own combo on join.
  if (getRoom()) {
    watch(
      () => getRoom()?.room.value?.translation_id,
      (tid) => applyRoomCombo(tid),
      { immediate: true },
    )

    // Broadcast a genuine LOCAL source change to the room. NOT immediate, so the
    // initial (room-applied) combo is the watcher's baseline and never echoes.
    // Skipped while applyingRoomCombo (a remote apply) or when no real provider
    // is picked yet (provider==='' is the un-resolved initial state).
    watch(
      () => state.combo.value,
      (combo) => {
        const room = getRoom()
        if (!room || applyingRoomCombo || !combo.provider) return
        room.emitChangeTranslation(
          comboToToken({
            provider: combo.provider,
            audio: combo.audio,
            lang: combo.lang,
            team: combo.team,
            server: combo.server,
          }),
        )
      },
      { deep: true },
    )

    // Adopt a remote episode change. The room's episode_id carries the episode
    // NUMBER as a string for aeplayer rooms. Switch the local episode under the
    // applyingRoomEpisode guard so onSelectEpisode does not re-emit it.
    watch(
      () => getRoom()?.room.value?.episode_id,
      (epId) => {
        if (!epId) return
        const num = Number(epId)
        if (!Number.isFinite(num)) return
        if (selectedEpisode.value?.number === num) return
        const ep =
          episodes.value.find((e) => e.number === num) ??
          ({ key: num, label: num, number: num } as EpisodeOption)
        applyingRoomEpisode = true
        try {
          deps.onSelectEpisode(ep)
        } finally {
          applyingRoomEpisode = false
        }
      },
    )
  }

  // WT: broadcast a genuine local episode pick to the room. Skipped when this
  // switch was itself driven by a remote room change (applyingRoomEpisode) so
  // the echo doesn't loop. The room's episode_id carries the episode number.
  function broadcastEpisodeChange(ep: EpisodeOption) {
    const room = getRoom()
    if (room && !applyingRoomEpisode) {
      room.emitChangeEpisode(String(ep.number))
    }
  }

  return { roomPinned, roomHasCombo, broadcastEpisodeChange }
}
