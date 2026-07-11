import { ladder } from '@/utils/protocolLadder'

/** The currently active HLS-proxy origin — the protocol ladder's active tier
 *  (single-origin `VITE_HLS_PROXY_BASE`, same-origin when unset, or one of the
 *  `VITE_HLS_PROXY_TIERS` h3/h2/h1 origins). No trailing slash. */
function hlsProxyBase(): string {
  return ladder.currentBase().replace(/\/+$/, '')
}

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
  return `${hlsProxyBase()}/api/streaming/hls-proxy?${query}`
}

/**
 * Roots a backend-minted masked proxy path (`/api/streaming/m/<token>/<leaf>`,
 * Track A opaque stream tokens) at the same active HLS tier as hlsProxyUrl, so
 * masked segment traffic — the bulk of stream egress — rides the protocol
 * ladder's h3→h2→h1 fallback rather than bypassing it to a static origin.
 */
export function maskedStreamUrl(path: string): string {
  return `${hlsProxyBase()}${path}`
}

/**
 * True if `url` is already a first-party proxy/masked URL — a same-origin
 * relative path, or absolute at one of the protocol ladder's tier origins
 * (`maskedStreamUrl()`/`hlsProxyUrl()` output). Callers use this to avoid
 * re-wrapping an already-authorized URL through `hlsProxyUrl()`, which the
 * backend's third-party allowlist gate rejects (our own tier hosts are
 * intentionally not on it — see `HLSProxyAllowedDomains`).
 */
export function isFirstPartyProxyUrl(url: string): boolean {
  return ladder.ownsUrl(url)
}
