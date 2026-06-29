// frontend/web/src/composables/aePlayer/__tests__/useSubtitleAutoSync.spec.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleAutoSync, type SpeechTap } from '../useSubtitleAutoSync'

function fakeTap() {
  let cb: ((t: number, s: boolean) => void) | null = null
  return {
    onFrame: (fn: (t: number, s: boolean) => void) => { cb = fn },
    dispose() { (this as { disposed: boolean }).disposed = true },
    frame(t: number, s: boolean) { cb?.(t, s) },
    disposed: false,
  } as SpeechTap & { frame(t: number, s: boolean): void; disposed: boolean }
}
function speak(tap: ReturnType<typeof fakeTap>, a: number, b: number) {
  tap.frame(a, true); tap.frame((a + b) / 2, true); tap.frame(b, true); tap.frame(b + 0.05, false)
}
const cfg = { minSpeech: 3, confMin: 0.1, resyncDelta: 0.5, maxEvents: 10, seekGapSec: 1, windowSec: 120 }
const vel = () => ref(document.createElement('video'))

describe('useSubtitleAutoSync', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(1000) })

  it('locks a constant offset once enough confident speech accrues', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]) // 2s early vs speech
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    await nextTick()
    expect(s.status.value).toBe('locked')
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    expect(s.syncEvents.value[0]).toMatchObject({ reason: 'lock' })
    expect(s.syncEvents.value[0].delta).toBeCloseTo(2, 1)
  })

  it('does NOT lock below the speech minimum (no-op)', async () => {
    const tap = fakeTap()
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }]), enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 11)
    await nextTick()
    expect(s.status.value).toBe('listening')
    expect(s.autoOffset.value).toBe(0)
  })

  it('re-syncs on a mid-episode step (eyecatch) and logs reason=resync', async () => {
    const tap = fakeTap()
    const cues = ref([{ start: 8, end: 10 }, { start: 18, end: 21 }, { start: 40, end: 42 }, { start: 50, end: 53 }])
    const s = useSubtitleAutoSync({ videoElement: vel(), cues, enabled: ref(true), episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    speak(tap, 45, 47); speak(tap, 55, 58)            // cue 40 -> speech 45 ⇒ +5
    await nextTick()
    expect(s.autoOffset.value).toBeCloseTo(5, 1)
    expect(s.syncEvents.value[0].reason).toBe('resync')
  })

  it('resets state and disposes tap on episode change', async () => {
    const tap = fakeTap()
    const ek = ref('a:1')
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]), enabled: ref(true), episodeKey: ek, createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    expect(s.autoOffset.value).toBeCloseTo(2, 1)
    ek.value = 'a:2'; await nextTick()
    expect(s.autoOffset.value).toBe(0)
    expect(s.syncEvents.value).toEqual([])
    expect(tap.disposed).toBe(true)
  })

  it('disabling drops autoOffset to 0 and disposes', async () => {
    const tap = fakeTap()
    const enabled = ref(true)
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }, { start: 18, end: 21 }]), enabled, episodeKey: ref('a:1'), createTap: () => tap, config: cfg })
    speak(tap, 10, 12); speak(tap, 20, 23)
    enabled.value = false; await nextTick()
    expect(s.autoOffset.value).toBe(0)
    expect(s.status.value).toBe('idle')
    expect(tap.disposed).toBe(true)
  })

  it('marks unsupported if the tap factory throws', async () => {
    const s = useSubtitleAutoSync({ videoElement: vel(), cues: ref([{ start: 8, end: 10 }]), enabled: ref(true), episodeKey: ref('a:1'), createTap: () => { throw new Error('no audio') }, config: cfg })
    await nextTick()
    expect(s.status.value).toBe('unsupported')
    expect(s.autoOffset.value).toBe(0)
  })
})
