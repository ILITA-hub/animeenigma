import { describe, expect, it } from 'vitest'
import { isHelpHotkey } from './globalHotkeys'

function keyEvent(
  key: string,
  opts: { ctrl?: boolean; meta?: boolean; alt?: boolean; shift?: boolean; target?: EventTarget } = {},
): KeyboardEvent {
  const e = new KeyboardEvent('keydown', {
    key,
    ctrlKey: opts.ctrl ?? false,
    metaKey: opts.meta ?? false,
    altKey: opts.alt ?? false,
    shiftKey: opts.shift ?? false,
  })
  if (opts.target) Object.defineProperty(e, 'target', { value: opts.target })
  return e
}

describe('isHelpHotkey', () => {
  it('matches a bare F1 press', () => {
    expect(isHelpHotkey(keyEvent('F1'))).toBe(true)
  })

  it('rejects other keys', () => {
    expect(isHelpHotkey(keyEvent('?'))).toBe(false)
    expect(isHelpHotkey(keyEvent('/'))).toBe(false)
    expect(isHelpHotkey(keyEvent('h'))).toBe(false)
    expect(isHelpHotkey(keyEvent('Escape'))).toBe(false)
  })

  it('rejects modified F1 chords — those are browser/OS commands', () => {
    expect(isHelpHotkey(keyEvent('F1', { ctrl: true }))).toBe(false)
    expect(isHelpHotkey(keyEvent('F1', { meta: true }))).toBe(false)
    expect(isHelpHotkey(keyEvent('F1', { alt: true }))).toBe(false)
    expect(isHelpHotkey(keyEvent('F1', { shift: true }))).toBe(false)
  })

  it('never fires while typing in a text field', () => {
    for (const tag of ['input', 'textarea', 'select']) {
      const el = document.createElement(tag)
      expect(isHelpHotkey(keyEvent('F1', { target: el }))).toBe(false)
    }
  })

  it('never fires inside contenteditable', () => {
    const el = document.createElement('div')
    Object.defineProperty(el, 'isContentEditable', { value: true })
    expect(isHelpHotkey(keyEvent('F1', { target: el }))).toBe(false)
  })

  it('fires on non-editable elements', () => {
    expect(isHelpHotkey(keyEvent('F1', { target: document.createElement('div') }))).toBe(true)
  })
})
