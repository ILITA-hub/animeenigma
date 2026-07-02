// transport.go — Miruro secure-pipe transport (Camoufox browser routing).
//
// Camoufox migration (2026-07-02): www.miruro.tv now sits behind Cloudflare — an
// interactive Turnstile on the SPA/homepage AND a HARD WAF managed-rule block
// ("Sorry, you have been blocked") on /api/secure/pipe for any un-cleared client.
// A stdlib-only Go client (D3 gate 2) 403s on both. When the DB roster sets this
// provider's engine="browser", the secure-pipe GET is routed through the Camoufox
// stealth-scraper sidecar's warm /fetch session, which solves the homepage
// Turnstile (~9s, our own datacenter IP, no residential proxy — verified live
// 2026-07-02); the in-page fetch to /api/secure/pipe then rides the same origin +
// cf_clearance + TLS fingerprint and is served as the SPA (200 + x-obfuscated).
//
// Owner-locked Approach 2 — Go still BUILDS every `e=` request descriptor
// (obfuscation.go BuildSecurePipeURL) and DECODES every response
// (DecodeObfuscatedResponse); the sidecar is a pure browser-fetch execution layer.
// The one wrinkle vs animepahe/nineanime: miruro signals its response transport
// codec in the `x-obfuscated` RESPONSE HEADER, so the browser-fetch closure surfaces
// response headers (sidecar.Client.FetchWithHeaders → /fetch header allowlist).
//
// engine=http is a DEGRADED fallback shape only — a curl-class GET of
// www.miruro.tv is Cloudflare-blocked, so plain requests 403 — kept so the parser
// still behaves when the browser route is off and so unit tests can drive
// doSecurePipe against a local httptest server.
package miruro

import "context"

// BrowserFetchFunc routes one secure-pipe GET through the Camoufox stealth-scraper
// sidecar's warm, challenge-solved /fetch session, returning
// (upstreamStatus, responseHeaders, body, err). Unlike the animepahe/nineanime
// closures it ALSO returns response headers because miruro's transport codec is
// carried by the `x-obfuscated` response header, not the body. Headers keys are
// lowercase (the sidecar lowercases them). Mirrors the closure injected in main.go.
type BrowserFetchFunc func(ctx context.Context, provider, url string) (int, map[string]string, []byte, error)

// browserEnabled reports whether the secure-pipe GET should route through the
// sidecar (DB engine="browser"). Requires both the live gate and the fetch closure
// to be wired — a partial wiring degrades to the plain-HTTP fallback rather than
// panicking on a nil closure.
func (p *Provider) browserEnabled() bool {
	return p.useBrowser != nil && p.browserFetch != nil && p.useBrowser()
}
