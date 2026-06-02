import { describe, it, expect } from 'vitest'
import { buildSelector, stripPII, extractClick, isTrackable } from '../autocapture'

describe('buildSelector', () => {
  it('uses id when present', () => {
    const el = document.createElement('button')
    el.id = 'buy'
    expect(buildSelector(el)).toContain('button#buy')
  })

  it('includes a couple of classes', () => {
    const el = document.createElement('a')
    el.className = 'cta primary'
    expect(buildSelector(el)).toContain('a.cta.primary')
  })
})

describe('stripPII', () => {
  it('redacts emails', () => {
    expect(stripPII('mail me at john@example.com now')).not.toContain('john@example.com')
  })
  it('redacts long digit runs (phones/cards)', () => {
    expect(stripPII('call 5551234567')).not.toContain('5551234567')
  })
  it('caps length to 200 chars', () => {
    expect(stripPII('x'.repeat(500)).length).toBeLessThanOrEqual(200)
  })
})

describe('isTrackable', () => {
  it('false when the element opts out via data-no-track', () => {
    const el = document.createElement('button')
    el.setAttribute('data-no-track', '')
    expect(isTrackable(el)).toBe(false)
  })
  it('false when an ancestor opts out', () => {
    const parent = document.createElement('div')
    parent.setAttribute('data-no-track', '')
    const child = document.createElement('button')
    parent.appendChild(child)
    expect(isTrackable(child)).toBe(false)
  })
  it('true for a normal element', () => {
    expect(isTrackable(document.createElement('button'))).toBe(true)
  })
})

describe('extractClick', () => {
  it('captures tag, selector, trimmed text, and data-* attrs', () => {
    const el = document.createElement('button')
    el.id = 'buy'
    el.textContent = '  Buy now  '
    el.setAttribute('data-plan', 'pro')
    el.setAttribute('aria-label', 'ignored')
    const c = extractClick(el)
    expect(c).not.toBeNull()
    expect(c!.el_tag).toBe('button')
    expect(c!.el_selector).toContain('button#buy')
    expect(c!.el_text).toBe('Buy now')
    expect(c!.el_attrs).toEqual({ 'data-plan': 'pro' })
  })

  it('returns null for opted-out elements', () => {
    const el = document.createElement('button')
    el.setAttribute('data-no-track', '')
    expect(extractClick(el)).toBeNull()
  })
})
