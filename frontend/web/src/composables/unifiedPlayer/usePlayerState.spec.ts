import { describe, it, expect, beforeEach } from 'vitest'
import { nextTick } from 'vue'
import { usePlayerState } from './usePlayerState'

describe('usePlayerState', () => {
  it('defaults: paused-progress 0, sub/en, autoplay+autoskip OFF', () => {
    const s = usePlayerState()
    expect(s.progress.value).toBe(0)
    expect(s.combo.value.audio).toBe('sub')
    expect(s.combo.value.lang).toBe('en')
    expect(s.autoNext.value).toBe(false)
    expect(s.autoSkip.value).toBe(false)
  })

  it('setProvider replaces provider+server and keeps audio/lang', () => {
    const s = usePlayerState()
    s.setProvider('kodik', 'Server 1')
    expect(s.combo.value.provider).toBe('kodik')
    expect(s.combo.value.server).toBe('Server 1')
    expect(s.combo.value.audio).toBe('sub')
  })

  it('setAudio resets team to null', () => {
    const s = usePlayerState()
    s.setTeam('AniLibria')
    s.setAudio('dub')
    expect(s.combo.value.team).toBeNull()
  })

  describe('hackerMode', () => {
    beforeEach(() => {
      localStorage.removeItem('pl_hacker_mode')
    })

    it('defaults to off', () => {
      const s = usePlayerState()
      expect(s.hackerMode.value).toBe(false)
    })

    it('persists to localStorage and restores', async () => {
      const s = usePlayerState()
      s.hackerMode.value = true
      await nextTick()
      expect(localStorage.getItem('pl_hacker_mode')).toBe('1')
      const s2 = usePlayerState()
      expect(s2.hackerMode.value).toBe(true)
    })

    it('clears the key when switched off', async () => {
      localStorage.setItem('pl_hacker_mode', '1')
      const s = usePlayerState()
      s.hackerMode.value = false
      await nextTick()
      expect(localStorage.getItem('pl_hacker_mode')).toBeNull()
    })
  })
})
