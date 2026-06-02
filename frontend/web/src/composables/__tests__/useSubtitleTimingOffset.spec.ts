import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('useSubtitleTimingOffset', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules() // fresh module-singleton per test so load() re-runs
  })

  it('defaults to 0 when nothing is stored', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(0)
  })

  it('loads an existing stored value on init', async () => {
    localStorage.setItem('aenigma_subtitle_timing_offset', '2.5')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(2.5)
  })

  it('clamps an out-of-range stored value to MAX', async () => {
    localStorage.setItem('aenigma_subtitle_timing_offset', '999')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(30)
  })

  it('clamps an out-of-range negative stored value to MIN', async () => {
    localStorage.setItem('aenigma_subtitle_timing_offset', '-999')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(-30)
  })

  it('falls back to 0 on a corrupt stored value', async () => {
    localStorage.setItem('aenigma_subtitle_timing_offset', 'not-a-number')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(0)
  })

  it('nudge accumulates and rounds to 1 decimal', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    const { offset, nudge } = useSubtitleTimingOffset()
    nudge(0.1); nudge(0.1); nudge(0.1)
    expect(offset.value).toBe(0.3) // not 0.30000000000000004
  })

  it('clamps nudge to MAX', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    const { offset, nudge } = useSubtitleTimingOffset()
    nudge(100)
    expect(offset.value).toBe(30)
  })

  it('reset returns to 0', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    const { offset, nudge, reset } = useSubtitleTimingOffset()
    nudge(1)
    reset()
    expect(offset.value).toBe(0)
  })

  it('writes through to localStorage synchronously on change', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    useSubtitleTimingOffset().nudge(1)
    expect(localStorage.getItem('aenigma_subtitle_timing_offset')).toBe('1')
  })
})
