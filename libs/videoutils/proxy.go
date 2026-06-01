package videoutils

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// VideoSource represents a source for video streaming
type VideoSource string

const (
	SourceMinio    VideoSource = "minio"    // Self-hosted MinIO storage
	SourceExternal VideoSource = "external" // External API/CDN
	SourceYouTube  VideoSource = "youtube"  // YouTube (for openings/endings)
)

// VideoStreamInfo contains information about a video stream
type VideoStreamInfo struct {
	Source      VideoSource `json:"source"`
	URL         string      `json:"url"`
	Quality     string      `json:"quality"`
	ContentType string      `json:"content_type"`
	Size        int64       `json:"size,omitempty"`
	Duration    int         `json:"duration,omitempty"` // seconds
}

// ProxyConfig configures the video proxy
type ProxyConfig struct {
	UserAgent       string        `json:"user_agent" yaml:"user_agent"`
	Timeout         time.Duration `json:"timeout" yaml:"timeout"`
	MaxBufferSize   int64         `json:"max_buffer_size" yaml:"max_buffer_size"`
	AllowedDomains  []string      `json:"allowed_domains" yaml:"allowed_domains"`
	RefererOverride string        `json:"referer_override" yaml:"referer_override"`
}

// DefaultProxyConfig returns sensible defaults
func DefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		Timeout:       30 * time.Second,
		MaxBufferSize: 10 * 1024 * 1024, // 10MB buffer
		AllowedDomains: []string{
			"storage.googleapis.com",
			"cdn.myanimelist.net",
			"shikimori.one",
			"nyaa.si",
			// Add more trusted domains
		},
	}
}

// VideoProxy handles proxying video streams from external sources
type VideoProxy struct {
	client *http.Client
	config ProxyConfig
}

// NewVideoProxy creates a new video proxy
func NewVideoProxy(cfg ProxyConfig) *VideoProxy {
	return &VideoProxy{
		client: &http.Client{
			// No timeout on client level — context cancellation handles timeouts.
			// A global timeout breaks streaming of large MP4 files (100s of MB).
			Transport: newIPv4Transport(),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Follow redirects but preserve headers
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		config: cfg,
	}
}

// newIPv4Transport returns an http.Transport whose dialer forces IPv4 ("tcp4").
//
// Several upstream CDNs (e.g. AnimePahe/Kwik's vault-*.owocdn.top edges) are
// dual-stack and return AAAA records, but our containers have no working IPv6
// egress. Go's default dual-stack dialer intermittently races the IPv6 address,
// which black-holes (no RST/ICMP) and stalls each connection until the upstream
// timeout fires — manifests/keys are tiny and usually slip through on the IPv4
// fallback, but the steady flood of HLS segments keeps landing on the dead IPv6
// path, so episodes load but never play. Forcing "tcp4" removes the IPv6 attempt
// entirely and makes segment fetches deterministic.
func newIPv4Transport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Coerce any TCP dial to IPv4-only.
		switch network {
		case "tcp", "tcp6":
			network = "tcp4"
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return t
}

// ProxyStream proxies a video stream from an external URL
func (p *VideoProxy) ProxyStream(ctx context.Context, sourceURL string, w http.ResponseWriter, r *http.Request) error {
	// Validate URL is from allowed domain
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source url: %w", err)
	}

	if !p.isDomainAllowed(parsed.Host) {
		return fmt.Errorf("domain not allowed: %s", parsed.Host)
	}

	// Create upstream request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers for upstream request
	req.Header.Set("User-Agent", p.config.UserAgent)
	if p.config.RefererOverride != "" {
		req.Header.Set("Referer", p.config.RefererOverride)
	}

	// Handle range requests for seeking
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// Make upstream request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream request: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set CORS headers for frontend access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Stream the response
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		// Client disconnected, not an error
		if strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "connection reset") {
			return nil
		}
		return fmt.Errorf("stream copy: %w", err)
	}

	return nil
}

// GetStreamInfo fetches information about a video stream without downloading
func (p *VideoProxy) GetStreamInfo(ctx context.Context, sourceURL string) (*VideoStreamInfo, error) {
	_, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", p.config.UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("head request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status: %d", resp.StatusCode)
	}

	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)

	return &VideoStreamInfo{
		Source:      SourceExternal,
		URL:         sourceURL,
		ContentType: resp.Header.Get("Content-Type"),
		Size:        size,
	}, nil
}

// isDomainAllowed checks if a domain is in the allowed list.
// Returns false when AllowedDomains is empty (fail-closed).
func (p *VideoProxy) isDomainAllowed(host string) bool {
	if len(p.config.AllowedDomains) == 0 {
		return false
	}

	host = strings.ToLower(host)
	for _, allowed := range p.config.AllowedDomains {
		allowed = strings.ToLower(allowed)
		// Wildcard prefix: "*.example.com" matches "cdn.example.com"
		// Also "htv-*" matches "htv-belias.com", "htv-hydaelyn.com", etc.
		if strings.HasPrefix(allowed, "*.") {
			suffix := allowed[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) {
				return true
			}
		} else if strings.HasSuffix(allowed, "*") {
			prefix := allowed[:len(allowed)-1] // "htv-"
			if strings.HasPrefix(host, prefix) || strings.Contains(host, "."+prefix) {
				return true
			}
		} else if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// StreamResult represents a resolved video stream
type StreamResult struct {
	Info      *VideoStreamInfo
	StreamFn  func(w http.ResponseWriter, r *http.Request) error
	ExpiresAt time.Time
}

// VideoResolver resolves video sources to streamable URLs
type VideoResolver interface {
	// Resolve finds the best available stream for a video
	Resolve(ctx context.Context, animeID, episodeNum string) (*StreamResult, error)

	// ResolveExternal resolves an external video URL
	ResolveExternal(ctx context.Context, externalURL string) (*StreamResult, error)
}

// AllowedDomain represents a single entry in the HLS proxy allow-list along
// with provenance metadata used by the quarterly review process.
//
// Provenance fields drive scripts/audit-hls-allowlist.sh — see
// docs/security/hls-proxy-allowlist.md for the review cadence and the
// requirements for new entries.
type AllowedDomain struct {
	// Domain is the host pattern matched by isHLSDomainAllowed. May be an
	// exact host (e.g. "kwik.cx"), an eTLD+1 (e.g. "premilkyway.com" — any
	// subdomain matches via the strings.HasSuffix(host, "."+allowed) gate),
	// or a prefix wildcard ending in "*" (e.g. "htv-*").
	Domain string
	// Reason is a short, human-readable note describing what this entry is
	// for (e.g. "AnimePahe CDN", "AllAnime upstream CDN"). Used by the audit
	// script and is the only place provenance lives in code.
	Reason string
	// Owner is the GitHub handle (with leading "@") of the person who
	// vouches for this entry during the quarterly review. Entries added
	// before the structured provenance refactor are owned by "@legacy" and
	// MUST be backfilled at the next review.
	Owner string
	// Added is the YYYY-MM-DD date this entry was added (or the date the
	// provenance refactor landed for entries with unknown history).
	Added string
}

// HLSProxyAllowedDomainsWithProvenance is the canonical structured allow-list
// used by the HLS proxy. Add new entries here only — the flat string view
// HLSProxyAllowedDomains is derived from this slice at package init time.
//
// IMPORTANT: New entries require a CODEOWNERS-gated review (see
// .github/CODEOWNERS) and must carry honest Owner/Added/Reason values.
// Quarterly review process: docs/security/hls-proxy-allowlist.md.
//
// The "@legacy" Owner + 2026-05-20 Added date on existing entries marks
// the structured-provenance refactor — those entries pre-date the new
// process and have no individually-attributable provenance. They will be
// backfilled with real owners on the next quarterly review.
var HLSProxyAllowedDomainsWithProvenance = []AllowedDomain{
	// Known streaming domains (HiAnime / MegaCloud family — predates this refactor).
	{Domain: "megacloud.tv", Reason: "MegaCloud HLS host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "megacloud.blog", Reason: "MegaCloud HLS host (mirror)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "megacloud.club", Reason: "MegaCloud HLS host (mirror)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "rapid-cloud.co", Reason: "Rapid-Cloud HLS host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "rapidcloud.live", Reason: "Rapid-Cloud HLS host (mirror)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "vidstream.pro", Reason: "VidStream HLS host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "vidstreamz.online", Reason: "VidStream HLS host (mirror)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "mcloud.to", Reason: "MCloud HLS host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "mcloud2.to", Reason: "MCloud HLS host (mirror)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "mgstatics.xyz", Reason: "MegaCloud static asset host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "netmagcdn.com", Reason: "MegaCloud HLS CDN", Owner: "@legacy", Added: "2026-05-20"},

	// AnimePahe CDN hosts (SCRAPER-PAHE-05).
	{Domain: "owocdn.top", Reason: "AnimePahe/Kwik CDN", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "uwucdn.top", Reason: "AnimePahe/Kwik CDN (mirror)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "kwik.cx", Reason: "AnimePahe CDN", Owner: "@legacy", Added: "2026-05-20"},

	// Japanese subtitles (Phase 14).
	{Domain: "jimaku.cc", Reason: "Japanese subtitle files", Owner: "@legacy", Added: "2026-05-20"},

	// AnimeLib video CDNs.
	{Domain: "cdnlibs.org", Reason: "AnimeLib video CDN", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "hentaicdn.org", Reason: "AnimeLib video CDN (mirror)", Owner: "@legacy", Added: "2026-05-20"},

	// Hanime video CDN family.
	{Domain: "hanime.tv", Reason: "Hanime primary host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "highwinds-cdn.com", Reason: "Hanime Highwinds CDN", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "htv-*", Reason: "Hanime htv-belias.com, htv-hydaelyn.com, etc. (prefix wildcard)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "hydaelyn-*", Reason: "Hanime hydaelyn-25x-00.top through 19.top (prefix wildcard)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "zodiark-*", Reason: "Hanime zodiark-25x-00.top through 09.top (prefix wildcard)", Owner: "@legacy", Added: "2026-05-20"},

	// Phase 18 — Anitaku/Gogoanime CDN entries.
	// Rotating subdomains match via strings.HasSuffix(host, "."+allowed) in isHLSDomainAllowed.
	{Domain: "anitaku.to", Reason: "Anitaku poster + future-proxy host (Phase 18)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "vibeplayer.site", Reason: "Vibeplayer same-origin HLS host (Phase 18)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "premilkyway.com", Reason: "StreamHG primary CDN, rotating subdomain on eTLD+1 (Phase 18)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "dramiyos-cdn.com", Reason: "Earnvids primary CDN, rotating subdomain on eTLD+1 (Phase 18)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "cdn.cimovix.store", Reason: "Subtitle .vtt host shared by vibeplayer/streamhg/earnvids (Phase 18)", Owner: "@legacy", Added: "2026-05-20"},

	// Phase 22 — Provider Robustness (SCRAPER-HEAL-10).
	{Domain: "managementadvisory.sbs", Reason: "StreamHG hls3 CDN, rotating subdomain on eTLD+1 (Phase 22 SCRAPER-HEAL-10)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "exoplanethunting.space", Reason: "Earnvids hls3 CDN, rotating subdomain on eTLD+1 (Phase 22 SCRAPER-HEAL-10)", Owner: "@legacy", Added: "2026-05-20"},

	// v3.1 milestone audit hotfix 2026-05-13 (closes BLK-INT-01).
	{Domain: "cdn-centaurus.com", Reason: "Observed StreamHG/Earnvids primary CDN, post-Phase-22 rotation", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "meadowlarkdesignstudio.cfd", Reason: "Observed hls3 CDN, post-Phase-22 rotation", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "goldenridgeproduction.shop", Reason: "Observed hls3 CDN (DEF-22-01)", Owner: "@legacy", Added: "2026-05-20"},

	// Workstream raw-jp / Phase 01 — AllAnime raw-JP CDN families.
	{Domain: "allanime.day", Reason: "AllAnime edge host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "allanime.to", Reason: "AllAnime edge host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "allmanga.to", Reason: "AllAnime edge host", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "wixmp.com", Reason: "Common AllAnime upstream CDN", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "wixmp-ed30a86b8c4858749c87952r.akamaized.net", Reason: "wixmp signed edges (AllAnime)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "blogger.com", Reason: "YouTube-fed mirror used by some AllAnime sources", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "googlevideo.com", Reason: "Direct YouTube CDN (some AllAnime sources resolve here)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "sharepoint.com", Reason: "OneDrive-backed source variant (AllAnime)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "fast4speed.rsvp", Reason: "AllAnime own CDN — direct MP4 with signed Authorization, requires Referer: https://allmanga.to/", Owner: "@legacy", Added: "2026-05-20"},

	// Phase 28 (SCRAPER-HEAL-36) — AnimeFever embed + HLS CDN hosts.
	{Domain: "am.vidstream.vip", Reason: "AnimeFever JWPlayer embed page host (Phase 28)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "static-cdn-ca1.mofl.pro", Reason: "AnimeFever HLS master.m3u8 + segments CDN (Phase 28)", Owner: "@legacy", Added: "2026-05-20"},

	// Phase 28 (SCRAPER-HEAL-37) — Miruro proxy hosts.
	// uwucdn.top already covered by AnimePahe legacy entry above; Miruro
	// shares the vault-*.uwucdn.top edges via the animepahe-derived 'kiwi'
	// source family.
	{Domain: "pro.ultracloud.cc", Reason: "Miruro upstream proxy host (Phase 28)", Owner: "@legacy", Added: "2026-05-20"},
	{Domain: "pru.ultracloud.cc", Reason: "Miruro alternate proxy host (Phase 28)", Owner: "@legacy", Added: "2026-05-20"},

	// Phase 28 (SCRAPER-HEAL-39) — 9anime.me.uk MP4 embed + CDN host.
	// my.1anime.site serves the <iframe src="...index.php?action=play&file=
	// <name>.mp4"> + the absolute MP4 at <host>/videos/<name>.mp4 with
	// Accept-Ranges: bytes. Cloudflare-fronted, Engintron caching.
	// Per D7 the allowlist entry lands in the same commit as the provider
	// registration; if the 9anime provider is later DEGRADED, this entry
	// stays (it's harmless — Phase 25 SCRAPER-HEAL-24 returns 502 on
	// unallowed hosts so a degraded provider can't accidentally serve
	// content through this entry).
	{Domain: "my.1anime.site", Reason: "9anime.me.uk MP4 embed + CDN host (Phase 28 SCRAPER-HEAL-39)", Owner: "@legacy", Added: "2026-05-20"},

	// 2026-06-01 — 9anime.me.uk popular catalog migrated to the megaplay.buzz
	// HLS player (1anime.site wrapper). The master + variant playlists live on
	// cdn.mewstream.buzz (stable, statically allowed here so it can seed
	// provenance tokens); subtitle .vtt tracks on *.lostproject.club. The
	// actual .ts SEGMENTS rotate across an UNBOUNDED pool of throwaway
	// .click/.buzz/.club domains and are therefore NOT listed — they ride the
	// HMAC provenance token minted when the proxy rewrites a playlist fetched
	// from one of these allowlisted origins (see provenance.go). megaplay.buzz
	// itself is intentionally absent: getSources is fetched server-side by the
	// scraper, never through this proxy.
	{Domain: "mewstream.buzz", Reason: "megaplay.buzz HLS master/variant playlist origin; seeds provenance tokens for rotating segment CDNs (nineanime revival 2026-06-01)", Owner: "@0neymik0", Added: "2026-06-01"},
	{Domain: "lostproject.club", Reason: "megaplay.buzz subtitle .vtt track host (nineanime revival 2026-06-01)", Owner: "@0neymik0", Added: "2026-06-01"},
}

// HLSProxyAllowedDomains is the flat []string view of
// HLSProxyAllowedDomainsWithProvenance. Existing call sites (the streaming
// HLS proxy, the scraper URL validator, and the various regression-lock
// tests) iterate this slice unchanged.
//
// Do NOT modify this slice directly — edit HLSProxyAllowedDomainsWithProvenance
// and this view is rebuilt automatically. The variable is left mutable for
// backwards compatibility with test code that historically read len() and
// values, but production code should treat it as read-only.
var HLSProxyAllowedDomains = HLSProxyAllowedDomainsList()

// HLSProxyAllowedDomainsList returns the flat []string view of the HLS proxy
// allow-list. Equivalent to the package-level HLSProxyAllowedDomains, but
// guaranteed to be a fresh slice — callers may mutate the result without
// affecting other callers.
//
// New code should prefer HLSProxyAllowedDomainsWithProvenance when provenance
// is relevant (audit tooling, security review); use this function or
// HLSProxyAllowedDomains when only the host strings are needed.
func HLSProxyAllowedDomainsList() []string {
	out := make([]string, len(HLSProxyAllowedDomainsWithProvenance))
	for i, e := range HLSProxyAllowedDomainsWithProvenance {
		out[i] = e.Domain
	}
	return out
}

// UpstreamError represents an error from the upstream CDN.
type UpstreamError struct {
	StatusCode int
	Domain     string
	HTML       bool // true if upstream returned HTML (e.g. Cloudflare challenge)
}

func (e *UpstreamError) Error() string {
	if e.HTML {
		return fmt.Sprintf("upstream returned HTML error page (status %d, domain %s)", e.StatusCode, e.Domain)
	}
	return fmt.Sprintf("upstream error (status %d, domain %s)", e.StatusCode, e.Domain)
}

// DomainNotAllowedError is returned by ProxyWithReferer (and any future
// HLS-proxy entry point) when the parsed URL's host is not in
// HLSProxyAllowedDomains. Streaming handlers should catch this with
// errors.As and emit HTTP 502 — the upstream URL is structurally
// unreachable through our allowlist gate, not a transient/caller error.
type DomainNotAllowedError struct {
	Domain string
}

func (e *DomainNotAllowedError) Error() string {
	return fmt.Sprintf("domain not allowed for HLS proxy: %s", e.Domain)
}

// ProxyWithReferer proxies a stream with a custom Referer header
// This is needed for HLS streams that require specific referer for authentication
func (p *VideoProxy) ProxyWithReferer(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) error {
	// Validate URL
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source url: %w", err)
	}

	// Check if domain is allowed for HLS proxy. A valid provenance token
	// (minted when the proxy rewrote a playlist from an allowlisted origin)
	// authorizes an otherwise-unlisted host — this is how rotating segment
	// CDNs (megaplay/mewstream) are served without a static per-domain entry.
	if !isHLSDomainAllowed(parsed.Host) &&
		!validProvenanceToken(sourceURL, r.URL.Query().Get("exp"), r.URL.Query().Get("sig"), time.Now()) {
		return &DomainNotAllowedError{Domain: parsed.Host}
	}

	// Create upstream request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers for upstream request
	req.Header.Set("User-Agent", p.config.UserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
		// Also set Origin which some CDNs check
		if parsedReferer, err := url.Parse(referer); err == nil {
			req.Header.Set("Origin", parsedReferer.Scheme+"://"+parsedReferer.Host)
		}
	}

	// Handle range requests for seeking
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// Make upstream request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream request: %w", err)
	}
	defer resp.Body.Close()

	// Detect upstream errors (403, 5xx, etc.) — don't forward garbage to HLS.js
	if resp.StatusCode >= 400 {
		// Check if upstream returned HTML instead of video data (e.g. Cloudflare challenge)
		upstreamCT := resp.Header.Get("Content-Type")
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		isHTML := strings.Contains(upstreamCT, "text/html") ||
			strings.Contains(string(body), "<!DOCTYPE") ||
			strings.Contains(string(body), "<html")

		if isHTML {
			return &UpstreamError{StatusCode: resp.StatusCode, Domain: parsed.Host, HTML: true}
		}
		return &UpstreamError{StatusCode: resp.StatusCode, Domain: parsed.Host}
	}

	// Check if this is an M3U8 file that needs URL rewriting
	contentType := resp.Header.Get("Content-Type")
	isM3U8 := strings.Contains(contentType, "mpegurl") ||
		strings.Contains(contentType, "x-mpegurl") ||
		strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8")

	// Set CORS headers for frontend access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

	if isM3U8 && resp.StatusCode == http.StatusOK {
		// Read and rewrite M3U8 content
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read m3u8 body: %w", err)
		}

		// Rewrite URLs in the M3U8
		rewritten := rewriteM3U8URLs(string(body), sourceURL, referer)

		// Set headers (skip Content-Length as it changed)
		for key, values := range resp.Header {
			if key == "Connection" || key == "Keep-Alive" || key == "Transfer-Encoding" || key == "Content-Length" {
				continue
			}
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(rewritten))
		return nil
	}

	// Copy response headers for non-M3U8 content
	for key, values := range resp.Header {
		// Skip hop-by-hop headers and Content-Type (we'll set it ourselves)
		if key == "Connection" || key == "Keep-Alive" || key == "Transfer-Encoding" || key == "Content-Type" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Fix Content-Type — some CDNs return wrong types (like image/jpeg) to obfuscate content.
	// The caller can also force a specific container via ?type=mp4 when the upstream URL
	// has no extension in the path (e.g. AllAnime's fast4speed.rsvp CDN), which would
	// otherwise default to application/octet-stream and trip the HLS-segment rate limiter.
	correctContentType := getCorrectHLSContentType(parsed.Path, resp.Header.Get("Content-Type"))
	switch strings.ToLower(r.URL.Query().Get("type")) {
	case "mp4":
		correctContentType = "video/mp4"
	case "webm":
		correctContentType = "video/webm"
	}
	w.Header().Set("Content-Type", correctContentType)

	// For direct video files (MP4), ensure range request headers are set for seeking
	isDirectVideo := strings.HasPrefix(correctContentType, "video/mp4") ||
		strings.HasPrefix(correctContentType, "video/webm")
	if isDirectVideo {
		w.Header().Set("Accept-Ranges", "bytes")
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Stream the response — rate limit HLS segments (5MB/s) but not direct video files
	if isDirectVideo {
		io.Copy(w, resp.Body)
	} else {
		rateLimitedCopy(w, resp.Body, 5*1024*1024) // 5MB/s for HLS segments
	}

	return nil
}

// rewriteM3U8URLs rewrites URLs in an M3U8 playlist to go through the proxy
// Also fixes unsupported audio codecs (mp4a.40.1 -> mp4a.40.2)
func rewriteM3U8URLs(content, baseURL, referer string) string {
	// Fix unsupported audio codec: AAC Main Profile (mp4a.40.1) -> AAC-LC (mp4a.40.2)
	// Chrome/Edge don't support mp4a.40.1 but the actual audio is usually AAC-LC compatible
	if strings.Contains(content, "mp4a.40.1") {
		content = strings.ReplaceAll(content, "mp4a.40.1", "mp4a.40.2")
	}

	// Parse base URL for resolving relative URLs
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return content
	}

	// Get the base path for relative URLs (directory of the M3U8 file)
	basePath := parsedBase.Scheme + "://" + parsedBase.Host
	if lastSlash := strings.LastIndex(parsedBase.Path, "/"); lastSlash > 0 {
		basePath += parsedBase.Path[:lastSlash+1]
	} else {
		basePath += "/"
	}

	var result strings.Builder
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments (except URI attributes)
		if trimmed == "" || (strings.HasPrefix(trimmed, "#") && !strings.Contains(trimmed, "URI=\"")) {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Handle lines with URI="..." attributes (like #EXT-X-KEY)
		if strings.Contains(trimmed, "URI=\"") {
			rewritten := rewriteURIAttribute(line, basePath, referer)
			result.WriteString(rewritten)
			result.WriteString("\n")
			continue
		}

		// This is a URL line (not a comment)
		if !strings.HasPrefix(trimmed, "#") {
			rewrittenURL := rewriteHLSURL(trimmed, basePath, referer)
			result.WriteString(rewrittenURL)
			result.WriteString("\n")
			continue
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// rewriteHLSURL rewrites a single HLS URL to go through the proxy
func rewriteHLSURL(urlStr, basePath, referer string) string {
	// Skip if already a proxy URL (check both encoded and decoded)
	if strings.Contains(urlStr, "/api/streaming/hls-proxy") ||
		strings.Contains(urlStr, "%2Fapi%2Fstreaming%2Fhls-proxy") ||
		strings.Contains(urlStr, "hls-proxy") {
		return urlStr
	}

	// Already absolute URL
	var absoluteURL string
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		absoluteURL = urlStr
	} else if strings.HasPrefix(urlStr, "/") {
		// Root-relative URL - but skip if it's our proxy path
		if strings.HasPrefix(urlStr, "/api/streaming/hls-proxy") {
			return urlStr
		}
		parsedBase, _ := url.Parse(basePath)
		absoluteURL = parsedBase.Scheme + "://" + parsedBase.Host + urlStr
	} else {
		// Relative URL
		absoluteURL = basePath + urlStr
	}

	// Build proxy URL
	proxyURL := "/api/streaming/hls-proxy?url=" + url.QueryEscape(absoluteURL)
	if referer != "" {
		proxyURL += "&referer=" + url.QueryEscape(referer)
	}

	// Mint a provenance token. This URL was extracted from a playlist the
	// proxy fetched from an allowlisted origin, so it inherits that trust:
	// the segment/variant request may bypass the static host allowlist (see
	// provenance.go) — essential for players whose segment CDN rotates across
	// an unbounded pool of throwaway domains. Harmless for already-allowlisted
	// hosts, which pass the static check regardless.
	exp, sig := signProvenance(absoluteURL, time.Now())
	proxyURL += "&exp=" + exp + "&sig=" + sig

	return proxyURL
}

// rewriteURIAttribute rewrites URI="..." attributes in M3U8 tags
func rewriteURIAttribute(line, basePath, referer string) string {
	// Find URI="..." and rewrite the URL inside
	uriStart := strings.Index(line, "URI=\"")
	if uriStart == -1 {
		return line
	}
	uriStart += 5 // len("URI=\"")

	uriEnd := strings.Index(line[uriStart:], "\"")
	if uriEnd == -1 {
		return line
	}

	originalURI := line[uriStart : uriStart+uriEnd]
	rewrittenURI := rewriteHLSURL(originalURI, basePath, referer)

	return line[:uriStart] + rewrittenURI + line[uriStart+uriEnd:]
}

// getCorrectContentType returns the correct Content-Type for proxied content.
// Some CDNs return incorrect types (like image/jpeg) to obfuscate video content.
func getCorrectHLSContentType(path, upstreamContentType string) string {
	pathLower := strings.ToLower(path)

	// Direct video files (MP4, WebM)
	if strings.HasSuffix(pathLower, ".mp4") || strings.HasSuffix(pathLower, ".m4v") {
		return "video/mp4"
	}
	if strings.HasSuffix(pathLower, ".webm") {
		return "video/webm"
	}

	// M3U8 playlists
	if strings.HasSuffix(pathLower, ".m3u8") {
		return "application/vnd.apple.mpegurl"
	}

	// MPEG-TS segments
	if strings.HasSuffix(pathLower, ".ts") {
		return "video/mp2t"
	}

	// Common segment extensions used by various CDNs
	// Many CDNs use custom extensions or no extension for encrypted segments
	if strings.HasSuffix(pathLower, ".seg") ||
		strings.HasSuffix(pathLower, ".segment") ||
		strings.HasSuffix(pathLower, ".frag") {
		return "video/mp2t"
	}

	// If the upstream says it's an image but the content length suggests video
	// (video segments are typically > 100KB), treat it as video
	if strings.HasPrefix(upstreamContentType, "image/") {
		// Assume it's a video segment - CDNs often lie about Content-Type
		return "video/mp2t"
	}

	// Key files (AES-128 encryption keys are exactly 16 bytes)
	if strings.Contains(pathLower, "key") || strings.Contains(pathLower, "enc") {
		return "application/octet-stream"
	}

	// If upstream already has a valid media type, use it
	if strings.Contains(upstreamContentType, "mpegurl") ||
		strings.Contains(upstreamContentType, "mp2t") ||
		strings.Contains(upstreamContentType, "octet-stream") ||
		strings.Contains(upstreamContentType, "video/") ||
		strings.Contains(upstreamContentType, "audio/") {
		return upstreamContentType
	}

	// Default to octet-stream for unknown types
	return "application/octet-stream"
}

// isHLSDomainAllowed checks if a domain is allowed for HLS proxying
func isHLSDomainAllowed(host string) bool {
	host = strings.ToLower(host)

	// Strip port number if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}

	for _, allowed := range HLSProxyAllowedDomains {
		allowed = strings.ToLower(allowed)
		if strings.HasSuffix(allowed, "*") {
			// Prefix wildcard: "htv-*" matches "htv-belias.com" and "p34.htv-hydaelyn.com"
			prefix := allowed[:len(allowed)-1]
			if strings.HasPrefix(host, prefix) || strings.Contains(host, "."+prefix) {
				return true
			}
		} else if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}

	return false
}

// rateLimitedCopy copies data with a rate limit
func rateLimitedCopy(dst io.Writer, src io.Reader, bytesPerSecond int64) error {
	buf := make([]byte, 32*1024) // 32KB buffer
	var written int64
	startTime := time.Now()

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)

			// Calculate expected time based on bytes written
			expectedDuration := time.Duration(float64(written) / float64(bytesPerSecond) * float64(time.Second))
			elapsed := time.Since(startTime)

			// Sleep if we're ahead of the rate limit
			if expectedDuration > elapsed {
				time.Sleep(expectedDuration - elapsed)
			}

			if ew != nil {
				// Client disconnected, not an error
				if strings.Contains(ew.Error(), "broken pipe") ||
					strings.Contains(ew.Error(), "connection reset") {
					return nil
				}
				return ew
			}
			if nr != nw {
				return io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return nil
			}
			return er
		}
	}
}
