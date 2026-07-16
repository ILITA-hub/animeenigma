import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { computed, nextTick, ref, type ComputedRef, type Ref } from 'vue'
import { usePlayerState } from '@/composables/aePlayer/usePlayerState'
import { useComboBootstrap } from '@/composables/aePlayer/useComboBootstrap'
import type { ProviderRow } from '@/types/aePlayer'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { VerifyReport } from '@/types/contentVerify'

// The composable pulls in useWatchPreferences (Pinia + a network resolver) —
// mocked deterministically, same pattern as AePlayer.urlsync.spec.ts. The
// second element controls WHEN (or whether) `preferenceSettled` flips true.
const resolveMock = vi.fn().mockResolvedValue(undefined)
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({ resolve: (...args: unknown[]) => resolveMock(...args), resolvedCombo: ref(null) }),
}))

// A single group:'en' provider is enough for buildAvailable() to yield a
// non-empty combo list, so Watcher A's resolvePreference().finally() runs and
// preferenceSettled flips true.
const cap: ProviderCap = {
  provider: 'gogoanime', display_name: 'Gogoanime', state: 'active', selectable: true,
  hacker_only: false, order: 90, group: 'en', audios: ['sub', 'dub'], variants: [],
}
const REPORT: CapabilityReport = { anime_id: 'a1', families: [{ family: 'others', providers: [cap] }] }

function row(id: string, order: number): ProviderRow {
  return { id, label: id, group: 'en', state: 'active', selectable: true, hackerOnly: false, order, audios: ['sub', 'dub'] }
}

const verifiedReport: VerifyReport = {
  animeId: 'a1',
  providers: { providerB: { status: 'verified', raw: true, dub_langs: [], hardsub_langs: [] } },
}

interface Harness {
  rows: ComputedRef<ProviderRow[]>
  rowsRef: Ref<ProviderRow[]>
  verifyReport: Ref<VerifyReport | null>
  providerAutoSelected: Ref<boolean>
  recordDecision: ReturnType<typeof vi.fn>
  getHasStarted: ReturnType<typeof vi.fn>
  state: ReturnType<typeof usePlayerState>
  bootstrap: ReturnType<typeof useComboBootstrap>
}

function setup(opts: { hasStarted?: boolean; roomHasCombo?: boolean; neverSettle?: boolean } = {}): Harness {
  const state = usePlayerState()
  const rowsRef = ref<ProviderRow[]>([row('providerA', 50)])
  const rows = computed(() => rowsRef.value)
  const verifyReport = ref<VerifyReport | null>(null)
  const providerAutoSelected = ref(false)
  const recordDecision = vi.fn()
  const getHasStarted = vi.fn(() => opts.hasStarted ?? false)

  if (opts.neverSettle) resolveMock.mockImplementationOnce(() => new Promise(() => {}))

  const bootstrap = useComboBootstrap({
    state,
    rows,
    report: computed(() => REPORT),
    capMap: computed(() => new Map<string, ProviderCap>([['gogoanime', cap]])),
    roomHasCombo: computed(() => opts.roomHasCombo ?? false),
    providerAutoSelected,
    recordDecision,
    animeId: 'a1',
    getInitialProvider: () => undefined,
    getInitialTeam: () => undefined,
    getInitialAudio: () => undefined,
    getInitialLang: () => undefined,
    getInitialEpisode: () => undefined,
    isHentai: () => false,
    verifyReport,
    getHasStarted,
  })

  return { rows, rowsRef, verifyReport, providerAutoSelected, recordDecision, getHasStarted, state, bootstrap }
}

describe('useComboBootstrap — content-verify re-pick watcher', () => {
  beforeEach(() => {
    resolveMock.mockClear()
    resolveMock.mockResolvedValue(undefined)
  })

  it('re-picks the smart default when a verify report lands pre-playback over an auto-selected provider', async () => {
    const h = setup({ hasStarted: false })
    await flushPromises()
    await nextTick()

    // Watcher B already auto-selected the only candidate.
    expect(h.state.combo.value.provider).toBe('providerA')
    expect(h.providerAutoSelected.value).toBe(true)

    // Verdicts land: `rows` (Task 14) now also surfaces a higher-order source.
    h.rowsRef.value = [row('providerA', 50), row('providerB', 90)]
    h.verifyReport.value = verifiedReport
    await nextTick()

    expect(h.state.combo.value.provider).toBe('providerB')
    expect(h.providerAutoSelected.value).toBe(true)
    expect(h.recordDecision).toHaveBeenCalledWith('content-verify update — re-picked best source')
  })

  it('stays silent once playback has started, even with a better verified source available', async () => {
    const h = setup({ hasStarted: true })
    await flushPromises()
    await nextTick()
    expect(h.state.combo.value.provider).toBe('providerA')
    h.recordDecision.mockClear()

    h.rowsRef.value = [row('providerA', 50), row('providerB', 90)]
    h.verifyReport.value = verifiedReport
    await nextTick()

    expect(h.state.combo.value.provider).toBe('providerA')
    expect(h.recordDecision).not.toHaveBeenCalled()
  })

  it('never overrides a manual pin (providerAutoSelected=false)', async () => {
    const h = setup({ hasStarted: false })
    await flushPromises()
    await nextTick()
    expect(h.state.combo.value.provider).toBe('providerA')

    // Simulate a manual pick: user explicitly picked providerA (still the
    // current provider), which flips the auto-selected flag off.
    h.providerAutoSelected.value = false
    h.recordDecision.mockClear()

    h.rowsRef.value = [row('providerA', 50), row('providerB', 90)]
    h.verifyReport.value = verifiedReport
    await nextTick()

    expect(h.state.combo.value.provider).toBe('providerA')
    expect(h.recordDecision).not.toHaveBeenCalled()
  })

  it('stays silent inside a room with a pinned combo', async () => {
    const h = setup({ hasStarted: false, roomHasCombo: true })
    await flushPromises()
    await nextTick()
    // Watcher B is also gated by roomHasCombo — no auto-pick happened.
    expect(h.state.combo.value.provider).toBe('')

    h.rowsRef.value = [row('providerA', 50), row('providerB', 90)]
    h.verifyReport.value = verifiedReport
    await nextTick()

    expect(h.state.combo.value.provider).toBe('')
    expect(h.recordDecision).not.toHaveBeenCalled()
  })

  it('stays silent while the saved-preference resolution has not settled yet', async () => {
    const h = setup({ hasStarted: false, neverSettle: true })
    await flushPromises()
    await nextTick()
    expect(h.bootstrap.preferenceSettled.value).toBe(false)
    expect(h.state.combo.value.provider).toBe('')

    h.rowsRef.value = [row('providerA', 50), row('providerB', 90)]
    h.verifyReport.value = verifiedReport
    await nextTick()

    expect(h.state.combo.value.provider).toBe('')
    expect(h.recordDecision).not.toHaveBeenCalled()
  })
})
