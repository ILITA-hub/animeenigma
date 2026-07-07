import { describe, it, expect } from 'vitest'
import { mapKeyToAction } from './playerHotkeys'

type Mods = { ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean; shiftKey?: boolean }

/** Build a minimal KeyboardEvent-like object for the mapper. */
function ev(key: string, target: Partial<HTMLElement> = {}, mods: Mods = {}): KeyboardEvent {
  return {
    key,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    ...mods,
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

  it('never hijacks Ctrl/Cmd+C — the browser copy must pass through', () => {
    expect(mapKeyToAction(ev('c', {}, { ctrlKey: true }))).toBeNull()
    expect(mapKeyToAction(ev('c', {}, { metaKey: true }))).toBeNull()
    expect(mapKeyToAction(ev('C', {}, { metaKey: true }))).toBeNull()
  })

  it('lets every other Ctrl/Cmd/Alt browser shortcut through, even ones that collide with a hotkey', () => {
    expect(mapKeyToAction(ev('f', {}, { ctrlKey: true }))).toBeNull() // find
    expect(mapKeyToAction(ev('p', {}, { metaKey: true }))).toBeNull() // print
    expect(mapKeyToAction(ev('x', {}, { ctrlKey: true }))).toBeNull() // cut
    expect(mapKeyToAction(ev('l', {}, { ctrlKey: true }))).toBeNull() // focus URL bar
    expect(mapKeyToAction(ev('5', {}, { ctrlKey: true }))).toBeNull() // no decile-seek on a chord
    expect(mapKeyToAction(ev('k', {}, { altKey: true }))).toBeNull()
  })

  it('still fires bare and Shift-modified hotkeys (Shift is just uppercase, not a browser command)', () => {
    expect(mapKeyToAction(ev('c'))).toEqual({ type: 'subs' })
    expect(mapKeyToAction(ev('C', {}, { shiftKey: true }))).toEqual({ type: 'subs' })
    expect(mapKeyToAction(ev('Z', {}, { shiftKey: true }))).toEqual({ type: 'sub-offset', value: -0.1 })
  })
})
