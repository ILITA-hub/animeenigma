import { ref, computed, watch, type Ref, type ComputedRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import type { WatchCombo } from '@/types/preference'
import type { PlayerKind } from '@/types/watch-together'
import type { WtCreateSeed } from '@/composables/aePlayer/wtCreateSeed'
import type { ViewerContextData } from '@/stores/viewerContext'
import { resolveInitialPlayerPref, CLASSIC_KODIK_KEY } from '@/views/animePlayerPrefs'
import { nextWatchQuery, watchQueryChanged, type WatchUrlState } from '@/views/watchUrlSync'

export interface PlayerSurfaceDeps {
  /** ?provider= deep-link (from useAnimeDeepLinks) — forces the aePlayer surface. */
  queryProvider: ComputedRef<string | undefined>
  /** Start-episode authority (from useAnimeWatchFlow) — token-less WT room fallback. */
  resumeStartEpisode: ComputedRef<number>
  /** Player-emitted max loaded episode (from useAnimeWatchFlow) — fed by available-translations. */
  resumeLoadedEpisodes: Ref<number>
}

/**
 * Page-level player-surface state for the anime page (extracted from
 * Anime.vue): the binary Classic Kodik fallback toggle, watch-preference
 * resolution for KodikPlayer's :preferred-combo, the Watch-Together invite
 * seed/payload, and the aePlayer two-way URL sync write path.
 */
export function usePlayerSurface(deps: PlayerSurfaceDeps) {
  const { queryProvider, resumeStartEpisode, resumeLoadedEpisodes } = deps
  const route = useRoute()
  const router = useRouter()

  // Plan B — AePlayer is the DEFAULT mounted player; "Classic Kodik" (the iframe
  // KodikPlayer) is the opt-in fallback. The legacy per-language tabs + provider
  // sub-tabs are retired (AePlayer's own SourcePanel serves every surviving
  // source: ae / kodik-HLS / EN scraper chain / raw / 18anime). One boolean now
  // chooses the surface, normalized from the old localStorage shape on mount.
  const _playerPref = resolveInitialPlayerPref({
    [CLASSIC_KODIK_KEY]: localStorage.getItem(CLASSIC_KODIK_KEY),
    unified_player_selected: localStorage.getItem('unified_player_selected'),
    preferred_video_provider: localStorage.getItem('preferred_video_provider'),
  })
  const classicKodik = ref(_playerPref.classicKodik)
  watch(classicKodik, (on) => {
    localStorage.setItem(CLASSIC_KODIK_KEY, on ? 'true' : 'false')
  })

  // A `?provider=` deep-link always opens aePlayer (the param speaks aePlayer's
  // source vocabulary), so make sure we're not on the Classic Kodik fallback.
  if (queryProvider.value) classicKodik.value = false

  // Watch preference resolution
  const preferenceState = ref<{
    resolvedCombo: WatchCombo | null
    resolve: (available: WatchCombo[]) => Promise<void>
  } | null>(null)

  const resolvedCombo = computed(() => preferenceState.value?.resolvedCombo ?? null)

  // Watch-Together create seed surfaced by AePlayer (live combo + episode). When
  // the aePlayer surface is the active one at room-create time, this drives the
  // Invite button to create the room AS aeplayer seeded with that exact combo.
  // Null until AePlayer has a resolved source (or when aePlayer isn't selected).
  const aeWtSeed = ref<WtCreateSeed | null>(null)

  // Two-way URL sync — DONE RIGHT (reintroduced after the 2026-06-21 revert).
  // AePlayer emits `url-sync` with the live source/team/episode; provider/team are
  // EMPTY for an auto/smart-default selection and populated only for a user-pinned
  // source (manual pick or `?provider=` deep-link). We mirror exactly that into
  // `?provider/?team/?episode` so the address bar is shareable + bookmarkable —
  // WITHOUT the old bug: because the smart default emits no provider, a plain
  // reload writes no `?provider` and re-runs the deterministic BEST default (the
  // product rule a previously-watched source must not override). The read path
  // (queryProvider/queryTeam/queryEpisode → :initial-* props) is one-shot at
  // mount, so writing here never re-triggers it. router.replace (not push) keeps
  // history clean; we only write on a real diff. AePlayer suppresses the emit
  // inside WT rooms, and Anime.vue is never a room — so no churn there.
  function onUrlSync(s: WatchUrlState) {
    const next = nextWatchQuery(route.query, s)
    if (watchQueryChanged(route.query, next)) {
      void router.replace({ query: next }).catch(() => {})
    }
  }

  // The Invite button's create-room payload. WatchTogether is aePlayer-only
  // (Kodik retired from WT 2026-06-19 — aePlayer plays Kodik content internally).
  // When the player has resolved a combo, seed the room with it; otherwise create
  // a token-less aeplayer room (the room's player resolves a BEST source in-room
  // and broadcasts it to joiners), so the invite works before the player resolves.
  const wtInvitePayload = computed<{ player: PlayerKind; translationId: string; episodeId: string }>(() => {
    if (aeWtSeed.value) {
      return {
        player: aeWtSeed.value.player,
        translationId: aeWtSeed.value.translation_id,
        episodeId: aeWtSeed.value.episode_id,
      }
    }
    return {
      player: 'aeplayer' as PlayerKind,
      translationId: '',
      episodeId: String(resumeStartEpisode.value),
    }
  })

  // Plan B — the per-provider override tracker (useOverrideTracker) is retired
  // along with the provider sub-tabs: there is no longer a user-facing player
  // switch on this page (AePlayer owns source selection inside its own
  // SourcePanel; the only page-level toggle is the binary Classic Kodik
  // fallback). The resolved coarse combo is still resolved here purely to feed
  // KodikPlayer's :preferred-combo (the WT invite payload uses the aePlayer seed).

  const initPreferences = (animeId: string, tier1Combo?: ViewerContextData['combo']) => {
    const pref = useWatchPreferences(animeId, { tier1Combo })
    preferenceState.value = {
      resolvedCombo: pref.resolvedCombo.value,
      resolve: async (available: WatchCombo[]) => {
        await pref.resolve(available)
        if (preferenceState.value) {
          preferenceState.value.resolvedCombo = pref.resolvedCombo.value
        }
      }
    }
  }

  const handleAvailableTranslations = (combos: WatchCombo[]) => {
    if (preferenceState.value) {
      preferenceState.value.resolve(combos)
    }
    // Feed the real provider episode count into the resume state machine so a
    // freshly-aired-but-already-uploaded episode is classified 'watching', not a
    // false "not loaded yet" (Shikimori episodesAired lags reality by hours).
    // Take the max across teams — if ANY team has ep N, it's watchable.
    const maxLoaded = combos.reduce((m, c) => Math.max(m, c.episodes_count ?? 0), 0)
    if (maxLoaded > resumeLoadedEpisodes.value) resumeLoadedEpisodes.value = maxLoaded
  }

  return {
    classicKodik,
    resolvedCombo,
    aeWtSeed,
    wtInvitePayload,
    onUrlSync,
    initPreferences,
    handleAvailableTranslations,
  }
}
