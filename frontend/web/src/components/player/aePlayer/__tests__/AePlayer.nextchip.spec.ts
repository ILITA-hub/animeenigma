import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

// Feature (feedback 2026-07-08T15-21-12_tNeymik): a manual "Next episode" chip
// on the autoplay-OFF path, plus N / Shift+N hotkeys. The chip is styled like
// (and stacked above) the Skip-Ending chip and shows from the ending segment
// through the episode end. This harness mirrors AePlayer.skipchip.spec.ts.
const gogoCap = {
  provider: 'gogoanime', display_name: 'GogoAnime',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 90, group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub')[],
  variants: [],
}
const report: CapabilityReport = {
  anime_id: 'anime-uuid',
  families: [{ family: 'others', providers: [gogoCap] }],
}

vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({
    report: ref(report),
    capMap: ref(new Map<string, ProviderCap>([['gogoanime', gogoCap]])),
  }),
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: vi.fn().mockResolvedValue([
      { key: 1, label: 1, number: 1 },
      { key: 2, label: 2, number: 2 },
    ]),
    listTeams: vi.fn().mockResolvedValue([]),
    resolveStream: vi.fn().mockResolvedValue({ type: 'hls', url: '', servers: [] }),
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null), load: vi.fn().mockResolvedValue(undefined), destroy: vi.fn(),
    levels: ref([]), currentLevelLabel: ref('Auto'), setLevel: vi.fn(),
    fragStats: ref([]), bandwidthEstimate: ref(0), lastKnownPlayback: ref(null),
  }),
}))
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({ resolve: vi.fn().mockResolvedValue(undefined), resolvedCombo: ref(null) }),
}))
vi.mock('@/composables/aePlayer/useWatchTracking', () => ({
  useWatchTracking: () => ({
    maxTime: ref(0), episodeMarked: ref(false), marking: ref(false), onTick: vi.fn(),
    saveNow: vi.fn(), beaconSave: vi.fn(), markWatched: vi.fn().mockResolvedValue(undefined), resetEpisode: vi.fn(),
  }),
}))
vi.mock('@/composables/aePlayer/usePlaybackStats', () => ({ usePlaybackStats: () => ({ stats: ref(null), sample: vi.fn() }) }))
vi.mock('@/composables/useWatchedEpisodes', () => ({ useWatchedEpisodes: () => ({ watchedUpTo: ref(0), refresh: vi.fn().mockResolvedValue(undefined) }) }))
// A crowdsourced ENDING segment (op left null) so activeSkipSegment offers an
// outro whenever currentTime lands in [1322, 1411).
vi.mock('@/composables/useSkipTimes', () => ({
  useSkipTimes: () => ({
    opening: ref(null),
    ending: ref({ start: 1322, end: 1411 }),
    loading: ref(false), error: ref(null), refresh: vi.fn(),
  }),
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false, user: null }) }))
vi.mock('@/stores/viewerContext', () => ({ useViewerContextStore: () => ({ whenLoaded: vi.fn().mockResolvedValue(null) }) }))
vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn().mockResolvedValue({ data: { data: null } }) },
  aeApi: { getEpisodes: vi.fn().mockResolvedValue({ data: { data: { available: false, episodes: [] } } }) },
  scraperApi: {
    getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [] } } }),
    getServers: vi.fn().mockResolvedValue({ data: { data: { servers: [] } } }),
    getStream: vi.fn().mockResolvedValue({ data: { data: { stream: { sources: [] } } } }),
  },
}))
vi.mock('@/utils/playerTelemetry', () => ({ recordPlayerEvent: vi.fn() }))

import AePlayer from '../AePlayer.vue'

// NextEpisodeChip is deliberately NOT stubbed so its `v-if="visible"` reflects
// the real showNextEpChip → [data-test="next-episode-chip"] is the assertion
// surface. (autoNext defaults false, so the autoplay countdown card never shows.)
const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, SkipIntroChip: true, NextEpisodeCard: true, WatchTogetherButton: true,
  SubtitleOverlay: true, ResumePill: true,
}

// Track mounted wrappers so afterEach detaches them from document.body.
let mounted: ReturnType<typeof mount>[] = []

function mountPlayer(extraProps: Record<string, unknown> = {}) {
  const wrapper = mount(AePlayer, {
    // attachTo a real DOM node so focusing the player root makes it the
    // document.activeElement — playerIsActive() (the hotkey gate) requires it.
    attachTo: document.body,
    props: { animeId: 'anime-uuid', anime: { title: 'T', ep: 1, eps: 12 }, theater: false, ...extraProps },
    global: { mocks: { $t: (k: string) => k }, stubs },
  })
  mounted.push(wrapper)
  return wrapper
}

async function settle() {
  await flushPromises()
  await nextTick()
  await flushPromises()
}

const CHIP = '[data-test="next-episode-chip"]'

// Drive the element playhead into the ending window and sync currentTime.value
// via the pause path (onVideoPause → stopRaf → writeProgress), matching a viewer
// who reached the outro. duration is set below the ending end so the segment
// stays active (activeSkipSegment offers the outro).
async function seekAndPause(wrapper: ReturnType<typeof mountPlayer>, t: number) {
  const el = wrapper.find('video').element as HTMLVideoElement
  Object.defineProperty(el, 'currentTime', { value: t, writable: true, configurable: true })
  Object.defineProperty(el, 'duration', { value: 1420, writable: true, configurable: true })
  await wrapper.find('video').trigger('pause')
  await nextTick()
}

function pressKey(key: string, shiftKey = false) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, bubbles: true }))
}

// The current episode number, read from the test-only exposed ref. Vue may
// auto-unwrap an exposed ref, so accept both the ref and the bare value.
function currentEp(wrapper: ReturnType<typeof mountPlayer>): number | undefined {
  const raw = (wrapper.vm as unknown as { __selectedEpisode: unknown }).__selectedEpisode as
    | { value?: { number: number } | null; number?: number }
    | null
  if (!raw) return undefined
  const ep = 'value' in raw ? raw.value : raw
  return (ep as { number?: number } | null)?.number
}

// Pick the provider so the real load flow runs: it lists episodes (populating
// episodes.value = [ep1, ep2] — goToNextEpisode walks that list) and reconciles
// selectedEpisode to episode 1. The harness auto-picks no provider on its own.
async function readyPlayer(wrapper: ReturnType<typeof mountPlayer>) {
  const vm = wrapper.vm as unknown as { __setProvider: (id: string, server: string) => void }
  vm.__setProvider('gogoanime', '')
  await settle()
  // Focus the player SHELL (the inner rootRef element carrying tabindex/role,
  // not the outer .pl-wrap) so keyboard shortcuts are in scope (playerIsActive).
  const region = wrapper.get('[role="region"]').element as HTMLElement
  region.focus()
}

beforeEach(() => vi.clearAllMocks())
afterEach(() => {
  mounted.forEach((w) => w.unmount())
  mounted = []
})

describe('AePlayer — manual "Next episode" chip + hotkeys', () => {
  it('offers the chip in the ending window (autoplay off) and not on a fresh playhead', async () => {
    const wrapper = mountPlayer()
    await settle()
    await readyPlayer(wrapper)

    // Fresh at 0:00 — no outro, no end reached → chip hidden.
    expect(wrapper.find(CHIP).exists()).toBe(false)

    // Parked in the ending window → chip offered, stacked above the skip chip.
    await seekAndPause(wrapper, 1350)
    expect(wrapper.find(CHIP).exists()).toBe(true)
  })

  it('clicking the chip advances to the next episode', async () => {
    const wrapper = mountPlayer()
    await settle()
    await readyPlayer(wrapper)
    await seekAndPause(wrapper, 1350)
    expect(wrapper.find(CHIP).exists()).toBe(true)

    expect(currentEp(wrapper)).toBe(1)
    await wrapper.find(CHIP).trigger('click')
    await settle()

    expect(currentEp(wrapper)).toBe(2) // advanced to the next episode
  })

  it('Shift+N jumps to the next episode (always-on, whenever a next episode exists)', async () => {
    const wrapper = mountPlayer()
    await settle()
    await readyPlayer(wrapper)
    await seekAndPause(wrapper, 1350)
    expect(wrapper.find(CHIP).exists()).toBe(true)

    expect(currentEp(wrapper)).toBe(1)
    pressKey('N', true) // Shift+N — always advances when a next episode exists
    await settle()

    expect(currentEp(wrapper)).toBe(2) // advanced to the next episode
  })

  it('bare N advances while the chip is up (prompt-scoped)', async () => {
    const wrapper = mountPlayer()
    await settle()
    await readyPlayer(wrapper)
    await seekAndPause(wrapper, 1350)
    expect(wrapper.find(CHIP).exists()).toBe(true)

    expect(currentEp(wrapper)).toBe(1)
    pressKey('n') // prompt is visible → N acts
    await settle()

    expect(currentEp(wrapper)).toBe(2)
  })

  it('does not leak the end-of-episode chip across a manual episode switch', async () => {
    const wrapper = mountPlayer()
    await settle()
    await readyPlayer(wrapper)

    // Episode ends → the end flag opens the chip (autoplay off).
    await wrapper.find('video').trigger('ended')
    await nextTick()
    expect(wrapper.find(CHIP).exists()).toBe(true)

    // Manually pick a different episode. resetPlaybackClock (on the swap) must
    // clear reachedEpisodeEnd so the chip doesn't render over the incoming,
    // not-yet-playing episode.
    await (wrapper.vm as unknown as { onSelectEpisode: (e: { key: number; label: number; number: number }) => void })
      .onSelectEpisode({ key: 2, label: 2, number: 2 })
    await settle()

    expect(wrapper.find(CHIP).exists()).toBe(false)
  })

  it('bare N does nothing without a visible prompt (no outro, not ended)', async () => {
    const wrapper = mountPlayer()
    await settle()
    await readyPlayer(wrapper)
    // Fresh playhead at 0:00 — no chip, no card.
    expect(wrapper.find(CHIP).exists()).toBe(false)
    expect(currentEp(wrapper)).toBe(1)

    pressKey('n') // prompt-scoped — inert with nothing showing
    await settle()

    expect(currentEp(wrapper)).toBe(1) // still on the same episode
  })
})
