package videoutils

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/videoutils/netguard"
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

	// UpstreamSigner, when set, gets a chance to rewrite each upstream URL
	// just before the proxy fetches it. It returns (signedURL, true) to
	// substitute the fetch URL, or ("", false) to leave it unchanged. This
	// is the seam used to presign self-hosted MinIO reads (private bucket)
	// without making the proxy MinIO-aware: only URLs the signer claims are
	// rewritten, so every external-CDN path is untouched. The ORIGINAL URL
	// is still used for allow-list checks, M3U8 rewriting, and provenance —
	// the signer only affects the actual outbound GET. Not serialized.
	UpstreamSigner func(rawURL string) (string, bool) `json:"-" yaml:"-"`

	// FirstPartyHosts are internal service hosts the proxy legitimately reaches
	// over the Docker network (MinIO object store, the stealth-scraper sidecar).
	// They resolve to private IPs, so the dial-time SSRF guard (finding #64/#65)
	// exempts EXACTLY these hosts; every other host is blocked from dialing a
	// private/loopback/link-local address. Match is exact (case-insensitive).
	FirstPartyHosts []string `json:"first_party_hosts" yaml:"first_party_hosts"`

	// SolodcdnEdges is the sibling-edge pool for Kodik/solodcdn edge rotation
	// (Layer A of the playback self-healing design, AUTO-562). When a
	// p<N>.solodcdn.com edge answers >=500 for a path, the HLS proxy re-fetches
	// the identical path on these sibling edges (skipping the one that failed,
	// capped at maxSolodcdnRotations). Empty => the built-in default
	// (defaultSolodcdnEdges). The streaming service populates this from
	// STREAMING_SOLODCDN_EDGES.
	SolodcdnEdges []string `json:"solodcdn_edges" yaml:"solodcdn_edges"`

	// OnEdgeRotation, when set, is invoked once per attempted solodcdn edge
	// rotation with (fromEdge, toEdge, outcome) where outcome is one of
	// "success" (sibling served <400), "fail" (sibling also >=400), or "error"
	// (transport error reaching the sibling). It is the observability seam the
	// streaming service uses to emit proxy_edge_rotations_total WITHOUT this
	// shared lib taking a Prometheus dependency — mirrors UpstreamSigner. Not
	// serialized.
	OnEdgeRotation func(from, to, outcome string) `json:"-" yaml:"-"`

	// OnEdgeAttempt, when set, is invoked once per upstream attempt in a
	// solodcdn failover sequence — INCLUDING the nominal (first) attempt — with
	// (edge, outcome, elapsedMs). outcome is one of "ok" (<400), "http4xx",
	// "http5xx", "dial_error" (hard transport error), or "timeout" (response-
	// header window elapsed). Feeds proxy_edge_attempt_seconds. Not serialized.
	OnEdgeAttempt func(edge, outcome string, ms int64) `json:"-" yaml:"-"`

	// OnEdgeServed, when set, is invoked once with the edge that ultimately
	// served the returned (<400) response. Feeds proxy_edge_selected_total. Not
	// serialized.
	OnEdgeServed func(edge string) `json:"-" yaml:"-"`
}

// fetchURLFor returns the URL the proxy should actually GET for sourceURL:
// the UpstreamSigner's rewrite when it claims the URL, otherwise sourceURL
// unchanged. Allow-list / rewrite / provenance logic always uses the
// original sourceURL — only the outbound request target changes.
func (p *VideoProxy) fetchURLFor(sourceURL string) string {
	if p.config.UpstreamSigner != nil {
		if signed, ok := p.config.UpstreamSigner(sourceURL); ok {
			return signed
		}
	}
	return sourceURL
}

// DefaultProxyConfig returns sensible defaults
func DefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		// Full Firefox UA. okcdn (ok.ru CDN, used by the okru provider) bakes the
		// resolving browser's engine into the stream URL (srcAg/GECKO) and rejects
		// a mismatched fetch UA with 400 — so the HLS proxy must identify as a real
		// browser, not the old truncated "...AppleWebKit/537.36" string (no engine
		// token => 400). Other CDNs ignore the UA, so this is safe globally. Must
		// stay a Gecko/Firefox UA to match the okru extractor's resolve UA (okruUA).
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0",
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
	firstParty := cfg.FirstPartyHosts
	return &VideoProxy{
		client: &http.Client{
			// No timeout on client level — context cancellation handles timeouts.
			// A global timeout breaks streaming of large MP4 files (100s of MB).
			Transport: newIPv4Transport(firstParty),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Follow redirects but preserve headers
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				// Re-validate each redirect hop (finding #64): a redirect target
				// whose scheme is not http/https or whose IP-literal host is
				// private/loopback/link-local is rejected up front. (The dial-time
				// guard below is the authoritative, rebind-safe layer; this gives a
				// fast, explicit failure for the obvious cases.) First-party hosts
				// are exempt so an internal 30x still works.
				if !allowLoopbackForTest && !firstPartyAddr(req.URL.Host, firstParty) {
					if err := netguard.ValidatePublicURL(req.URL.String()); err != nil {
						return fmt.Errorf("redirect blocked: %w", err)
					}
				}
				return nil
			},
		},
		config: cfg,
	}
}

// allowLoopbackForTest is a TEST-ONLY seam. When true, the SSRF guards (the
// provenance URL check and the dial-time private-IP block) are relaxed so unit
// tests can sign and fetch httptest fixtures on 127.0.0.1. Never set in
// production. Mirrors libs/streamprobe's allowLoopbackForTests convention.
var allowLoopbackForTest bool

// firstPartyAddr reports whether the dial addr (host[:port]) targets a
// configured first-party internal host — matched EXACTLY (case-insensitive,
// port/trailing-dot tolerant). These hosts are the only ones permitted to dial
// a private IP; the match is exact (not subdomain) so a "minio.evil.com" or
// "api.minio" cannot borrow the exemption.
func firstPartyAddr(addr string, firstParty []string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	for _, fp := range firstParty {
		if host == strings.ToLower(strings.TrimSpace(fp)) {
			return true
		}
	}
	return false
}

// newIPv4Transport returns an http.Transport whose dialer forces IPv4 ("tcp4")
// and guards against SSRF to private addresses (finding #64/#65).
//
// IPv4: several upstream CDNs (e.g. AnimePahe/Kwik's vault-*.owocdn.top edges)
// are dual-stack and return AAAA records, but our containers have no working
// IPv6 egress. Go's default dual-stack dialer intermittently races the IPv6
// address, which black-holes (no RST/ICMP) and stalls each connection until the
// upstream timeout fires — manifests/keys are tiny and usually slip through on
// the IPv4 fallback, but the steady flood of HLS segments keeps landing on the
// dead IPv6 path, so episodes load but never play. Forcing "tcp4" removes the
// IPv6 attempt entirely and makes segment fetches deterministic.
//
// SSRF: every non-first-party dial runs through netguard.DenyPrivateControl,
// which rejects connections whose POST-DNS address is private/loopback/
// link-local — closing DNS-rebind and redirect-to-internal bypasses for hosts
// pulled from an upstream playlist. The configured firstParty hosts (MinIO,
// stealth-scraper) legitimately resolve to Docker-private IPs, so they use a
// plain dialer and skip the guard.
func newIPv4Transport(firstParty []string) *http.Transport {
	plain := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	guarded := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: netguard.DenyPrivateControl}
	t := http.DefaultTransport.(*http.Transport).Clone()
	// Raise the per-host idle keep-alive pool from Go's default of 2 (finding
	// L417): the hottest path is a steady flood of HLS segment GETs to ONE CDN
	// host, and a 2-conn idle cap forces a re-dial + re-TLS on every request
	// past the first two idle conns. 64 lets segments reuse keep-alive conns;
	// the global MaxIdleConns=100 ceiling (Clone default) is left intact.
	t.MaxIdleConnsPerHost = 64
	// Bound the time-to-first-header phase (finding L781): an upstream that
	// completes TCP+TLS but never sends response headers would otherwise pin a
	// proxy slot indefinitely (streaming caps concurrency at 50 → pool
	// exhaustion → 503). This caps only the header wait, NOT the body stream —
	// the body still flows under the request context (cancelled on client
	// disconnect), so large MP4s are unaffected. 45s is deliberately generous:
	// a Kodik solodcdn edge (and some other CDNs) can be slow to first byte
	// while it cold-starts / prepares the HLS on demand — we'd rather WAIT for
	// it to answer (200 or an honest 5xx that trips edge failover) than abandon
	// a slow-but-alive edge prematurely. Only a genuinely hung edge burns the
	// full window before its timeout error rotates to a sibling.
	t.ResponseHeaderTimeout = 45 * time.Second
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Coerce any TCP dial to IPv4-only.
		switch network {
		case "tcp", "tcp6":
			network = "tcp4"
		}
		if allowLoopbackForTest || firstPartyAddr(addr, firstParty) {
			return plain.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
	return t
}

// countReader wraps an io.Reader and atomically tallies the number of bytes
// read through it. It is the bytes_in (upstream ingress) counter for the
// egress register: wrapping resp.Body BEFORE io.Copy/rateLimitedCopy lets the
// proxy count exactly the bytes pulled from upstream WITHOUT buffering the
// whole body (D-05 — never io.ReadAll the stream path). The atomic counter is
// safe to read concurrently while the copy is in flight (T-02-LOCK: no lock is
// held across the copy).
type countReader struct {
	r io.Reader
	n *uint64
}

func (c *countReader) Read(p []byte) (int, error) {
	nr, err := c.r.Read(p)
	if nr > 0 {
		atomic.AddUint64(c.n, uint64(nr))
	}
	return nr, err
}

// newSessToken mints a fresh per-manifest correlation id. It is crypto/rand
// derived (NOT from user_id/PII — Security V3 / T-02-PII / T-02-SESS) and
// carries NO authority: it only groups a manifest's segment GETs into one
// aggregated egress row (AR-EGRESS-04). On the (vanishingly unlikely) rand
// failure it returns "" — the caller simply omits the param and that
// manifest's segments fall back to per-request (unaggregated) accounting.
func newSessToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// ProxyStream proxies a video stream from an external URL
func (p *VideoProxy) ProxyStream(ctx context.Context, sourceURL string, w http.ResponseWriter, r *http.Request) error {
	_, _, err := p.ProxyStreamCounted(ctx, sourceURL, w, r)
	return err
}

// ProxyStreamCounted is ProxyStream that additionally reports the bytes_in
// (upstream resp.Body) and bytes_out (client sink) counts for this call so the
// streaming handler can fold them into the per-session egress tally
// (AR-EGRESS-05). bytes_out is the io.Copy return; bytes_in is the countReader
// total. Both are zero on early-return errors before the copy.
func (p *VideoProxy) ProxyStreamCounted(ctx context.Context, sourceURL string, w http.ResponseWriter, r *http.Request) (bytesIn, bytesOut uint64, _ error) {
	// Validate URL is from allowed domain
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid source url: %w", err)
	}

	if !p.isDomainAllowed(parsed.Host) {
		return 0, 0, fmt.Errorf("domain not allowed: %s", parsed.Host)
	}

	// Create upstream request (fetch URL may be a presigned rewrite of
	// sourceURL for self-hosted MinIO; sourceURL itself is unchanged).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.fetchURLFor(sourceURL), nil)
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
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
		return 0, 0, fmt.Errorf("upstream request: %w", err)
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

	// Stream the response, counting upstream bytes_in (countReader) and
	// client bytes_out (io.Copy return) — no buffering (D-05).
	src := &countReader{r: resp.Body, n: &bytesIn}
	written, err := io.Copy(w, src)
	bytesOut = uint64(written)
	if err != nil {
		// Client disconnected, not an error
		if strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "connection reset") {
			return bytesIn, bytesOut, nil
		}
		return bytesIn, bytesOut, fmt.Errorf("stream copy: %w", err)
	}

	return bytesIn, bytesOut, nil
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

// matchHLSDomain reports whether host matches a single allow-list pattern.
// host MUST already be lower-cased (and port-stripped if the caller strips
// ports). Supported pattern forms:
//
//   - "example.com"   exact host OR any subdomain (host == p || host ends ".p")
//   - "*.example.com" any subdomain of example.com
//   - "htv-*.com"     prefix wildcard ANCHORED to a domain suffix: some DNS
//     label begins with "htv-" AND the host ends with ".com". The suffix anchor
//     is the fix for the SSRF allow-list bypass — the old unanchored "htv-*"
//     matched ANY host starting with the prefix on ANY TLD (htv-evil.attacker.io
//     passed), letting an attacker register a matching domain. Anchoring to the
//     family's real TLD rejects every off-TLD lookalike.
//   - "htv-*"         bare prefix wildcard (no suffix anchor) — legacy form,
//     still honored for backward compatibility but discouraged; prefer a suffix.
//
// NOTE (residual, tracked): a prefix wildcard anchored to a registrable TLD
// (.com/.top) still admits an attacker-registered same-TLD lookalike
// (htv-evil.com). Fully closing that needs a dial-time private-IP SSRF guard
// (block loopback/private/link-local/metadata targets) that is allow-list-aware
// so it does not break the internal MinIO fetch path — see the audit follow-up.
func matchHLSDomain(host, pattern string) bool {
	pattern = strings.ToLower(pattern)
	switch {
	case strings.HasPrefix(pattern, "*."):
		return strings.HasSuffix(host, pattern[1:]) // ".example.com"
	case strings.Contains(pattern, "*"):
		star := strings.IndexByte(pattern, '*')
		prefix, suffix := pattern[:star], pattern[star+1:]
		if suffix != "" && !strings.HasSuffix(host, suffix) {
			return false
		}
		// Prefix must begin a DNS label: leftmost label, or right after a dot.
		return strings.HasPrefix(host, prefix) || strings.Contains(host, "."+prefix)
	default:
		return host == pattern || strings.HasSuffix(host, "."+pattern)
	}
}

// isDomainAllowed checks if a domain is in the allowed list.
// Returns false when AllowedDomains is empty (fail-closed).
func (p *VideoProxy) isDomainAllowed(host string) bool {
	if len(p.config.AllowedDomains) == 0 {
		return false
	}

	host = strings.ToLower(host)
	for _, allowed := range p.config.AllowedDomains {
		if matchHLSDomain(host, allowed) {
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

// HLSProxyAllowedDomains is the static host allow-list for the HLS proxy's
// third-party-domain gate. It is the FALLBACK trust path — the primary
// mechanism is signed-URL provenance (see provenance.go): catalog signs every
// scraper-resolved stream/subtitle URL (services/catalog/internal/streamsign),
// and the proxy mints fresh HMAC tokens for child playlist/segment URLs it
// rewrites, so scraper CDNs — including unbounded rotating segment hosts —
// need NO entry here.
//
// Only two kinds of hosts belong on this list:
//
//  1. First-party infrastructure reached by hostname over the docker network
//     (stealth-scraper sidecar, MinIO).
//  2. Hosts emitted by catalog endpoints that do NOT sign their URLs yet
//     (Kodik ad-free, Hanime, AnimeLib, 18anime, subtitle files).
//
// Before adding an entry, sign at the source instead (streamsign.Sign — see
// the animejoy endpoints for the pattern).
var HLSProxyAllowedDomains = []string{
	// First-party docker-network hosts. stealth-scraper's /hls restreams the
	// real rotating CDN inside its Camoufox context for engine=browser
	// providers (the upstream CDN itself stays unlisted); MinIO holds the
	// library/raw-provider HLS segments (port-stripped "minio" matches
	// minio:9000). Both also appear in FirstPartyHosts for the SSRF dial
	// guard — this gate runs BEFORE presigning/dialing, so they must be
	// listed here too.
	"stealth-scraper",
	"minio",

	// Kodik ad-free HLS (kodikextract; GetKodikStreamSource returns the URL
	// unsigned). Manifest on cloud.solodcdn.com 302-redirects to node hosts
	// (draco.cloud.solodcdn.com, ...); the eTLD+1 entry covers those via the
	// HasSuffix(host, "."+allowed) match.
	"solodcdn.com",
	"cloud.solodcdn.com",

	// Hanime video CDN family (GetHanimeStream returns URLs unsigned).
	// Wildcards are anchored to the family's real TLD to block off-TLD
	// lookalikes — see matchHLSDomain.
	"hanime.tv",
	"highwinds-cdn.com",
	"htv-*.com",      // htv-belias.com, htv-hydaelyn.com, ...
	"hydaelyn-*.top", // hydaelyn-25x-00.top through 19.top
	"zodiark-*.top",  // zodiark-25x-00.top through 09.top

	// AnimeLib video CDNs (GetAnimeLibStream returns URLs unsigned).
	"cdnlibs.org",
	"hentaicdn.org",

	// 18anime resolved stream hosts (catalog's Get18AnimeStream re-packages
	// the scraper body WITHOUT the provenance signature, so these stay until
	// that path signs). NOT 18anime.me itself — these are the embed mirrors.
	"mp4upload.com",    // progressive MP4 (aN.mp4upload.com:183), requires Referer https://www.mp4upload.com/
	"turboviplay.com",  // turbovid master m3u8 host (cdnN.turboviplay.com)
	"turbosplayer.com", // turbovid nested variant/segment host

	// Japanese subtitle files (subtitle endpoints return URLs unsigned).
	"jimaku.cc",

	// AUTO-517 stop-gap: Miruro vidtube inner-embed CDN. The (signed)
	// ultracloud.cc master/variant playlists 302-redirect here, and the
	// redirect target is re-gated without a token. Remove once the proxy
	// mints provenance across the redirect chain.
	"mt.nekostream.site",
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

// solodcdnEdgeRe matches the rotating Kodik ad-free edge hosts
// p<N>.solodcdn.com (p12, p13, p14, ...). ONLY these hosts are eligible for
// edge rotation — the anchored pattern is what fences AUTO-562's retry to the
// solodcdn family and keeps every other CDN path a no-op.
var solodcdnEdgeRe = regexp.MustCompile(`^p\d+\.solodcdn\.com$`)

// defaultSolodcdnEdges is the fallback edge pool used when ProxyConfig.SolodcdnEdges
// is empty (STREAMING_SOLODCDN_EDGES unset). Treat as read-only.
var defaultSolodcdnEdges = []string{"p12", "p13", "p14"}

// maxSolodcdnRotations caps how many sibling edges we try after the first 5xx.
const maxSolodcdnRotations = 2

// edgesOrDefault returns the configured edge pool, or the built-in default when
// none is configured.
func edgesOrDefault(edges []string) []string {
	if len(edges) == 0 {
		return defaultSolodcdnEdges
	}
	return edges
}

// edgeAttempt records one upstream attempt in a solodcdn edge-failover sequence,
// for the X-AE-Edge-Trail response header and per-attempt metrics.
type edgeAttempt struct {
	edge    string // "p13"; "" for a non-solodcdn single attempt
	outcome string // ok | http4xx | http5xx | dial_error | timeout
	ms      int64
}

// edgeFailover carries fetchWithEdgeFailover's result beyond the *http.Response:
// which edge produced the returned response (served), and the ordered trail of
// every attempt made.
type edgeFailover struct {
	served string // edge of the returned response; "" for a non-solodcdn fetch
	trail  []edgeAttempt
}

// trailString renders the attempt trail as a compact "edge:outcome:ms" CSV for
// the X-AE-Edge-Trail header, e.g. "p13:timeout:45003,p12:ok:210" — the LOGIC
// and METRICS behind edge selection, not just the final decision.
func (ef edgeFailover) trailString() string {
	parts := make([]string, 0, len(ef.trail))
	for _, a := range ef.trail {
		parts = append(parts, a.edge+":"+a.outcome+":"+strconv.FormatInt(a.ms, 10))
	}
	return strings.Join(parts, ",")
}

// classifyEdgeOutcome maps a single fetch result to a trail/metric outcome token.
// A response-header timeout surfaces as a net.Error whose Timeout() is true; any
// other transport error (dial refused/reset/DNS) is "dial_error". Both trigger
// edge rotation — the distinction is only for observability.
func classifyEdgeOutcome(resp *http.Response, err error) string {
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return "timeout"
		}
		return "dial_error"
	}
	switch {
	case resp.StatusCode >= 500:
		return "http5xx"
	case resp.StatusCode >= 400:
		return "http4xx"
	default:
		return "ok"
	}
}

// rotationOutcome maps a sibling attempt's fine-grained outcome to the coarser
// proxy_edge_rotations_total vocabulary (success|fail|error) the existing
// dashboard expects: a transport-level failure is "error", a <400 answer is
// "success", and any >=400 (authoritative 4xx OR still-5xx) is "fail".
func rotationOutcome(o string) string {
	switch o {
	case "dial_error", "timeout":
		return "error"
	case "ok":
		return "success"
	default: // http4xx, http5xx
		return "fail"
	}
}

// solodcdnEdgeOf returns the "p<N>" label for a p<N>.solodcdn.com host, or ""
// when host is not a solodcdn edge (port stripped before matching).
func solodcdnEdgeOf(rawHost string) string {
	host := strings.ToLower(rawHost)
	if colon := strings.LastIndex(host, ":"); colon != -1 {
		host = host[:colon]
	}
	if !solodcdnEdgeRe.MatchString(host) {
		return ""
	}
	return host[:strings.IndexByte(host, '.')] // "p12.solodcdn.com" -> "p12"
}

// fetchWithEdgeFailover implements Layer A of the playback self-healing design
// (docs/superpowers/specs/2026-07-10-kodik-edge-failover-design.md; extends
// AUTO-562): it issues the request for sourceURL via do and, when the host is a
// p<N>.solodcdn.com edge, transparently fails over across sibling edges on a
// hard transport error, a response-header timeout, OR a >=500 response. The
// nominal edge is tried FIRST; siblings (the configured pool minus the nominal)
// follow, capped at maxSolodcdnRotations. A <400 answer (including an
// authoritative 4xx) stops the sequence.
//
// For any non-solodcdn host it performs a SINGLE attempt and returns its
// (resp,err) unchanged — every EN/Raw/Hanime/ae path through this shared lib is
// byte-for-byte untouched. It returns the served response (which may be a >=500
// fallback when every edge answered >=500), an edgeFailover with the served-edge
// and full trail, and an error only when EVERY attempt failed at the transport
// layer (no live response to hand back). Each attempt fires OnEdgeAttempt; each
// sibling rotation fires OnEdgeRotation. drainClose reclaims superseded bodies.
func (p *VideoProxy) fetchWithEdgeFailover(sourceURL string, do func(fetchURL string) (*http.Response, error)) (*http.Response, edgeFailover, error) {
	var ef edgeFailover

	// Fast path — the universal per-segment hot path (every EN/Raw/Hanime/ae/
	// non-edge Kodik fetch): one plain attempt, no url.Parse, no regex, no trail,
	// no metrics. The cheap substring gate keeps the parse+regex off the ~all-
	// segments-succeed path so the failover machinery only touches actual
	// p<N>.solodcdn.com edges. cloud.solodcdn.com (unsigned manifest host, not a
	// rotatable edge) also takes this path via the fromEdge == "" guard below.
	if !strings.Contains(sourceURL, ".solodcdn.com") {
		resp, err := do(sourceURL)
		return resp, ef, err
	}
	parsed, perr := url.Parse(sourceURL)
	fromEdge := ""
	if perr == nil {
		fromEdge = solodcdnEdgeOf(parsed.Host)
	}
	if fromEdge == "" {
		resp, err := do(sourceURL)
		return resp, ef, err
	}

	// timed runs one edge attempt, records it into the trail, fires OnEdgeAttempt,
	// and returns the classified outcome (reused by the rotation hook below).
	timed := func(fetchURL, edge string) (*http.Response, error, string) {
		start := time.Now()
		resp, err := do(fetchURL)
		ms := time.Since(start).Milliseconds()
		outcome := classifyEdgeOutcome(resp, err)
		ef.trail = append(ef.trail, edgeAttempt{edge: edge, outcome: outcome, ms: ms})
		if p.config.OnEdgeAttempt != nil {
			p.config.OnEdgeAttempt(edge, outcome, ms)
		}
		return resp, err, outcome
	}

	// Nominal (first) attempt on the edge the URL already carries.
	resp, err, _ := timed(sourceURL, fromEdge)
	if err == nil && resp.StatusCode < 500 {
		ef.served = fromEdge // <400 or authoritative 4xx — serve it, no rotation
		return resp, ef, nil
	}

	// Nominal failed (>=500 OR transport error/timeout) → rotate to siblings.
	// currentEdge is only read when current != nil, at which point it always holds
	// a real edge (fromEdge on a live nominal >=500, or the adopted sibling below).
	current, currentErr, currentEdge := resp, err, fromEdge
	rotations := 0
	for _, edge := range edgesOrDefault(p.config.SolodcdnEdges) {
		if rotations >= maxSolodcdnRotations {
			break
		}
		if edge == fromEdge {
			continue // never retry the edge that just failed
		}
		rotations++

		sibling := *parsed
		sibling.Host = edge + ".solodcdn.com"
		sResp, sErr, sOutcome := timed(sibling.String(), edge)
		p.reportEdgeRotation(fromEdge, edge, rotationOutcome(sOutcome))

		if sErr != nil {
			// Transport error reaching the sibling: keep whatever live response we
			// have (if any) and try the next edge.
			if current == nil {
				currentErr = sErr
			}
			continue
		}
		if sResp.StatusCode < 500 {
			// Healed (<400) or authoritative 4xx — adopt and stop.
			if current != nil {
				drainClose(current.Body)
			}
			ef.served = edge
			return sResp, ef, nil
		}
		// Sibling is also >=500: adopt as the live fallback and keep rotating.
		if current != nil {
			drainClose(current.Body)
		}
		current, currentErr, currentEdge = sResp, nil, edge
	}

	// Exhausted: return the last live response (a >=500 the caller turns into a
	// 502) or, if we never got one, the last transport error.
	if current != nil {
		ef.served = currentEdge
		return current, ef, nil
	}
	return nil, ef, currentErr
}

// reportEdgeRotation fires the OnEdgeRotation observability hook when configured.
func (p *VideoProxy) reportEdgeRotation(from, to, outcome string) {
	if p.config.OnEdgeRotation != nil {
		p.config.OnEdgeRotation(from, to, outcome)
	}
}

// drainClose drains a small bounded prefix of a superseded upstream body before
// closing it, so the underlying keep-alive connection can be returned to the
// pool instead of being torn down.
func drainClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 8*1024))
	_ = body.Close()
}

// ProxyWithReferer proxies a stream with a custom Referer header
// This is needed for HLS streams that require specific referer for authentication
func (p *VideoProxy) ProxyWithReferer(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) error {
	_, _, err := p.ProxyWithRefererCounted(ctx, sourceURL, referer, w, r)
	return err
}

// ProxyWithRefererCounted is ProxyWithReferer that additionally reports the
// per-call bytes_in (upstream resp.Body) and bytes_out (client sink) so the
// streaming handler can Observe(...) them into the per-session HLS tally
// (AR-EGRESS-04/05). bytes_in is counted via a countReader wrapping resp.Body
// before io.Copy/rateLimitedCopy; bytes_out is the copy's write total. For an
// M3U8 rewrite (the manifest path) the counts reflect the rewritten payload.
func (p *VideoProxy) ProxyWithRefererCounted(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) (uint64, uint64, error) {
	return p.proxyRefererCounted(ctx, sourceURL, referer, false, w, r)
}

// ProxyPreauthCounted is ProxyWithRefererCounted for an upstream URL that was
// already authorized by decoding a sealed stream token (streamtoken.go). The
// static-allowlist / provenance-signature gate is skipped — the AES-GCM token
// WAS the authorization. Everything else (SSRF dial guard, edge failover,
// m3u8 rewriting, byte counting) is identical.
func (p *VideoProxy) ProxyPreauthCounted(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) (uint64, uint64, error) {
	return p.proxyRefererCounted(ctx, sourceURL, referer, true, w, r)
}

// proxyRefererCounted is the shared pipeline behind ProxyWithRefererCounted and ProxyPreauthCounted.
func (p *VideoProxy) proxyRefererCounted(ctx context.Context, sourceURL, referer string, preauth bool, w http.ResponseWriter, r *http.Request) (bytesIn, bytesOut uint64, _ error) {
	// Validate URL
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid source url: %w", err)
	}

	// Check if domain is allowed for HLS proxy. A valid provenance token
	// (minted when the proxy rewrote a playlist from an allowlisted origin)
	// authorizes an otherwise-unlisted host — this is how rotating segment
	// CDNs (megaplay/mewstream) are served without a static per-domain entry.
	// preauth=true skips the gate entirely: the caller already authorized the
	// URL by opening a sealed AES-GCM stream token (streamtoken.go), which can
	// only be minted server-side for SSRF-vetted URLs.
	if !preauth && !isHLSDomainAllowed(parsed.Host) &&
		!validProvenanceToken(sourceURL, r.URL.Query().Get("exp"), r.URL.Query().Get("sig"), time.Now()) {
		return 0, 0, &DomainNotAllowedError{Domain: parsed.Host}
	}

	// Build an upstream GET for a given fetch URL, applying the same UA/Referer/
	// Origin/Range headers. Factored out so solodcdn edge rotation (below) can
	// re-issue the identical request against a sibling edge host. The fetch URL
	// may be a presigned rewrite of the source for self-hosted MinIO; the
	// original sourceURL stays the base for M3U8 rewriting + provenance below.
	doUpstream := func(fetchURL string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
		if err != nil {
			return nil, err
		}
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
		return p.client.Do(req)
	}

	// Make the upstream request through solodcdn edge failover (Layer A of the
	// playback self-healing design; extends AUTO-562). For a p<N>.solodcdn.com
	// edge this rotates to a sibling on a >=500, a hard transport error, OR a
	// response-header timeout — "try another p straight ahead". No-op for every
	// non-solodcdn host, so EN/Raw/Hanime/ae paths through this shared lib are
	// untouched. edgeInfo carries the served-edge + attempt trail for telemetry.
	resp, edgeInfo, err := p.fetchWithEdgeFailover(sourceURL, func(fetchURL string) (*http.Response, error) {
		return doUpstream(p.fetchURLFor(fetchURL))
	})
	if err != nil {
		return 0, 0, fmt.Errorf("upstream request: %w", err)
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
			return 0, 0, &UpstreamError{StatusCode: resp.StatusCode, Domain: parsed.Host, HTML: true}
		}
		return 0, 0, &UpstreamError{StatusCode: resp.StatusCode, Domain: parsed.Host}
	}

	// Check if this is an M3U8 file that needs URL rewriting. Some CDNs (e.g.
	// okcdn.ru) serve path-style variant playlists with no .m3u8 suffix — the
	// mixed-case "application/x-mpegURL" Content-Type is the only signal, so
	// this match must be case-insensitive or those playlists' relative
	// segment URIs never get rewritten to go through the proxy.
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	isM3U8 := strings.Contains(contentType, "mpegurl") ||
		strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8")

	// Check if this is a WebVTT storyboard/thumbnail track — its cue payloads
	// reference sprite-sheet JPEGs by bare relative name on the same private
	// host the manifest itself came from, so they need the same proxy+sign
	// rewrite the M3U8 branch above gives playlist children.
	isVTT := strings.Contains(contentType, "vtt") ||
		strings.HasSuffix(strings.ToLower(parsed.Path), ".vtt")

	// Set CORS headers for frontend access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges, X-AE-Edge-Served, X-AE-Edge-Trail")

	// Surface which solodcdn edge actually served + the full attempt trail, so
	// the aePlayer hacker-mode HUD (and any debugging) sees the METRICS + LOGIC
	// behind edge selection, not just the final decision. Empty for every
	// non-solodcdn source (edgeInfo.served == ""), so other providers are
	// header-for-header unchanged. Exposed above so cross-origin JS can read it.
	if edgeInfo.served != "" {
		w.Header().Set("X-AE-Edge-Served", edgeInfo.served)
		w.Header().Set("X-AE-Edge-Trail", edgeInfo.trailString())
		if p.config.OnEdgeServed != nil {
			p.config.OnEdgeServed(edgeInfo.served)
		}
	}

	if isM3U8 && resp.StatusCode == http.StatusOK {
		// Read and rewrite M3U8 content. This is a small manifest (not a
		// segment stream) so a bounded ReadAll is acceptable here — the D-05
		// "never ReadAll the stream path" rule applies to segment bodies, which
		// take the io.Copy/rateLimitedCopy path below.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, 0, fmt.Errorf("read m3u8 body: %w", err)
		}
		bytesIn = uint64(len(body))

		// Rewrite URLs in the M3U8
		rewritten := rewriteM3U8URLs(string(body), sourceURL, referer)

		bytesOut = writeRewrittenText(w, resp, rewritten)
		return bytesIn, bytesOut, nil
	}

	if isVTT && resp.StatusCode == http.StatusOK {
		// Read and rewrite the VTT content. Same rationale as the M3U8 branch
		// above: a small cue-sheet payload, not a segment stream, so a bounded
		// ReadAll is fine here.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, 0, fmt.Errorf("read vtt body: %w", err)
		}
		bytesIn = uint64(len(body))

		// Rewrite storyboard image cue URLs in the VTT. rewriteVTTURLs mints
		// its own per-manifest correlation token (AR-EGRESS-04) internally,
		// same as rewriteM3U8URLs does for playlist children — every sheet
		// image fetched from this storyboard track groups into one
		// aggregated egress row.
		rewritten := rewriteVTTURLs(string(body), sourceURL, referer)

		bytesOut = writeRewrittenText(w, resp, rewritten)
		return bytesIn, bytesOut, nil
	}

	// Copy response headers for non-M3U8/VTT content
	for key, values := range resp.Header {
		// Skip hop-by-hop headers, Content-Type (we set it ourselves), and the
		// CORS headers we set above — an upstream Access-Control-Allow-* would
		// otherwise duplicate them and browsers reject a multi-value ACAO as a
		// CORS failure (notably the stealth-scraper /hls sidecar always sends
		// Access-Control-Allow-Origin: *).
		if key == "Connection" || key == "Keep-Alive" || key == "Transfer-Encoding" || key == "Content-Type" || isProxySetCORSHeader(key) {
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
	correctContentType := getCorrectHLSContentType(parsed.Path, resp.Header.Get("Content-Type"),
		firstPartyAddr(parsed.Host, p.config.FirstPartyHosts))
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

	// Stream the response — rate limit HLS segments (5MB/s) but not direct
	// video files. Wrap resp.Body in a countReader (bytes_in) so the upstream
	// ingress is tallied WITHOUT buffering (D-05); the copy writes everything
	// it reads to the client sink, so bytes_out == bytes_in on success here
	// (the handler's CountingResponseWriter is the authoritative client-egress
	// counter, but ProxyWithRefererCounted reports its own copy total).
	src := &countReader{r: resp.Body, n: &bytesIn}
	if isDirectVideo {
		written, _ := io.Copy(w, src)
		bytesOut = uint64(written)
	} else {
		_ = rateLimitedCopy(w, src, 5*1024*1024) // 5MB/s for HLS segments
		bytesOut = bytesIn
	}

	return bytesIn, bytesOut, nil
}

// isProxySetCORSHeader reports whether key is a CORS header this proxy sets
// itself. Such headers must NOT be copied from the upstream response: if the
// upstream also emits them (the stealth-scraper /hls sidecar always sends
// Access-Control-Allow-Origin: *), the client receives two values and the
// Fetch spec treats a multi-value Access-Control-Allow-Origin as a CORS
// failure — silently breaking playback under the cross-origin stream.* base.
func isProxySetCORSHeader(key string) bool {
	switch key {
	case "Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Expose-Headers":
		return true
	}
	return false
}

// writeRewrittenText writes a rewritten manifest/cue-sheet payload (M3U8 or
// VTT) to w: it copies resp's headers (skipping Connection/Keep-Alive/
// Transfer-Encoding/Content-Length — the length changed — and the CORS
// headers the proxy already set itself, since an upstream that also sends
// Access-Control-Allow-* would otherwise duplicate them and a response with
// two ACAO values is rejected by browsers as a CORS error), sets the new
// Content-Length, writes the upstream status code, then writes the
// rewritten body. Returns the number of bytes written (bytesOut).
func writeRewrittenText(w http.ResponseWriter, resp *http.Response, rewritten string) uint64 {
	for key, values := range resp.Header {
		if key == "Connection" || key == "Keep-Alive" || key == "Transfer-Encoding" || key == "Content-Length" || isProxySetCORSHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
	w.WriteHeader(resp.StatusCode)
	n, _ := w.Write([]byte(rewritten))
	return uint64(n)
}

// manifestDirBase derives the directory base (scheme://host/dir/) a manifest's
// relative child URLs resolve against — the directory the manifest itself
// lives in, NOT the manifest URL. rewriteHLSURL's basePath argument resolves
// relative URLs by plain concatenation (basePath + urlStr), so passing the
// full manifest URL here would produce a mangled result (e.g.
// ".../playlist.m3u8seg-1.ts"). Shared by rewriteM3U8URLs (playlist children)
// and rewriteVTTURLs (storyboard cue sheets). Returns "" if manifestURL fails
// to parse — callers treat that as "leave content unrewritten", mirroring the
// original inline rewriteM3U8URLs behavior.
func manifestDirBase(manifestURL string) string {
	parsedBase, err := url.Parse(manifestURL)
	if err != nil {
		return ""
	}
	basePath := parsedBase.Scheme + "://" + parsedBase.Host
	if lastSlash := strings.LastIndex(parsedBase.Path, "/"); lastSlash > 0 {
		basePath += parsedBase.Path[:lastSlash+1]
	} else {
		basePath += "/"
	}
	return basePath
}

// rewriteM3U8URLs rewrites URLs in an M3U8 playlist to go through the proxy
// Also fixes unsupported audio codecs (mp4a.40.1 -> mp4a.40.2)
func rewriteM3U8URLs(content, baseURL, referer string) string {
	// Fix unsupported audio codec: AAC Main Profile (mp4a.40.1) -> AAC-LC (mp4a.40.2)
	// Chrome/Edge don't support mp4a.40.1 but the actual audio is usually AAC-LC compatible
	if strings.Contains(content, "mp4a.40.1") {
		content = strings.ReplaceAll(content, "mp4a.40.1", "mp4a.40.2")
	}

	// Get the base path for relative URLs (directory of the M3U8 file)
	basePath := manifestDirBase(baseURL)
	if basePath == "" {
		return content
	}

	// Mint ONE per-manifest correlation token. Every segment/child URL we
	// rewrite from THIS manifest carries the same token, so the streaming
	// service can group all of a watch's segment GETs into a single
	// aggregated egress row (AR-EGRESS-04). One manifest fetch → one token.
	sess := newSessToken()

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
			rewritten := rewriteURIAttribute(line, basePath, referer, sess)
			result.WriteString(rewritten)
			result.WriteString("\n")
			continue
		}

		// This is a URL line (not a comment)
		if !strings.HasPrefix(trimmed, "#") {
			rewrittenURL := rewriteHLSURL(trimmed, basePath, referer, sess)
			result.WriteString(rewrittenURL)
			result.WriteString("\n")
			continue
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// rewriteHLSURL rewrites a single HLS URL to go through the proxy. The sess
// argument is the per-manifest correlation token (minted once by the caller
// rewriteM3U8URLs); when non-empty it is appended as &sess=<token> so the
// streaming service can aggregate this manifest's segment GETs into one egress
// row (AR-EGRESS-04). Already-proxied URLs are returned untouched (the existing
// skip rule), so the token is only added to freshly-rewritten URLs.
func rewriteHLSURL(urlStr, basePath, referer, sess string) string {
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

	// Per-manifest session correlation token (AR-EGRESS-04). Non-authoritative
	// (crypto/rand, carries no authority — T-02-SESS); only groups segment GETs
	// into one aggregated egress row. Omitted when empty (rand failure) so the
	// segments simply fall back to per-request accounting.
	if sess != "" {
		proxyURL += "&sess=" + sess
	}

	return proxyURL
}

// vttImageCue matches storyboard-style cue payloads: an image path with an
// optional #xywh fragment. Anything else (real subtitles) passes through.
var vttImageCue = regexp.MustCompile(`^[^\s#]+\.(?:jpe?g|png|webp)(?:#.*)?$`)

// rewriteVTTURLs rewrites image cue payloads in a WebVTT thumbnail track (a
// storyboard track referencing sprite-sheet JPEGs on a private MinIO host) to
// proxied+signed URLs, preserving #xywh fragments — the same treatment
// rewriteM3U8URLs gives playlist children via rewriteHLSURL. Timing lines,
// headers, and non-image payloads (real subtitle text) are left untouched.
// Mints its own per-manifest correlation token (AR-EGRESS-04), mirroring how
// rewriteM3U8URLs mints one internally for playlist children — every sheet
// image fetched from this storyboard track groups into one aggregated
// egress row.
func rewriteVTTURLs(content, manifestURL, referer string) string {
	basePath := manifestDirBase(manifestURL)
	if basePath == "" {
		return content
	}

	sess := newSessToken()

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "WEBVTT") ||
			strings.HasPrefix(trimmed, "NOTE") || strings.Contains(trimmed, "-->") {
			continue
		}
		if !vttImageCue.MatchString(trimmed) {
			continue
		}
		urlPart, frag, hasFrag := strings.Cut(trimmed, "#")
		rewritten := rewriteHLSURL(urlPart, basePath, referer, sess)
		if hasFrag {
			rewritten += "#" + frag
		}
		lines[i] = rewritten
	}
	return strings.Join(lines, "\n")
}

// rewriteURIAttribute rewrites URI="..." attributes in M3U8 tags. sess is the
// per-manifest correlation token threaded through from rewriteM3U8URLs.
func rewriteURIAttribute(line, basePath, referer, sess string) string {
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
	rewrittenURI := rewriteHLSURL(originalURI, basePath, referer, sess)

	return line[:uriStart] + rewrittenURI + line[uriStart+uriEnd:]
}

// getCorrectContentType returns the correct Content-Type for proxied content.
// Some CDNs return incorrect types (like image/jpeg) to obfuscate video content.
// trustedUpstream marks first-party hosts (MinIO, stealth-scraper) whose
// image responses are genuine and must not be second-guessed.
func getCorrectHLSContentType(path, upstreamContentType string, trustedUpstream bool) string {
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

	// Genuine first-party images (e.g. scrub-preview sprite sheets on MinIO)
	// keep their declared type: the image→video assumption below exists for
	// lying scraper CDNs, and first-party hosts don't lie — while image bytes
	// labeled video/mp2t break under any future nosniff header.
	if trustedUpstream && strings.HasPrefix(upstreamContentType, "image/") && hasImageExt(pathLower) {
		return upstreamContentType
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

// hasImageExt reports whether the (lowercased) path ends in a genuine image
// extension — the shapes first-party assets actually use.
func hasImageExt(pathLower string) bool {
	return strings.HasSuffix(pathLower, ".jpg") || strings.HasSuffix(pathLower, ".jpeg") ||
		strings.HasSuffix(pathLower, ".png") || strings.HasSuffix(pathLower, ".webp")
}

// isHLSDomainAllowed checks if a domain is allowed for HLS proxying
func isHLSDomainAllowed(host string) bool {
	host = strings.ToLower(host)

	// Strip port number if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}

	for _, allowed := range HLSProxyAllowedDomains {
		if matchHLSDomain(host, allowed) {
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
