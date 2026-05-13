const SHIKIMORI_BASE = 'https://shikimori.one'

const TYPE_URL_MAP: Record<string, string> = {
  character: 'characters',
  anime: 'animes',
  manga: 'mangas',
  ranobe: 'ranobe',
  person: 'people',
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;')
}

function slugToDisplay(slug: string): string {
  const text = slug.replace(/-/g, ' ')
  return text.charAt(0).toUpperCase() + text.slice(1)
}

function makeLink(type: string, id: string, text: string): string {
  const urlPath = TYPE_URL_MAP[type]
  if (!urlPath) return text
  return `<a href="${SHIKIMORI_BASE}/${urlPath}/${id}" target="_blank" rel="noopener" class="shiki-link">${text}</a>`
}

// parseDescription converts Shikimori BBCode-style markup into safe HTML.
//
// XSS-safety invariant — REVIEW.md CR-05:
//   1. The first step ALWAYS calls escapeHtml() on the raw input. After
//      this step, the string contains zero unescaped `<`, `>`, `"`, `'`
//      or `&` characters, so no attacker-controlled HTML tags or
//      attributes can survive into the output.
//   2. Subsequent regex replacements ONLY interpolate already-escaped
//      capture groups (or numeric ids matched by \d+) into HARDCODED
//      HTML templates (<a>, <span>, <br>). The href base is a constant
//      and the URL path comes from a whitelist (TYPE_URL_MAP).
//   3. No raw user content is ever placed in an attribute value or in a
//      javascript: URL context.
// Result: the output is safe to bind via v-html. DOMPurify is therefore
// NOT required at the call site. Any future edit that bypasses
// escapeHtml() or interpolates non-numeric capture groups into href
// attributes MUST re-evaluate this invariant.
export function parseDescription(raw: string): string {
  let text = escapeHtml(raw)

  // 1. Pair tags: [type=ID slug]Display Text[/type] and [type=ID]Display Text[/type]
  text = text.replace(
    /\[(character|anime|manga|ranobe|person)=(\d+)[^\]]*\](.*?)\[\/\1\]/g,
    (_, type, id, content) => makeLink(type, id, content),
  )

  // 2. Self-closing tags: [type=ID slug-text]
  text = text.replace(
    /\[(character|anime|manga|ranobe|person)=(\d+)\s+([^\]]+)\]/g,
    (_, type, id, slug) => makeLink(type, id, slugToDisplay(slug)),
  )

  // 3. [size=N]text[/size] → small muted text
  text = text.replace(
    /\[size=\d+\](.*?)\[\/size\]/gs,
    (_, content) => `<span class="shiki-footnote">${content}</span>`,
  )

  // 4. [[text]] → plain text
  text = text.replace(/\[\[([^\]]+)\]\]/g, '$1')

  // 5. Newlines → <br>
  text = text.replace(/\n/g, '<br>')

  return text
}
