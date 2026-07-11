import { ladder } from '@/utils/protocolLadder'

/**
 * Builds an HLS-proxy URL. By default returns a same-origin relative path
 * (`/api/streaming/hls-proxy?<query>`). The URL is rooted at the protocol
 * ladder's currently active tier (`ladder.currentBase()`) — single-origin
 * (`VITE_HLS_PROXY_BASE`) or same-origin when unset, or one of the
 * `VITE_HLS_PROXY_TIERS` origins (h3/h2/h1) once the ladder has more than one
 * tier configured. The proxy already sends
 * `Access-Control-Allow-Origin: *`, so cross-subdomain fetches work without
 * further CORS changes.
 *
 * Scope: HLS video + subtitle-track fetches only. Image proxy stays same-origin.
 *
 * @param query the already-encoded query string WITHOUT the leading `?`
 */
export function hlsProxyUrl(query: string): string {
  const base = ladder.currentBase().replace(/\/+$/, '')
  return `${base}/api/streaming/hls-proxy?${query}`
}
