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
  it('matches a bare "?" press', () => {
    expect(isHelpHotkey(keyEvent('?'))).toBe(true)
  })

  it('matches "?" with shift held — the glyph is shifted on most layouts', () => {
    expect(isHelpHotkey(keyEvent('?', { shift: true }))).toBe(true)
  })

  it('rejects other keys', () => {
    expect(isHelpHotkey(keyEvent('/'))).toBe(false)
    expect(isHelpHotkey(keyEvent('h'))).toBe(false)
    expect(isHelpHotkey(keyEvent('Escape'))).toBe(false)
  })

  it('rejects ctrl/cmd/alt chords — those are browser/OS commands', () => {
    expect(isHelpHotkey(keyEvent('?', { ctrl: true }))).toBe(false)
    expect(isHelpHotkey(keyEvent('?', { meta: true }))).toBe(false)
    expect(isHelpHotkey(keyEvent('?', { alt: true }))).toBe(false)
  })

  it('never fires while typing in a text field', () => {
    for (const tag of ['input', 'textarea', 'select']) {
      const el = document.createElement(tag)
      expect(isHelpHotkey(keyEvent('?', { target: el }))).toBe(false)
    }
  })

  it('never fires inside contenteditable', () => {
    const el = document.createElement('div')
    Object.defineProperty(el, 'isContentEditable', { value: true })
    expect(isHelpHotkey(keyEvent('?', { target: el }))).toBe(false)
  })

  it('fires on non-editable elements', () => {
    expect(isHelpHotkey(keyEvent('?', { target: document.createElement('div') }))).toBe(true)
  })
})
