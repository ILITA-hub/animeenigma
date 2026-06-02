// Autocapture helpers: build a stable-ish CSS selector, strip PII from text,
// and extract a click descriptor. Respects `data-no-track` opt-out on the
// element or any ancestor.
const EMAIL_RE = /[\w.+-]+@[\w-]+\.[\w.-]+/g
const DIGIT_RUN_RE = /\d{6,}/g // phone / card-like runs
const MAX_TEXT = 200
const MAX_DEPTH = 3

export function isTrackable(el: Element | null): boolean {
  let cur: Element | null = el
  while (cur) {
    if (cur.hasAttribute && cur.hasAttribute('data-no-track')) return false
    cur = cur.parentElement
  }
  return true
}

function selectorPart(el: Element): string {
  let part = el.tagName.toLowerCase()
  if (el.id) {
    part += `#${el.id}`
    return part
  }
  const cls = (el.getAttribute('class') || '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
  if (cls.length) part += '.' + cls.join('.')
  return part
}

// buildSelector walks up to MAX_DEPTH ancestors, building a descendant path.
// Stops early at an element with an id (ids are unique enough).
export function buildSelector(el: Element): string {
  const parts: string[] = []
  let cur: Element | null = el
  let depth = 0
  while (cur && depth < MAX_DEPTH) {
    const part = selectorPart(cur)
    parts.unshift(part)
    if (part.includes('#')) break
    cur = cur.parentElement
    depth++
  }
  return parts.join(' > ')
}

export function stripPII(text: string): string {
  return text
    .replace(EMAIL_RE, '[email]')
    .replace(DIGIT_RUN_RE, '[num]')
    .slice(0, MAX_TEXT)
}

export interface ClickDescriptor {
  el_tag: string
  el_selector: string
  el_text: string
  el_attrs: Record<string, string>
}

export function extractClick(el: Element): ClickDescriptor | null {
  if (!isTrackable(el)) return null
  const attrs: Record<string, string> = {}
  for (const a of Array.from(el.attributes)) {
    if (a.name.startsWith('data-') && a.name !== 'data-no-track') {
      attrs[a.name] = a.value
    }
  }
  const raw = (el.textContent || '').trim().replace(/\s+/g, ' ')
  return {
    el_tag: el.tagName.toLowerCase(),
    el_selector: buildSelector(el),
    el_text: stripPII(raw),
    el_attrs: attrs,
  }
}
