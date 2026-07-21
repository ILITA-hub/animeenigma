/**
 * Safe review mini-markdown parser (feedback 2026-07-21, @realMiZZeR).
 *
 * Same safety contract as renderFanfic.ts: dependency-free, returns typed
 * tokens that ReviewMarkdown.vue renders as TEXT nodes ({{ t.text }}, never
 * v-html) — that projection is the entire XSS defense. Flat subset, no
 * nesting: **bold**, *italic*, ~~strike~~, ||spoiler||, "- " bullet lists,
 * blank-line paragraphs with single-newline line breaks.
 */

export type InlineToken = {
  kind: 'text' | 'bold' | 'italic' | 'strike' | 'spoiler'
  text: string
}
export type ReviewLine = InlineToken[]
export type ReviewBlock =
  | { type: 'p'; lines: ReviewLine[] }
  | { type: 'ul'; items: ReviewLine[] }

// Longest markers first so ** wins over *. Italic forbids inner newlines so a
// line-leading "* " list marker never pairs with a later asterisk.
//
// No lookbehind: Safari/iOS <16.4 throws a SyntaxError parsing `(?<!...)` at
// script-load time, which would crash the whole app on those devices. Instead
// of fencing the italic opener with a lookbehind, we post-filter in
// parseInline below: a candidate italic match whose immediately-preceding
// source character is "*" is really the tail of an unmatched "**" pair (e.g.
// "**open *and ~~more") and must be rejected and treated as literal text —
// otherwise a lone "*" that is half of a "**" would get greedily paired with
// the next stray "*" and swallow real text into a fake italic token. Only
// lookahead survives ((?!\*) is supported everywhere), used to stop an
// italic close from matching the first "*" of a following "**".
const INLINE_RE = /\*\*([^*]+)\*\*|\*([^*\n]+)\*(?!\*)|~~([^~]+)~~|\|\|([^|]+)\|\|/g

function parseInline(line: string): ReviewLine {
  const tokens: ReviewLine = []
  let last = 0
  INLINE_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = INLINE_RE.exec(line))) {
    const idx = m.index
    if (m[2] !== undefined && idx > 0 && line[idx - 1] === '*') {
      // Rejected italic candidate (preceded by "*") — not a real opener;
      // resume scanning right after this "*" so later real tokens still match.
      INLINE_RE.lastIndex = idx + 1
      continue
    }
    if (idx > last) tokens.push({ kind: 'text', text: line.slice(last, idx) })
    if (m[1] !== undefined) tokens.push({ kind: 'bold', text: m[1] })
    else if (m[2] !== undefined) tokens.push({ kind: 'italic', text: m[2] })
    else if (m[3] !== undefined) tokens.push({ kind: 'strike', text: m[3] })
    else tokens.push({ kind: 'spoiler', text: m[4] })
    last = idx + m[0].length
  }
  if (last < line.length) tokens.push({ kind: 'text', text: line.slice(last) })
  return tokens
}

const LIST_ITEM_RE = /^[-*]\s+/

export function parseReviewMarkdown(src: string): ReviewBlock[] {
  const blocks: ReviewBlock[] = []
  for (const raw of src.split(/\n{2,}/)) {
    const chunk = raw.trim()
    if (!chunk) continue
    const lines = chunk.split('\n').map((l) => l.trim()).filter(Boolean)
    let para: ReviewLine[] = []
    let list: ReviewLine[] = []
    const flushPara = () => {
      if (para.length) blocks.push({ type: 'p', lines: para })
      para = []
    }
    const flushList = () => {
      if (list.length) blocks.push({ type: 'ul', items: list })
      list = []
    }
    for (const line of lines) {
      if (LIST_ITEM_RE.test(line)) {
        flushPara()
        list.push(parseInline(line.replace(LIST_ITEM_RE, '')))
      } else {
        flushList()
        para.push(parseInline(line))
      }
    }
    flushPara()
    flushList()
  }
  return blocks
}
