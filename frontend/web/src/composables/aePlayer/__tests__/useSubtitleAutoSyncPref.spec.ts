import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleAutoSyncPref, DAY_MS } from '../useSubtitleAutoSyncPref'

const KEY = (k: string) => `aenigma_subautosync_${k}`

describe('useSubtitleAutoSyncPref', () => {
  beforeEach(() => { localStorage.clear(); vi.useFakeTimers(); vi.setSystemTime(1_000_000) })
  afterEach(() => { vi.useRealTimers() })

  it('defaults to true when nothing stored', () => {
    expect(useSubtitleAutoSyncPref(ref('a:1')).enabled.value).toBe(true)
  })
  it('persists setEnabled(false) with a 24h expiry', () => {
    const { enabled, setEnabled } = useSubtitleAutoSyncPref(ref('a:1'))
    setEnabled(false)
    expect(enabled.value).toBe(false)
    const raw = JSON.parse(localStorage.getItem(KEY('a:1'))!)
    expect(raw.value).toBe(false); expect(raw.expiresAt).toBe(1_000_000 + DAY_MS)
  })
  it('reverts to default true once the stored value expires', () => {
    localStorage.setItem(KEY('a:1'), JSON.stringify({ value: false, expiresAt: 1_000_000 - 1 }))
    expect(useSubtitleAutoSyncPref(ref('a:1')).enabled.value).toBe(true)
  })
  it('is isolated per episode: disabling a:1 leaves a:2 default-on', () => {
    useSubtitleAutoSyncPref(ref('a:1')).setEnabled(false)
    expect(useSubtitleAutoSyncPref(ref('a:2')).enabled.value).toBe(true)
  })
  it('re-reads when episodeKey changes', async () => {
    localStorage.setItem(KEY('a:2'), JSON.stringify({ value: false, expiresAt: 1_000_000 + DAY_MS }))
    const ek = ref('a:1'); const { enabled } = useSubtitleAutoSyncPref(ek)
    expect(enabled.value).toBe(true)
    ek.value = 'a:2'; await nextTick()
    expect(enabled.value).toBe(false)
  })
  it('falls back to true if storage throws', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked') })
    expect(useSubtitleAutoSyncPref(ref('a:1')).enabled.value).toBe(true)
    spy.mockRestore()
  })
})
