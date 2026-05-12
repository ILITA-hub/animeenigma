// kwik.go — in-process Kwik embed extractor.
//
// SCRAPER-PAHE-03 / Plan 16-02. Unlike MegacloudClient (HTTP sidecar wrapper),
// KwikExtractor performs the Dean-Edwards-packer unpack INSIDE the Go process
// via github.com/dop251/goja. This is appropriate because the Kwik unpacker is
// a self-contained packer expression with no DOM / browser API usage — goja's
// stripped runtime is sufficient.
//
// Pitfalls (RESEARCH.md):
//
//   - Pitfall 2: goja.Runtime is NOT thread-safe. Each Extract() constructs a
//     FRESH `goja.New()` — never cached, never pooled across goroutines.
//   - Pitfall 3: vm.Interrupt() MUST be invoked from a separate goroutine, not
//     the goroutine running RunString — otherwise a runaway script cannot be
//     cancelled. The watchdog goroutine in Extract() owns the Interrupt.
//   - Body cap: io.LimitReader at 2 MiB prevents a hostile / misbehaving
//     upstream from OOMing the scraper.
//   - SSRF: Matches() uses host-equality + strict subdomain check (HasSuffix on
//     "."+known); substring matching is forbidden and explicitly tested against.
package embeds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// kwikHosts is the case-insensitive allowlist of embed hosts this extractor
// handles. Match policy: host equality OR strict subdomain (HasSuffix on
// "."+known). Adding a host that Kwik rotates onto is a one-line change here.
var kwikHosts = []string{
	"kwik.cx",
	"kwik.si",
}

// defaultKwikTimeout is the goja execution budget per Extract call. The Kwik
// unpacker normally completes in < 10 ms; this 5s cap exists to bound the
// worst case (hostile JS / infinite loop).
const defaultKwikTimeout = 5 * time.Second

// defaultKwikHTTPTimeout bounds the HTTP fetch of the embed page itself.
// The actual GET → packed-HTML round-trip is typically < 1s.
const defaultKwikHTTPTimeout = 15 * time.Second

// maxKwikBody caps the response body read at 2 MiB. Real Kwik embed pages are
// < 100 KiB; this cap exists purely as a DoS guard against a hostile upstream
// returning a multi-gigabyte body.
const maxKwikBody = 2 << 20 // 2 MiB

// htmlCommentRegex matches HTML `<!-- ... -->` blocks. We strip these before
// running packerStartRegex so an HTML comment that documents an example packer
// (as in our golden fixture) cannot shadow the real `<script>eval(...)` block.
var htmlCommentRegex = regexp.MustCompile(`(?s)<!--.*?-->`)

// packerStartRegex locates the Dean-Edwards packer entry point. We anchor on
// the fixed-parameter list `function(p,a,c,k,e,d)` because the packer always
// emits exactly that signature. After locating the start, we use a paren
// balancer (extractBalancedPacker) to find the matching `))` — regex alone
// cannot reliably handle nested-paren / brace args (Kwik packers embed
// `.split('|')` and trailing `{}` dict args in real-world output).
var packerStartRegex = regexp.MustCompile(`eval\(function\(p,a,c,k,e,d\)`)

// sourceURLRegex pulls m3u8 URLs out of the unpacked script.  Real Kwik
// upstream uses const declarations (`const source='...m3u8...'`) and an
// optional `const sources=[{file:'...m3u8...', label:'480p'}, ...]` for
// multi-quality variants. We're permissive about the declaration keyword
// (const/var/let), surrounding quote style, and whether the URL is followed
// by query-string junk (Kwik tokens, expiry timestamps, etc.).
//
// Capture group 1 is the m3u8 URL. We deliberately tolerate ONLY the URL
// itself — any trailing label / quality string is consumed via a subsequent
// regex (qualityLabelRegex) so a malformed source line cannot smuggle an
// arbitrary value through.
var sourceURLRegex = regexp.MustCompile(`(?:const|var|let|file\s*:)\s*(?:source\s*=\s*)?\\?['"]?(https?://[^'"\\\s]+\.m3u8[^'"\\\s]*)`)

// qualityLabelRegex pulls the quality label (e.g. "720p") from a
// `{file:'...m3u8...', label:'720p'}` style entry. Only used as a best-effort
// annotation on the returned Source; absence is fine.
var qualityLabelRegex = regexp.MustCompile(`label\s*:\s*\\?['"]?(\d{3,4}p)`)

// extractPacker scans `body` for `eval(function(p,a,c,k,e,d)` and returns the
// IIFE substring beginning at `function(...)` and ending at the matching
// outer `))`. Returns (iife, true) on success or ("", false) if no balanced
// packer is found.
//
// We hand-roll a paren / brace balancer instead of using regex because real
// Kwik IIFE arguments embed `.split('|')` (nested parens) and a trailing `{}`
// dict — patterns regex can express but only with brittle precedence rules.
// Manual balancing is unambiguous and bounded by the body length.
//
// Structure of a Dean-Edwards-packed IIFE:
//
//	function(p,a,c,k,e,d){ body }( args )
//
// We balance in three stages: (1) param-list `(...)`, (2) function-body
// `{...}`, (3) call-args `(...)`. The string returned is the substring from
// `function` through the final `)` of stage 3.
//
// String-literal awareness: we track when we're inside a `'...'` or `"..."`
// literal and ignore parens / braces inside them. Backslash-escaped quotes
// (e.g. `\'`) are honored — Dean-Edwards emits escaped single quotes in its
// packed string argument.
func extractPacker(body string) (string, bool) {
	loc := packerStartRegex.FindStringIndex(body)
	if loc == nil {
		return "", false
	}
	// Move past the leading `eval(` — we want the IIFE itself, starting at
	// `function(...)`.
	const evalPrefix = "eval("
	iffeStart := loc[0] + len(evalPrefix)

	// Stage 1: balance the parameter-list `(p,a,c,k,e,d)`. Skip whitespace
	// to find the opening `(`, then walk to its match.
	i := iffeStart
	for i < len(body) && body[i] != '(' {
		i++
	}
	if i >= len(body) {
		return "", false
	}
	end, ok := balanceUntil(body, i, '(', ')')
	if !ok {
		return "", false
	}

	// Stage 2: balance the function body `{...}`. Skip whitespace to find `{`.
	j := end + 1
	for j < len(body) && (body[j] == ' ' || body[j] == '\t' || body[j] == '\n' || body[j] == '\r') {
		j++
	}
	if j >= len(body) || body[j] != '{' {
		return "", false
	}
	end, ok = balanceUntil(body, j, '{', '}')
	if !ok {
		return "", false
	}

	// Stage 3: balance the call-args `(...)`. Skip whitespace to find `(`.
	j = end + 1
	for j < len(body) && (body[j] == ' ' || body[j] == '\t' || body[j] == '\n' || body[j] == '\r') {
		j++
	}
	if j >= len(body) || body[j] != '(' {
		return "", false
	}
	end, ok = balanceUntil(body, j, '(', ')')
	if !ok {
		return "", false
	}

	return body[iffeStart : end+1], true
}

// balanceUntil walks `body` from index `start` (which MUST point at `open`)
// and returns the index of the matching `close`, accounting for string
// literals and backslash-escape sequences inside them. Returns (idx, true) on
// success or (0, false) if the close is never reached.
//
// Only `open` / `close` characters affect the depth counter — other bracket
// pairs (e.g. balancing parens while we're inside a `{`-body) are NOT counted.
// The caller invokes balanceUntil once per nesting level.
func balanceUntil(body string, start int, open, close byte) (int, bool) {
	if start >= len(body) || body[start] != open {
		return 0, false
	}
	type qstate int
	const (
		none qstate = iota
		sgl
		dbl
	)
	q := none
	depth := 0
	for i := start; i < len(body); i++ {
		c := body[i]
		// Backslash escape inside a string literal: skip the next char.
		if q != none && c == '\\' && i+1 < len(body) {
			i++
			continue
		}
		switch q {
		case sgl:
			if c == '\'' {
				q = none
			}
			continue
		case dbl:
			if c == '"' {
				q = none
			}
			continue
		}
		switch c {
		case '\'':
			q = sgl
		case '"':
			q = dbl
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// KwikExtractor implements domain.EmbedExtractor for kwik.cx / kwik.si.
type KwikExtractor struct {
	http    *http.Client
	timeout time.Duration
}

// KwikOption configures NewKwikExtractor. See WithKwikTimeout, WithKwikHTTPClient.
type KwikOption func(*KwikExtractor)

// WithKwikTimeout overrides the goja-execution timeout (default 5s).
// Tests use this to inject very short timeouts when feeding the runtime a
// deliberate infinite loop.
func WithKwikTimeout(d time.Duration) KwikOption {
	return func(k *KwikExtractor) {
		if d > 0 {
			k.timeout = d
		}
	}
}

// WithKwikHTTPClient overrides the http.Client used to fetch the embed page.
// Defaults to a fresh client with Timeout=defaultKwikHTTPTimeout.
func WithKwikHTTPClient(c *http.Client) KwikOption {
	return func(k *KwikExtractor) {
		if c != nil {
			k.http = c
		}
	}
}

// NewKwikExtractor constructs a KwikExtractor with the given options applied.
// Defaults: 15s HTTP timeout, 5s goja-execution timeout.
func NewKwikExtractor(opts ...KwikOption) *KwikExtractor {
	k := &KwikExtractor{
		http:    &http.Client{Timeout: defaultKwikHTTPTimeout},
		timeout: defaultKwikTimeout,
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// Name implements domain.EmbedExtractor.
func (k *KwikExtractor) Name() string { return "kwik" }

// Matches reports whether embedURL points to a kwik-family host. Match policy:
// host equality OR strict subdomain (HasSuffix on "."+known). Substring matches
// in path / query are NOT matched — see TestKwik_Matches_RejectsSubdomainImposters
// for the explicit SSRF regression test. WR-05: reject schemes other than
// http/https up-front so an embedURL like `kwik://kwik.cx/` does not pretend
// to match — Go's HTTP client would later reject the unknown scheme with an
// unhelpful error mapping (ErrProviderDown), but it should never get that far.
func (k *KwikExtractor) Matches(embedURL string) bool {
	u, err := url.Parse(embedURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	for _, known := range kwikHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract fetches the Kwik embed page, locates the Dean-Edwards-packed IIFE,
// evaluates it inside a fresh goja runtime, and parses the resulting unpacked
// string for m3u8 URLs. Returns a *domain.Stream with one Source per quality
// found (multi-quality fan-out via sources=[{file,label}...] arrays).
//
// Error mapping:
//
//   - http.Do failure → WrapProviderDown ("kwik: fetch failed")
//   - non-2xx status → WrapProviderDown with status code
//   - body read failure → WrapProviderDown ("kwik: read body")
//   - no packed-JS match → WrapExtractFailed ("kwik: no eval() packer")
//   - goja runtime error / interrupt → WrapExtractFailed ("kwik: goja runtime")
//   - unpacked string has no m3u8 → WrapExtractFailed ("kwik: no m3u8 in unpacked source")
func (k *KwikExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "kwik: build request")
	}
	// Forward caller-supplied headers; default Referer to animepahe if absent
	// (Kwik upstream requires a Referer for the embed page fetch).
	for hk, vs := range headers {
		for _, v := range vs {
			req.Header.Add(hk, v)
		}
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", "https://animepahe.ru")
	}

	resp, err := k.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "kwik: fetch failed")
	}
	defer func() {
		// Drain unread bytes so the keep-alive connection can be reused, then
		// close.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapProviderDown(
			errors.New(http.StatusText(resp.StatusCode)),
			fmt.Sprintf("kwik: upstream status %d", resp.StatusCode),
		)
	}

	// Cap the body at 2 MiB so a hostile upstream can't OOM us. Real Kwik
	// embed pages are < 100 KiB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxKwikBody))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "kwik: read body")
	}

	// Strip HTML comments first so a documented example packer inside a
	// `<!-- ... -->` block cannot shadow the real `<script>eval(...)` block.
	// Real Kwik HTML rarely contains comments, but our golden fixture does.
	stripped := htmlCommentRegex.ReplaceAll(body, []byte(""))

	iife, ok := extractPacker(string(stripped))
	if !ok {
		return nil, domain.WrapExtractFailed(
			errors.New("packed-JS scanner found no eval(function(p,a,c,k,e,d){...}) block"),
			"kwik: no eval() packer",
		)
	}

	// Wrap with leading + trailing parens so RunString returns the IIFE's
	// return value (the unpacked source) rather than executing it as a
	// statement and discarding the value.
	wrapper := "(" + iife + ")"

	unpacked, err := k.runGoja(ctx, wrapper)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "kwik: goja runtime")
	}

	matches := sourceURLRegex.FindAllStringSubmatch(unpacked, -1)
	if len(matches) == 0 {
		return nil, domain.WrapExtractFailed(
			errors.New("no m3u8 URL captured from unpacked source"),
			"kwik: no m3u8 in unpacked source",
		)
	}

	// Dedupe URLs (a Dean-Edwards unpack sometimes emits `const source='...'`
	// AND a `sources=[{file:'...'}]` containing the SAME URL — we want one
	// Source per unique URL).
	seen := make(map[string]bool, len(matches))
	sources := make([]domain.Source, 0, len(matches))

	// Parallel walk: find the quality label nearest each URL by scanning the
	// surrounding ~80-char window. This is best-effort — absent labels just
	// leave Quality="".
	qualityMatches := qualityLabelRegex.FindAllStringSubmatchIndex(unpacked, -1)

	for _, m := range matches {
		u := m[1]
		if seen[u] {
			continue
		}
		seen[u] = true
		src := domain.Source{
			URL:  u,
			Type: "hls",
		}
		// Find the m3u8 URL's offset in the unpacked string, then find the
		// nearest label match within +/- 80 chars.
		urlIdx := strings.Index(unpacked, u)
		if urlIdx >= 0 {
			for _, qm := range qualityMatches {
				labelStart := qm[2]
				if labelStart >= urlIdx-80 && labelStart <= urlIdx+80 {
					src.Quality = unpacked[qm[2]:qm[3]]
					break
				}
			}
		}
		sources = append(sources, src)
	}

	return &domain.Stream{
		Sources: sources,
		Headers: map[string]string{
			// Kwik HLS segments require Referer=kwik.cx; the downstream HLS
			// proxy uses this when fetching variant playlists + segments.
			"Referer": "https://kwik.cx/",
		},
	}, nil
}

// runGoja evaluates the given wrapper expression (a parenthesized IIFE) inside
// a FRESH goja runtime with a watchdog goroutine that fires vm.Interrupt() on
// either ctx cancellation or the timeout, whichever comes first.
//
// Plan 18-03 refactor: the goja-runtime + watchdog body has been LIFTED into
// a package-level helper (`runGoja` in packed_common.go) so the same logic is
// consumed by KwikExtractor (via this method) and the shared packedExtractor
// base type (StreamHG / Earnvids). This method is now a thin one-line wrapper
// that forwards the call with k.timeout — Phase 16 callers and tests are
// unchanged.
//
// Pitfall 2: a fresh runtime per call — never cached, never pooled.
// Pitfall 3: Interrupt() comes from a goroutine other than the one running
// RunString. The `done` channel ensures the watchdog goroutine exits promptly
// after a normal completion so we don't leak goroutines.
func (k *KwikExtractor) runGoja(ctx context.Context, expr string) (string, error) {
	return runGoja(ctx, expr, k.timeout)
}

// Compile-time assertion: KwikExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*KwikExtractor)(nil)
