import { absolute } from '@/offline/playlistRewrite'

export interface StoryboardCue {
  start: number
  end: number
  url: string
  x: number
  y: number
  w: number
  h: number
}

const TIMING = /^(\d{2,}):(\d{2}):(\d{2})\.(\d{3})\s+-->\s+(\d{2,}):(\d{2}):(\d{2})\.(\d{3})/
const XYWH = /#xywh=(\d+),(\d+),(\d+),(\d+)\s*$/

function toSec(h: string, m: string, s: string, ms: string): number {
  return Number(h) * 3600 + Number(m) * 60 + Number(s) + Number(ms) / 1000
}

/** Parse a WebVTT thumbnail track (url#xywh cue payloads). Malformed cues are
 *  skipped — a broken storyboard degrades to "no preview", never to a throw. */
export function parseStoryboardVtt(text: string, baseUrl: string): StoryboardCue[] {
  const cues: StoryboardCue[] = []
  const lines = text.split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    const t = TIMING.exec(lines[i])
    if (!t) continue
    const payload = (lines[i + 1] ?? '').trim()
    const g = XYWH.exec(payload)
    if (!g) continue
    const raw = payload.slice(0, payload.indexOf('#'))
    // Only absolute or root-relative URLs are valid — the proxy rewrites every
    // cue to a signed /api/streaming/hls-proxy URL. A bare-relative name means
    // the rewrite didn't happen; resolving it client-side could never produce
    // a fetchable (signed) sheet URL, so skip the cue.
    if (!/^(?:https?:)?\/\//.test(raw) && !raw.startsWith('/')) continue
    let url: string
    try {
      // baseUrl is usually a ROOT-RELATIVE proxy path (/api/streaming/hls-proxy?…)
      // — hlsProxyUrl() emits relative URLs unless VITE_HLS_PROXY_BASE is set, and
      // new URL() throws "Invalid URL" on a relative base. absolute() anchors on
      // the document origin — shared with offline/playlistRewrite.ts, which
      // resolves playlist resource URIs the same way.
      url = absolute(raw, baseUrl)
    } catch {
      continue
    }
    cues.push({
      start: toSec(t[1], t[2], t[3], t[4]),
      end: toSec(t[5], t[6], t[7], t[8]),
      url,
      x: Number(g[1]),
      y: Number(g[2]),
      w: Number(g[3]),
      h: Number(g[4]),
    })
  }
  return cues
}

/** Binary search for the cue covering t (cues are emitted sorted). */
export function cueAt(cues: StoryboardCue[], t: number): StoryboardCue | null {
  let lo = 0
  let hi = cues.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    const c = cues[mid]
    if (t < c.start) hi = mid - 1
    else if (t >= c.end) lo = mid + 1
    else return c
  }
  return null
}
