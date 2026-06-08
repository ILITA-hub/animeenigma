// Autocapture helpers: resolve the meaningful element behind a click, derive a
// human-readable label for it, build a stable-ish CSS selector, and strip PII.
// Respects `data-no-track` opt-out on the element or any ancestor.
const EMAIL_RE = /[\w.+-]+@[\w-]+\.[\w.-]+/g
const DIGIT_RUN_RE = /\d{6,}/g // phone / card-like runs
const MAX_TEXT = 200
const MAX_DEPTH = 3
const LABEL_SUBTREE_CAP = 80

// Elements that represent a meaningful interaction. A click landing on an inner
// <svg>/<span>/<img> is attributed to the nearest of these (see resolveTarget).
// `[data-track]` is included so an explicit analytics anchor on any element wins.
const INTERACTIVE_SEL =
  'a, button, input, select, textarea, label, summary, ' +
  '[role="button"], [role="link"], [role="tab"], [role="menuitem"], [data-track]'

// Tags small enough that their full (capped) subtree text is a fine label when
// they carry no direct text of their own (e.g. a logo anchor wrapping spans).
const SUBTREE_TAGS = new Set(['a', 'button', 'label', 'summary'])

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

// resolveTarget attributes a click to the nearest enclosing interactive element
// (self included). A click on the <path> inside an icon button is reported as
// the button; a click on either <span> half of a split logo is reported as the
// wrapping <a>. Falls back to the clicked element when nothing interactive is
// nearby — never climbs to <html>/<body>, which keeps page-level junk out.
function resolveTarget(el: Element): Element {
  const hit = el.closest ? el.closest(INTERACTIVE_SEL) : null
  return (hit as Element | null) ?? el
}

function attr(el: Element, name: string): string {
  return (el.getAttribute(name) || '').trim()
}

// directText returns only the element's OWN immediate text — never the full
// subtree. This is what prevents clicks near the DOM root from sweeping in the
// entire page (including <script>/comment text) as a "label".
function directText(el: Element): string {
  let s = ''
  for (const n of Array.from(el.childNodes)) {
    if (n.nodeType === Node.TEXT_NODE) s += n.textContent || ''
  }
  return s.replace(/\s+/g, ' ').trim()
}

// elementLabel derives a human-readable label, preferring (in order):
//   1. data-track  — an intentional, refactor-stable analytics name
//   2. own visible text — the most natural label when present
//   3. accessible-name attributes (aria-label, title) for textless controls
//   4. type-specific attributes (img alt, input placeholder/value)
//   5. capped subtree text, but ONLY for small interactive controls
//   6. ⟨tag⟩ — give up readably; never emit page/subtree text for containers
export function elementLabel(el: Element): string {
  const tag = el.tagName.toLowerCase()

  const dataTrack = attr(el, 'data-track')
  if (dataTrack) return dataTrack

  const own = directText(el)
  if (own) return own

  const aria = attr(el, 'aria-label')
  if (aria) return aria

  const title = attr(el, 'title')
  if (title) return title

  if (tag === 'img') {
    const alt = attr(el, 'alt')
    if (alt) return alt
  }
  if (tag === 'input' || tag === 'textarea') {
    const ph = attr(el, 'placeholder')
    if (ph) return ph
    const val = attr(el, 'value')
    if (tag === 'input' && val) return val
  }

  if (SUBTREE_TAGS.has(tag) || el.getAttribute('role') === 'button') {
    const sub = (el.textContent || '').replace(/\s+/g, ' ').trim()
    if (sub) return sub.slice(0, LABEL_SUBTREE_CAP)
  }

  return `⟨${tag}⟩`
}

export interface ClickDescriptor {
  el_tag: string
  el_selector: string
  el_text: string
  el_attrs: Record<string, string>
}

export function extractClick(el: Element): ClickDescriptor | null {
  if (!isTrackable(el)) return null
  const target = resolveTarget(el)
  const attrs: Record<string, string> = {}
  for (const a of Array.from(target.attributes)) {
    if (a.name.startsWith('data-') && a.name !== 'data-no-track') {
      attrs[a.name] = a.value
    }
  }
  return {
    el_tag: target.tagName.toLowerCase(),
    el_selector: buildSelector(target),
    el_text: stripPII(elementLabel(target)),
    el_attrs: attrs,
  }
}
