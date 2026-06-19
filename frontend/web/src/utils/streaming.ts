/**
 * Builds an HLS-proxy URL. By default returns a same-origin relative path
 * (`/api/streaming/hls-proxy?<query>`). When `VITE_HLS_PROXY_BASE` is set
 * (e.g. `https://stream.animeenigma.org`), the URL is rooted at that host so
 * heavy segment traffic is served from the dedicated HLS subdomain. The proxy
 * already sends `Access-Control-Allow-Origin: *`, so cross-subdomain fetches
 * work without further CORS changes.
 *
 * Scope: HLS video + subtitle-track fetches only. Image proxy stays same-origin.
 *
 * @param query the already-encoded query string WITHOUT the leading `?`
 */
export function hlsProxyUrl(query: string): string {
  const base = (import.meta.env.VITE_HLS_PROXY_BASE || '').replace(/\/+$/, '')
  return `${base}/api/streaming/hls-proxy?${query}`
}
