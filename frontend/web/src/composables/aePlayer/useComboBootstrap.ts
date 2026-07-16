import { ref, watch, type ComputedRef, type Ref } from 'vue'
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import { resolveDeepLinkProvider } from '@/composables/aePlayer/deepLinkProvider'
import { pickSmartDefault, pickRawBiased, pickSelectableFallback, defaultPool } from '@/composables/aePlayer/smartDefault'
import { langsForCap, langForProviderUnderRaw } from '@/composables/aePlayer/providerGroups'
import { watchComboToPartialCombo, providerToLegacyPlayer, clampLangForAudio } from '@/composables/aePlayer/comboMapping'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { ProviderRow, TrackLang } from '@/types/aePlayer'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { WatchCombo } from '@/types/preference'
import type { VerifyReport } from '@/types/contentVerify'

// ── Initial combo resolution: saved prefs, URL facets, deep-link, smart default ─
// On open, the player resolves a combo in this order (highest wins):
//   URL facet/provider  >  saved watch preference  >  smart default
// Stage 1b (saved combo) resolves first; the Stage 1a smart default is gated on
// `preferenceSettled` so the saved pick always wins when present.

export interface ComboBootstrapDeps {
  state: PlayerState
  rows: ComputedRef<ProviderRow[]>
  report: ComputedRef<CapabilityReport | null>
  capMap: ComputedRef<Map<string, ProviderCap>>
  roomHasCombo: ComputedRef<boolean>
  providerAutoSelected: Ref<boolean>
  recordDecision: (reason: string) => void
  /** Captured once at setup — matches the original useWatchPreferences(props.animeId). */
  animeId: string
  getInitialProvider: () => string | undefined
  getInitialTeam: () => string | undefined
  getInitialAudio: () => string | undefined
  getInitialLang: () => string | undefined
  getInitialEpisode: () => number | undefined
  isHentai: () => boolean
  /** Content-verify probe report (Task 13/14) — `rows` already reflects verdicts
   *  reactively, but the smart default only ever runs ONCE at bootstrap. When
   *  verdicts land later (poll tick, while the user is still on the pre-play
   *  screen) this re-triggers `pickFacetDefault()` so a source proven better
   *  (or worse) by content-verify can still win the auto-pick. */
  verifyReport: Ref<VerifyReport | null>
  /** Playback-started gate — content-verify may correct the pre-playback pick,
   *  but NEVER once the user has actually started watching (see the watcher
   *  below). */
  getHasStarted: () => boolean
}

export function useComboBootstrap(deps: ComboBootstrapDeps) {
  const { state, rows, report, capMap, roomHasCombo, providerAutoSelected } = deps

  const preferenceSettled = ref(false)
  const { resolve: resolvePreference, resolvedCombo } = useWatchPreferences(deps.animeId)

  function applyResolvedCombo() {
    const rc = resolvedCombo.value
    if (!rc || state.combo.value.provider) return
    // Restore the user's saved audio/lang/team PREFERENCES only — NOT the source.
    // Per product rule, the selected source (BEST) is a deterministic function of
    // first-party availability + third-party stats, so the smart default (below)
    // always picks it; a previously-watched source must not override it.
    const { audio, lang, team } = watchComboToPartialCombo(rc)
    // setAudio/setLang each reset team → null, so setTeam must come AFTER them.
    state.setAudio(audio)
    state.setLang(clampLangForAudio(audio, lang)) // no Japanese dub → dub/ja becomes dub/en
    if (team) state.setTeam(team)
  }

  // Notification / shared-link `?provider=` override: pin that source BEFORE the
  // smart default runs. Honored for any real, content-compatible, non-static-
  // disabled provider def (coarse/legacy/unavailable values fall through to the
  // smart default). Runs after applyResolvedCombo so initialTeam wins over the
  // saved-pref team, and after setAudio/setLang (which reset team → null) so the
  // team sticks.
  //
  // CRUCIAL: clamp audio/lang to what the provider serves. A row is only `active`
  // (and thus pinnable) when it matches the live audio/lang/content filter, and
  // the default combo is sub/en — so a ?provider=kodik (RU) pin would otherwise
  // land an `irrelevant` row and silently fall through to BEST. Switching the
  // facet first makes the deep-linked row relevant so the explicit choice holds.
  function applyInitialProvider() {
    if (state.combo.value.provider) return
    const pin = resolveDeepLinkProvider(
      deps.getInitialProvider(),
      state.combo.value,
      deps.isHentai() ? 'hentai' : 'common',
      capMap.value,
    )
    if (!pin) return
    providerAutoSelected.value = false // user-intent pin, not an auto-selection
    // setAudio/setLang each reset team → null, so they must come BEFORE setProvider
    // + setTeam (which is why initialTeam is applied last).
    state.setAudio(pin.audio)
    state.setLang(pin.lang)
    state.setProvider(pin.provider, '')
    const initialTeam = deps.getInitialTeam()
    if (initialTeam) state.setTeam(initialTeam)
    deps.recordDecision('deep-link — pinned from the ?provider/?team URL')
  }

  // URL facet override (?audio=raw|sub|dub, ?lang=en|ru|ja) — applied after the
  // saved combo, before the ?provider clamp. Precedence: URL > saved > smart default.
  function applyUrlFacet() {
    if (state.combo.value.provider) return
    const a = deps.getInitialAudio()
    if (a === 'dub') state.setAudio('dub')
    else if (a === 'raw' || a === 'sub') state.setAudio('sub')
    const l = deps.getInitialLang()
    if (l === 'en' || l === 'ru' || l === 'ja') state.setLang(clampLangForAudio(state.combo.value.audio, l))
  }

  // Enumerate EVERY real source's facet (across all families, NOT just the rows
  // matching the current audio/lang filter) so a saved/URL combo in any
  // language/audio can be matched. The old version iterated the facet-filtered
  // `rows` — which at mount only carry the default RAW/EN options — so every saved
  // preference (ru/dub/ja) failed to match and the player collapsed to SUB EN.
  const buildAvailable = (): WatchCombo[] => {
    const combos: WatchCombo[] = []
    const seen = new Set<string>()
    const rep = report.value
    if (!rep || !Array.isArray(rep.families)) return combos
    for (const fam of rep.families) {
      for (const cap of fam.providers ?? []) {
        if (cap.state === 'no_content') continue
        const player = providerToLegacyPlayer(cap.provider, cap.group, cap.player_key)
        if (!player) continue
        // A cap's real per-title `lang` (Phase C source-panel truth — set only
        // for ae's probed dub) overrides the group's default language set, so
        // e.g. an ae English dub routes under EN only, not every language
        // `firstparty` nominally serves. Caps without `lang` are unchanged.
        const langs = langsForCap(cap)
        const audios = [...new Set((cap.audios ?? []).map((a) => (a === 'dub' ? 'dub' : 'sub')))]
        for (const audio of audios) {
          for (const language of langs) {
            const key = `${player}:${audio}:${language}`
            if (seen.has(key)) continue
            seen.add(key)
            combos.push({
              player,
              language: language as WatchCombo['language'],
              watch_type: audio as WatchCombo['watch_type'],
              translation_id: '',
              translation_title: '',
            })
          }
        }
      }
    }
    return combos
  }

  // one-shot latch (non-reactive on purpose — read/written only inside the watcher)
  let resolveAttempted = false
  watch(rows, () => {
    // WT: when the room pins a usable combo it is authoritative — never run the
    // saved-combo restore. A token-less / legacy room has nothing to pin, so we
    // fall through and resolve normally (BEST source + saved audio/lang).
    if (roomHasCombo.value) return
    if (resolveAttempted) return
    const available = buildAvailable()
    if (available.length === 0) return
    resolveAttempted = true
    resolvePreference(available).finally(() => {
      applyResolvedCombo()
      applyUrlFacet()
      applyInitialProvider()
      preferenceSettled.value = true
    })
  }, { immediate: true })

  watch(
    [rows, preferenceSettled],
    () => {
      // WT: a room that pins a usable combo suppresses the smart default. A
      // token-less / legacy room resolves BEST and broadcasts it (see roomHasCombo).
      if (roomHasCombo.value) return
      if (state.combo.value.provider) return
      if (!preferenceSettled.value) return // let saved prefs (audio/lang) settle first
      const pick = pickFacetDefault()
      if (pick && !state.combo.value.provider) {
        providerAutoSelected.value = true
        state.setProvider(pick.id, '')
        deps.recordDecision('smart default — best available source')
      }
    },
    { immediate: true },
  )

  // Content-verify verdicts landed while the user is still reading the
  // description (spec §5): silently re-run the smart default. NEVER after
  // the first frame, NEVER over a manual pick.
  watch(deps.verifyReport, (rep) => {
    if (!rep) return
    if (deps.getHasStarted()) return
    if (roomHasCombo.value) return
    if (!preferenceSettled.value) return
    if (state.combo.value.provider && !providerAutoSelected.value) return
    const pick = pickFacetDefault()
    if (pick && pick.id !== state.combo.value.provider) {
      providerAutoSelected.value = true
      state.setProvider(pick.id, '')
      deps.recordDecision('content-verify update — re-picked best source')
    }
  })

  // Best provider for the current facet: language-biased under RAW (don't cross
  // language when a same-language source exists), plain best under DUB, with a
  // dead-player fallback to the top-ranked SELECTABLE (degraded) row so a
  // fully-degraded fleet still attempts playback instead of dead-ending.
  //
  // When no specific episode was requested (a fresh / first-time open, initial
  // episode ≤ 1), ae's partial first-party library must NOT win the default — it
  // may hold only a late auto-cached episode and would open that instead of
  // episode 1. `defaultPool` drops firstparty in that case (unless ae is the only
  // playable source). A resume / deep-link to a real episode (> 1) keeps ae
  // eligible; ae is always still MANUALLY selectable in the Source panel.
  function pickFacetDefault(): ProviderRow | null {
    const episodeSpecified = (deps.getInitialEpisode() ?? 1) > 1
    const pool = defaultPool(rows.value, episodeSpecified)
    const primary =
      state.combo.value.audio === 'sub'
        ? pickRawBiased(pool, state.combo.value.lang)
        : pickSmartDefault(pool)
    return primary ?? pickSelectableFallback(rows.value)
  }

  // Under RAW (audio:'sub') the language slider is hidden — derive combo.lang from
  // the chosen provider's group so persistence + the subtitle menu stay correct.
  // setServedLang preserves team; the facet watcher ignores RAW lang changes, so
  // this never churns the source.
  watch(
    () => state.combo.value.provider,
    (id) => {
      if (!id || state.combo.value.audio !== 'sub') return
      const row = rows.value.find((r) => r.id === id)
      if (!row) return
      const want: TrackLang = langForProviderUnderRaw(row.group, state.combo.value.lang)
      if (want !== state.combo.value.lang) state.setServedLang(want)
    },
  )

  return { preferenceSettled, pickFacetDefault }
}
