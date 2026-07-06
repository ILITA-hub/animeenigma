/**
 * Safe fanfic markdown renderer (Task 10, spec 2026-07-06).
 *
 * Tiny, dependency-free heading/paragraph splitter. Returns typed blocks that
 * FanficReader.vue renders as TEXT nodes (`{{ b.text }}`, never `v-html`) —
 * so injected markup in generated (or user-submitted) content can never
 * execute as HTML. This is the entire XSS defense for the reader: no markdown
 * library, no innerHTML, just headings (#/##) + blank-line paragraphs.
 */

export type FanficBlock = { type: 'h2' | 'h3' | 'p'; text: string }

/** Minimal, safe fanfic renderer: headings (#/##) + blank-line paragraphs. */
export function renderFanfic(md: string): FanficBlock[] {
  const blocks: FanficBlock[] = []
  for (const raw of md.split(/\n{2,}/)) {
    const chunk = raw.trim()
    if (!chunk) continue
    if (chunk.startsWith('## ')) blocks.push({ type: 'h3', text: chunk.slice(3).trim() })
    else if (chunk.startsWith('# ')) blocks.push({ type: 'h2', text: chunk.slice(2).trim() })
    else blocks.push({ type: 'p', text: chunk.replace(/\n/g, ' ') })
  }
  return blocks
}
