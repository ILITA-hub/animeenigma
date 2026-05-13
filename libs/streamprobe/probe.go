package streamprobe

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Result is the structured output of Probe.
type Result struct {
	Playable bool     // true only when Reason == ReasonPlayable
	Reason   Reason   // classification token (see reason.go)
	Sampled  []string // hostnames observed during the walk (for diagnostics)
}

const (
	perStepTimeout = 4 * time.Second
	totalBudget    = 10 * time.Second
	maxBodyBytes   = 1 << 20 // 1 MiB body cap (DoS guard for variant playlists)
	userAgent      = "AnimeEnigma-StreamProbe/1.0"
)

// Probe walks master m3u8 → first variant → first-segment HEAD and
// returns a structured Result. masterURL MUST be an absolute http(s) URL.
// headers are merged into the outbound request (Referer is the most
// common caller-supplied header).
//
// Per-step timeout: 4s (master GET, variant GET, segment HEAD each).
// Total budget: ≤ 10s via ctx with timeout.
//
// SSRF defense: rejects RFC1918 + loopback + link-local destinations
// BEFORE dialling. Ad-CDN host-suffix blocklist short-circuits BEFORE
// the HEAD probe (so we never leak our IP to TikTok's ad CDN).
func Probe(ctx context.Context, masterURL string, headers http.Header) Result {
	ctx, cancel := context.WithTimeout(ctx, totalBudget)
	defer cancel()

	// Step 1: validate URL + SSRF guard
	u, err := url.Parse(masterURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return Result{Reason: ReasonZeroMatch}
	}
	if !isPublicHost(u.Hostname()) {
		return Result{Reason: ReasonCDNUnreachable, Sampled: nil}
	}

	client := newHTTPClient()

	// Step 2: GET master
	masterBody, status, err := doGet(ctx, client, masterURL, headers)
	if err != nil {
		return Result{Reason: ReasonCDNUnreachable, Sampled: []string{u.Hostname()}}
	}
	if status == http.StatusForbidden {
		return classify403(masterURL, []string{u.Hostname()})
	}
	if status != http.StatusOK {
		return Result{Reason: ReasonStatus403, Sampled: []string{u.Hostname()}}
	}
	if !bytesIsM3U8(masterBody) {
		return Result{Reason: ReasonZeroMatch, Sampled: []string{u.Hostname()}}
	}

	// Step 3: extract first variant URI; if master IS already a media
	// playlist (#EXTINF rows directly), use it as the variant.
	variantURL, hasStreamInf := firstVariantURI(masterBody, u)
	if !hasStreamInf {
		// Master IS the media playlist — re-use masterBody as variant.
		return checkSegments(ctx, client, masterBody, u, []string{u.Hostname()}, headers)
	}
	if variantURL == "" {
		return Result{Reason: ReasonZeroMatch, Sampled: []string{u.Hostname()}}
	}

	vu, err := url.Parse(variantURL)
	if err != nil || !isPublicHost(vu.Hostname()) {
		return Result{Reason: ReasonCDNUnreachable, Sampled: []string{u.Hostname()}}
	}

	variantBody, vstatus, verr := doGet(ctx, client, variantURL, headers)
	sampled := []string{u.Hostname(), vu.Hostname()}
	if verr != nil {
		return Result{Reason: ReasonCDNUnreachable, Sampled: sampled}
	}
	if vstatus == http.StatusForbidden {
		return classify403(variantURL, sampled)
	}
	if vstatus != http.StatusOK {
		return Result{Reason: ReasonStatus403, Sampled: sampled}
	}
	if !bytesIsM3U8(variantBody) {
		return Result{Reason: ReasonZeroMatch, Sampled: sampled}
	}
	return checkSegments(ctx, client, variantBody, vu, sampled, headers)
}

// checkSegments walks #EXTINF entries, classifies the FIRST segment.
func checkSegments(ctx context.Context, client *http.Client, body []byte, base *url.URL, sampled []string, headers http.Header) Result {
	segs := extractSegmentURIs(body, base)
	if len(segs) == 0 {
		return Result{Reason: ReasonEmptyResponse, Sampled: sampled}
	}
	first := segs[0]
	fu, err := url.Parse(first)
	if err != nil {
		return Result{Reason: ReasonZeroMatch, Sampled: sampled}
	}
	segHost := fu.Hostname()
	sampled = append(sampled, segHost)
	// Ad-CDN check short-circuits BEFORE any HEAD probe — defense against
	// leaking our IP to TikTok's ad CDN (T-21-03).
	if isAdCDNHost(segHost) {
		return Result{Reason: ReasonAdDecoy, Sampled: sampled}
	}
	if !isPublicHost(segHost) {
		return Result{Reason: ReasonCDNUnreachable, Sampled: sampled}
	}
	status, herr := doHead(ctx, client, first, headers)
	if herr != nil {
		return Result{Reason: ReasonCDNUnreachable, Sampled: sampled}
	}
	if status == http.StatusForbidden {
		return classify403(first, sampled)
	}
	if status < 200 || status >= 300 {
		return Result{Reason: ReasonStatus403, Sampled: sampled}
	}
	return Result{Playable: true, Reason: ReasonPlayable, Sampled: sampled}
}

// newHTTPClient builds a client with per-step timeout 4s.
func newHTTPClient() *http.Client {
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: perStepTimeout}).DialContext,
		TLSHandshakeTimeout:   perStepTimeout,
		ResponseHeaderTimeout: perStepTimeout,
	}
	return &http.Client{Timeout: perStepTimeout, Transport: tr}
}

func doGet(ctx context.Context, client *http.Client, raw string, headers http.Header) ([]byte, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, perStepTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func doHead(ctx context.Context, client *http.Client, raw string, headers http.Header) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, perStepTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, raw, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, nil
}

// utf8BOM is the UTF-8 byte order mark (\xEF\xBB\xBF). Some upstream
// CDNs emit m3u8 playlists with a leading BOM that bytesIsM3U8 must
// strip before the #EXTM3U sentinel check.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// bytesIsM3U8 reports whether body starts with the #EXTM3U sentinel
// (allowing leading UTF-8 BOM + whitespace).
func bytesIsM3U8(body []byte) bool {
	body = bytes.TrimPrefix(body, utf8BOM)
	s := strings.TrimLeft(string(body), " \t\r\n")
	return strings.HasPrefix(s, "#EXTM3U")
}

// firstVariantURI returns the resolved absolute URL of the first
// #EXT-X-STREAM-INF variant entry, plus a flag indicating whether body
// appeared to be a master playlist (variant list) vs a media playlist
// (segment list). If no #EXT-X-STREAM-INF lines are found, returns
// ("", false) — caller should treat body as a media playlist directly.
func firstVariantURI(body []byte, base *url.URL) (string, bool) {
	lines := strings.Split(string(body), "\n")
	seenStreamInf := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#EXT-X-STREAM-INF") {
			seenStreamInf = true
			// Next non-comment, non-empty line is the URI.
			for j := i + 1; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if t == "" || strings.HasPrefix(t, "#") {
					continue
				}
				return resolveURI(base, t), true
			}
		}
	}
	return "", seenStreamInf
}

// extractSegmentURIs returns the resolved absolute URLs of every
// #EXTINF segment entry in body.
func extractSegmentURIs(body []byte, base *url.URL) []string {
	lines := strings.Split(string(body), "\n")
	out := make([]string, 0, 16)
	expectURI := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#EXTINF") {
			expectURI = true
			continue
		}
		if expectURI && !strings.HasPrefix(t, "#") {
			out = append(out, resolveURI(base, t))
			expectURI = false
		}
	}
	return out
}

func resolveURI(base *url.URL, raw string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

// allowLoopbackForTests opens an internal escape hatch for unit tests
// using httptest.NewServer (which binds to 127.0.0.1). Production code
// MUST NEVER flip this — it is package-private and untouched outside of
// _test.go files. Defense-in-depth: even with this flag true, RFC1918
// + link-local + unspecified are still blocked.
var allowLoopbackForTests bool

// isPublicHost rejects loopback, RFC1918 private, link-local, and
// unspecified hostnames before any HTTP dial. Hostnames that don't
// resolve to an IP (true DNS names) are allowed — DNS resolution
// happens at dial time, where the standard library's dialer will
// re-check.
func isPublicHost(host string) bool {
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname (not raw IP) — defer to dial-time resolution.
		// Block obvious cases by literal name (unless tests opted in).
		lower := strings.ToLower(host)
		if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
			return allowLoopbackForTests
		}
		return true
	}
	if ip.IsLoopback() {
		return allowLoopbackForTests
	}
	return !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// signedURLEpochRe captures a numeric `e=<unix-seconds>` or
// `expires=<unix-seconds>` query param.
var signedURLEpochRe = regexp.MustCompile(`(?:^|[?&])(?:e|expires)=(\d{8,12})(?:&|$)`)

// classify403 distinguishes a generic 403 from an EXPIRED signed URL
// (the latter is recoverable by re-fetching upstream).
func classify403(raw string, sampled []string) Result {
	m := signedURLEpochRe.FindStringSubmatch(raw)
	if len(m) == 2 {
		epoch, err := strconv.ParseInt(m[1], 10, 64)
		if err == nil && time.Now().Unix() > epoch {
			return Result{Reason: ReasonSignedURLExpired, Sampled: sampled}
		}
	}
	return Result{Reason: ReasonStatus403, Sampled: sampled}
}
