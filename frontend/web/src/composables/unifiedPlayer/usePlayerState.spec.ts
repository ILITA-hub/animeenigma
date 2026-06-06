import { describe, it, expect } from 'vitest'
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
})
