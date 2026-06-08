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
    // visible own text wins over aria-label
    expect(c!.el_text).toBe('Buy now')
    expect(c!.el_attrs).toEqual({ 'data-plan': 'pro' })
  })

  it('returns null for opted-out elements', () => {
    const el = document.createElement('button')
    el.setAttribute('data-no-track', '')
    expect(extractClick(el)).toBeNull()
  })

  // --- Fix 3: climb to the nearest meaningful (interactive) ancestor ---
  it('climbs from an inner svg/icon to the enclosing button', () => {
    const btn = document.createElement('button')
    btn.className = 'arrow-next'
    btn.setAttribute('aria-label', 'Next episode')
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path')
    svg.appendChild(path)
    btn.appendChild(svg)
    // user actually clicked the <path> inside the icon
    const c = extractClick(path as unknown as Element)
    expect(c!.el_tag).toBe('button')
    expect(c!.el_selector).toContain('button.arrow-next')
    expect(c!.el_text).toBe('Next episode')
  })

  it('collapses a multi-span anchor (split logo) into one labelled target', () => {
    const a = document.createElement('a')
    a.className = 'brand-link'
    const wrap = document.createElement('span')
    const b1 = document.createElement('span')
    b1.textContent = 'Anime'
    const b2 = document.createElement('span')
    b2.textContent = 'Enigma'
    wrap.append(b1, b2)
    a.appendChild(wrap)
    // clicking either half resolves to the same anchor + label
    expect(extractClick(b1)!.el_tag).toBe('a')
    expect(extractClick(b2)!.el_text).toBe('AnimeEnigma')
    expect(extractClick(b1)!.el_selector).toContain('a.brand-link')
  })

  // --- Fix 1: attribute fallback chain for TEXTLESS elements ---
  it('falls back to aria-label when there is no visible text', () => {
    const btn = document.createElement('button')
    btn.setAttribute('aria-label', 'Play')
    expect(extractClick(btn)!.el_text).toBe('Play')
  })

  it('falls back to placeholder for an empty input', () => {
    const input = document.createElement('input')
    input.setAttribute('placeholder', 'Search anime…')
    expect(extractClick(input)!.el_text).toBe('Search anime…')
  })

  it('prefers an explicit data-track label over everything', () => {
    const btn = document.createElement('button')
    btn.textContent = 'Смотреть'
    btn.setAttribute('data-track', 'episode-play')
    expect(extractClick(btn)!.el_text).toBe('episode-play')
  })

  it('labels a textless non-interactive element by tag, not by name', () => {
    const video = document.createElement('video')
    expect(extractClick(video)!.el_text).toBe('⟨video⟩')
  })

  // --- Fix 2: never sweep deep subtree text (the page-text / JS-comment leak) ---
  it('does NOT capture deep subtree text when clicking a bare container', () => {
    const root = document.createElement('div')
    root.innerHTML =
      '<header>Обратная связь</header><script>// Disable native scroll restoration</script>' +
      '<main><p>' + 'word '.repeat(100) + '</p></main>'
    const c = extractClick(root)
    // a plain container with no own text → tag label, NOT the giant subtree blob
    expect(c!.el_text).toBe('⟨div⟩')
    expect(c!.el_text.length).toBeLessThan(20)
  })

  it('caps subtree-derived labels for interactive controls', () => {
    const a = document.createElement('a')
    const span = document.createElement('span')
    span.textContent = 'x'.repeat(300)
    a.appendChild(span)
    const c = extractClick(a)
    expect(c!.el_text.length).toBeLessThanOrEqual(80)
  })
})
