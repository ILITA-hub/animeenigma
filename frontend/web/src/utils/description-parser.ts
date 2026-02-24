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
