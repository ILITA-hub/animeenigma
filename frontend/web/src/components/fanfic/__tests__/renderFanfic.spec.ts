import { describe, it, expect } from 'vitest'
import { renderFanfic } from '../renderFanfic'

describe('renderFanfic', () => {
  it('splits prose into heading and paragraph blocks (no raw HTML)', () => {
    const blocks = renderFanfic('# Title\n\nFirst para.\n\nSecond para.')
    expect(blocks).toEqual([
      { type: 'h2', text: 'Title' },
      { type: 'p', text: 'First para.' },
      { type: 'p', text: 'Second para.' },
    ])
  })

  it('does not emit raw HTML for injected tags (XSS-safe text nodes)', () => {
    const blocks = renderFanfic('<script>alert(1)</script>')
    expect(blocks[0]).toEqual({ type: 'p', text: '<script>alert(1)</script>' })
  })

  it('renders an h3 block for a level-2 heading', () => {
    const blocks = renderFanfic('## Chapter One\n\nBody text.')
    expect(blocks).toEqual([
      { type: 'h3', text: 'Chapter One' },
      { type: 'p', text: 'Body text.' },
    ])
  })

  it('collapses single newlines inside a paragraph into spaces', () => {
    const blocks = renderFanfic('Line one\nLine two')
    expect(blocks).toEqual([{ type: 'p', text: 'Line one Line two' }])
  })

  it('ignores blank/whitespace-only chunks and empty input', () => {
    expect(renderFanfic('')).toEqual([])
    expect(renderFanfic('\n\n\n')).toEqual([])
  })
})
