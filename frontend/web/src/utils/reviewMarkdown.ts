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
// line-leading "* " list marker never pairs with a later asterisk. Italic's
// delimiters are also fenced with (?<!\*)/(?!\*) so a lone "*" that is really
// half of an unmatched "**" pair (e.g. "**open *and ~~more") can never be
// mistaken for an italic opener/closer — without the fence, matchAll's
// non-backtracking-across-alternatives scan would greedily pair the second
// "*" of "**" with the next stray "*" and swallow real text into a fake
// italic token.
const INLINE_RE = /\*\*([^*]+)\*\*|(?<!\*)\*(?!\*)([^*\n]+)(?<!\*)\*(?!\*)|~~([^~]+)~~|\|\|([^|]+)\|\|/g

function parseInline(line: string): ReviewLine {
  const tokens: ReviewLine = []
  let last = 0
  for (const m of line.matchAll(INLINE_RE)) {
    const idx = m.index ?? 0
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
