import { describe, it, expect } from 'vitest'
import { mapKeyToAction } from './playerHotkeys'

/** Build a minimal KeyboardEvent-like object for the mapper. */
function ev(key: string, target: Partial<HTMLElement> = {}): KeyboardEvent {
  return {
    key,
    target: { tagName: 'DIV', isContentEditable: false, ...target },
  } as unknown as KeyboardEvent
}

describe('mapKeyToAction', () => {
  it('maps space and k to play-pause', () => {
    expect(mapKeyToAction(ev(' '))).toEqual({ type: 'play-pause' })
    expect(mapKeyToAction(ev('k'))).toEqual({ type: 'play-pause' })
  })

  it('maps ArrowLeft/j to seek back 5s and ArrowRight/l to seek forward 5s', () => {
    expect(mapKeyToAction(ev('ArrowLeft'))).toEqual({ type: 'seek-rel', value: -5 })
    expect(mapKeyToAction(ev('j'))).toEqual({ type: 'seek-rel', value: -5 })
    expect(mapKeyToAction(ev('ArrowRight'))).toEqual({ type: 'seek-rel', value: 5 })
    expect(mapKeyToAction(ev('l'))).toEqual({ type: 'seek-rel', value: 5 })
  })

  it('maps ArrowUp/ArrowDown to volume nudge', () => {
    expect(mapKeyToAction(ev('ArrowUp'))).toEqual({ type: 'vol-rel', value: 5 })
    expect(mapKeyToAction(ev('ArrowDown'))).toEqual({ type: 'vol-rel', value: -5 })
  })

  it('maps m/f/c/p to mute/fullscreen/subs/pip', () => {
    expect(mapKeyToAction(ev('m'))).toEqual({ type: 'mute' })
    expect(mapKeyToAction(ev('f'))).toEqual({ type: 'fullscreen' })
    expect(mapKeyToAction(ev('c'))).toEqual({ type: 'subs' })
    expect(mapKeyToAction(ev('p'))).toEqual({ type: 'pip' })
  })

  it('maps z/x to subtitle timing offset (earlier/later by 0.1s)', () => {
    expect(mapKeyToAction(ev('z'))).toEqual({ type: 'sub-offset', value: -0.1 })
    expect(mapKeyToAction(ev('x'))).toEqual({ type: 'sub-offset', value: 0.1 })
    expect(mapKeyToAction(ev('Z'))).toEqual({ type: 'sub-offset', value: -0.1 })
    expect(mapKeyToAction(ev('X'))).toEqual({ type: 'sub-offset', value: 0.1 })
  })

  it('is case-insensitive for letter shortcuts', () => {
    expect(mapKeyToAction(ev('K'))).toEqual({ type: 'play-pause' })
    expect(mapKeyToAction(ev('M'))).toEqual({ type: 'mute' })
  })

  it('maps digit keys to seek-percent', () => {
    expect(mapKeyToAction(ev('0'))).toEqual({ type: 'seek-pct', value: 0 })
    expect(mapKeyToAction(ev('5'))).toEqual({ type: 'seek-pct', value: 50 })
    expect(mapKeyToAction(ev('9'))).toEqual({ type: 'seek-pct', value: 90 })
  })

  it('returns null for unhandled keys', () => {
    expect(mapKeyToAction(ev('q'))).toBeNull()
    expect(mapKeyToAction(ev('Tab'))).toBeNull()
  })

  it('ignores keys while typing in inputs / textareas / contentEditable', () => {
    expect(mapKeyToAction(ev(' ', { tagName: 'INPUT' }))).toBeNull()
    expect(mapKeyToAction(ev('k', { tagName: 'TEXTAREA' }))).toBeNull()
    expect(mapKeyToAction(ev('f', { tagName: 'DIV', isContentEditable: true }))).toBeNull()
  })
})
