// Line-based m3u8 rewriting for offline storage. Deliberately NOT a full HLS
// parser — anime provider playlists are simple VOD lists; hls.js re-parses the
// rewritten output at playback, so structural fidelity is what matters.
import { offlinePath } from './types'

export interface PlaylistResource {
  path: string
  url: string
}

export function isVod(body: string): boolean {
  return body.includes('#EXT-X-ENDLIST')
}

/** Master playlist → variant URI closest to target (largest height ≤ target,
 *  else the smallest available). Null ⇒ no #EXT-X-STREAM-INF (media playlist). */
export function selectVariant(masterBody: string, targetHeight: number): { uri: string } | null {
  const lines = masterBody.split('\n')
  const variants: { height: number; uri: string }[] = []
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].startsWith('#EXT-X-STREAM-INF')) continue
    const res = /RESOLUTION=(\d+)x(\d+)/.exec(lines[i])
    const height = res ? parseInt(res[2], 10) : 0
    for (let j = i + 1; j < lines.length; j++) {
      const l = lines[j].trim()
      if (l === '' || l.startsWith('#')) continue
      variants.push({ height, uri: l })
      break
    }
  }
  if (variants.length === 0) return null
  const fitting = variants.filter((v) => v.height <= targetHeight)
  const pick = fitting.length
    ? fitting.reduce((a, b) => (b.height > a.height ? b : a))
    : variants.reduce((a, b) => (b.height < a.height ? b : a))
  return { uri: pick.uri }
}

/** Resolve a (possibly root-relative) URI against baseUrl, anchored on the
 *  document origin. Shared with components/player/aePlayer/storyboardVtt.ts,
 *  which resolves storyboard sprite-sheet cue URLs the same way. */
export function absolute(uri: string, baseUrl: string): string {
  // baseUrl is usually a ROOT-RELATIVE proxy path (/api/streaming/hls-proxy?…)
  // — hlsProxyUrl() emits relative URLs unless VITE_HLS_PROXY_BASE is set, and
  // new URL() throws "Invalid base URL" on a relative base. Anchor on the
  // document origin. (The proxy itself rewrites child URIs — segments AND
  // EXT-X-KEY/EXT-X-MAP — to root-relative /api/streaming/hls-proxy?… URLs,
  // libs/videoutils/proxy.go, so this path is the COMMON case, not the edge.)
  return new URL(uri, new URL(baseUrl, window.location.href)).href
}

/** Rewrite a MEDIA playlist: every segment URI → /__offline/{id}/r/{n}, every
 *  EXT-X-KEY URI → /k/{n}, every EXT-X-MAP URI → /m/{n}. Returns the rewritten
 *  body plus the local-path → remote-URL fetch list. */
export function rewriteMediaPlaylist(
  body: string,
  baseUrl: string,
  id: string,
): { body: string; resources: PlaylistResource[] } {
  const resources: PlaylistResource[] = []
  let seg = 0
  let key = 0
  let map = 0
  const out = body.split('\n').map((line) => {
    const t = line.trim()
    if (t.startsWith('#EXT-X-KEY') && t.includes('URI="')) {
      return line.replace(/URI="([^"]+)"/, (_, uri: string) => {
        const path = offlinePath(id, `k/${key++}`)
        resources.push({ path, url: absolute(uri, baseUrl) })
        return `URI="${path}"`
      })
    }
    if (t.startsWith('#EXT-X-MAP') && t.includes('URI="')) {
      return line.replace(/URI="([^"]+)"/, (_, uri: string) => {
        const path = offlinePath(id, `m/${map++}`)
        resources.push({ path, url: absolute(uri, baseUrl) })
        return `URI="${path}"`
      })
    }
    if (t !== '' && !t.startsWith('#')) {
      const path = offlinePath(id, `r/${seg++}`)
      resources.push({ path, url: absolute(t, baseUrl) })
      return path
    }
    return line
  })
  return { body: out.join('\n'), resources }
}
