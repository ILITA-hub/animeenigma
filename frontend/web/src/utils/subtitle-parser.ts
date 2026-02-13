/**
 * Subtitle parser for ASS, SRT, and VTT formats.
 * ASS parsing uses ass-compiler (dynamically imported).
 * SRT/VTT are parsed inline with no external dependencies.
 */

export interface SubtitleCue {
  start: number    // seconds
  end: number      // seconds
  text: string     // plain text (tags stripped)
  html: string     // HTML with basic styling
  style?: {
    color?: string
    fontSize?: number
    bold?: boolean
    italic?: boolean
    alignment?: number // ASS \an1-9
  }
}

// Strip ASS override tags like {\b1}, {\c&H0000FF&}, etc.
function stripASSTags(text: string): string {
  return text
    .replace(/\{[^}]*\}/g, '')
    .replace(/\\N/g, '\n')
    .replace(/\\n/g, '\n')
    .replace(/\\h/g, ' ')
    .trim()
}

// Extract basic style info from ASS override tags
function extractASSStyle(text: string): SubtitleCue['style'] {
  const style: SubtitleCue['style'] = {}

  // Bold: {\b1}
  if (/\{[^}]*\\b1[^}]*\}/.test(text)) style.bold = true
  // Italic: {\i1}
  if (/\{[^}]*\\i1[^}]*\}/.test(text)) style.italic = true

  // Color: {\c&HBBGGRR&} or {\1c&HBBGGRR&}
  const colorMatch = text.match(/\{[^}]*\\(?:1?c)&H([0-9A-Fa-f]{6})&[^}]*\}/)
  if (colorMatch) {
    const bgr = colorMatch[1]
    // ASS uses BGR, convert to RGB
    const r = bgr.slice(4, 6)
    const g = bgr.slice(2, 4)
    const b = bgr.slice(0, 2)
    style.color = `#${r}${g}${b}`
  }

  // Alignment: {\an1} through {\an9}
  const anMatch = text.match(/\{[^}]*\\an(\d)[^}]*\}/)
  if (anMatch) style.alignment = parseInt(anMatch[1])

  // Font size: {\fs24}
  const fsMatch = text.match(/\{[^}]*\\fs(\d+)[^}]*\}/)
  if (fsMatch) style.fontSize = parseInt(fsMatch[1])

  return style
}

// Convert plain text + style to HTML
function textToHtml(text: string, style?: SubtitleCue['style']): string {
  // Escape HTML entities
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')

  if (style?.bold) html = `<b>${html}</b>`
  if (style?.italic) html = `<i>${html}</i>`
  if (style?.color) html = `<span style="color:${style.color}">${html}</span>`

  return html
}

// Parse ASS content using ass-compiler (dynamically imported)
export async function parseASS(content: string): Promise<SubtitleCue[]> {
  const { parse } = await import('ass-compiler')
  const parsed = parse(content)

  const cues: SubtitleCue[] = []

  for (const event of parsed.events.dialogue) {
    const rawText = event.Text?.combined || event.Text?.parsed?.map((p: any) => {
      if (typeof p === 'string') return p
      if (p.text) return p.text
      return ''
    }).join('') || ''

    const style = extractASSStyle(rawText)
    const text = stripASSTags(rawText)
    if (!text) continue

    cues.push({
      start: event.Start,
      end: event.End,
      text,
      html: textToHtml(text, style),
      style,
    })
  }

  // Sort by start time
  cues.sort((a, b) => a.start - b.start)
  return cues
}

// Parse timestamp "HH:MM:SS,mmm" or "HH:MM:SS.mmm" to seconds
function parseTimestamp(ts: string): number {
  const parts = ts.trim().replace(',', '.').split(':')
  if (parts.length !== 3) return 0
  const hours = parseInt(parts[0]) || 0
  const minutes = parseInt(parts[1]) || 0
  const seconds = parseFloat(parts[2]) || 0
  return hours * 3600 + minutes * 60 + seconds
}

// Parse SRT content
export function parseSRT(content: string): SubtitleCue[] {
  const cues: SubtitleCue[] = []
  const blocks = content.trim().split(/\n\s*\n/)

  for (const block of blocks) {
    const lines = block.trim().split('\n')
    if (lines.length < 3) continue

    // Find the timing line (contains "-->")
    const timingIndex = lines.findIndex(l => l.includes('-->'))
    if (timingIndex === -1) continue

    const timeParts = lines[timingIndex].split('-->')
    if (timeParts.length !== 2) continue

    const start = parseTimestamp(timeParts[0])
    const end = parseTimestamp(timeParts[1])
    const text = lines.slice(timingIndex + 1).join('\n')
      .replace(/<[^>]+>/g, '') // Strip HTML tags
      .trim()

    if (!text) continue

    cues.push({
      start,
      end,
      text,
      html: textToHtml(text),
    })
  }

  return cues
}

// Parse VTT content
export function parseVTT(content: string): SubtitleCue[] {
  // Remove WEBVTT header
  const body = content.replace(/^WEBVTT[^\n]*\n/, '').trim()
  return parseSRT(body)
}
