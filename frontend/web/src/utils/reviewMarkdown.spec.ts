import { describe, it, expect } from 'vitest'
import { parseReviewMarkdown } from './reviewMarkdown'

describe('parseReviewMarkdown', () => {
  it('plain text becomes one paragraph with one text token', () => {
    expect(parseReviewMarkdown('hello world')).toEqual([
      { type: 'p', lines: [[{ kind: 'text', text: 'hello world' }]] },
    ])
  })

  it('blank lines split paragraphs, single newlines split lines', () => {
    const blocks = parseReviewMarkdown('a\nb\n\nc')
    expect(blocks).toEqual([
      { type: 'p', lines: [[{ kind: 'text', text: 'a' }], [{ kind: 'text', text: 'b' }]] },
      { type: 'p', lines: [[{ kind: 'text', text: 'c' }]] },
    ])
  })

  it('parses bold, italic, strike, spoiler inline tokens', () => {
    const [p] = parseReviewMarkdown('x **b** *i* ~~s~~ ||sp|| y')
    expect(p).toEqual({
      type: 'p',
      lines: [[
        { kind: 'text', text: 'x ' },
        { kind: 'bold', text: 'b' },
        { kind: 'text', text: ' ' },
        { kind: 'italic', text: 'i' },
        { kind: 'text', text: ' ' },
        { kind: 'strike', text: 's' },
        { kind: 'text', text: ' ' },
        { kind: 'spoiler', text: 'sp' },
        { kind: 'text', text: ' y' },
      ]],
    })
  })

  it('consecutive "- " / "* " lines form a ul block', () => {
    expect(parseReviewMarkdown('- one\n- **two**\ntail')).toEqual([
      { type: 'ul', items: [[{ kind: 'text', text: 'one' }], [{ kind: 'bold', text: 'two' }]] },
      { type: 'p', lines: [[{ kind: 'text', text: 'tail' }]] },
    ])
  })

  it('unclosed markers stay literal text', () => {
    expect(parseReviewMarkdown('**open *and ~~more')).toEqual([
      { type: 'p', lines: [[{ kind: 'text', text: '**open *and ~~more' }]] },
    ])
  })

  it('html in input stays inert text (XSS)', () => {
    const [p] = parseReviewMarkdown('<img src=x onerror=alert(1)> **<b>bold</b>**')
    expect(p.type).toBe('p')
    const line = (p as { lines: unknown[][] }).lines[0] as { kind: string; text: string }[]
    expect(line[0]).toEqual({ kind: 'text', text: '<img src=x onerror=alert(1)> ' })
    expect(line[1]).toEqual({ kind: 'bold', text: '<b>bold</b>' })
  })

  it('empty / whitespace input yields no blocks', () => {
    expect(parseReviewMarkdown('')).toEqual([])
    expect(parseReviewMarkdown('  \n\n ')).toEqual([])
  })
})
