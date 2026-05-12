// vibeplayer.go — VibePlayerExtractor for vibeplayer.site.
//
// SCRAPER-9ANI-03 / Plan 18-03. The wrapper page emits the playable HLS URL
// directly via `const src = "https://vibeplayer.site/public/stream/<id>/master.m3u8"`
// inside the JW Player initialization block. No Dean-Edwards packer is
// involved — vibeplayer is the simplest of the three new extractors and uses
// only regex (no goja runtime).
//
// Optional `const subtitle = "..."` is captured into stream.Tracks when
// non-empty; the JW Player template emits an empty string when no captions
// are available.
//
// Pitfalls:
//   - SSRF: Matches() uses host equality + strict subdomain (HasSuffix on
//     "."+known). Substring matches are forbidden by contract (T-18-14, T-18-15).
//   - DoS: Body cap at 2 MiB via io.LimitReader (T-18-16).
//   - Observability: parser_zero_match_total emits on src-regex failure with
//     selector="vibeplayer_src_const" so Phase 17 health probe catches drift.
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

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

const (
	defaultVibePlayerHTTPTimeout = 15 * time.Second
	maxVibePlayerBody            = 2 << 20 // 2 MiB DoS guard.

	// Selector identifiers for parser_zero_match_total. Short stable
	// strings, low-cardinality (P-02 from Phase 17 RESEARCH).
	selectorVibePlayerSrcRegex = "vibeplayer_src_const"
	selectorVibePlayerBodyRead = "vibeplayer_body_read"
)

// vibePlayerHosts is the case-insensitive allowlist. Match policy: host
// equality OR strict subdomain (HasSuffix on "."+known).
var vibePlayerHosts = []string{"vibeplayer.site"}

// vibePlayerReferer is forced on outgoing requests when the caller does not
// supply a Referer. The Stream's Referer header (returned to the HLS proxy)
// is the same value so segment fetches succeed.
const vibePlayerReferer = "https://vibeplayer.site/"

var (
	// vibePlayerSrcRegex captures the m3u8 URL from `const src = "..."` inside
	// the JW Player init block. The wrapper template emits the const using
	// double quotes only, so the regex is conservative.
	vibePlayerSrcRegex = regexp.MustCompile(`const\s+src\s*=\s*"(https?://[^"]+\.m3u8[^"]*)"`)
	// vibePlayerSubRegex captures the (optional) caption URL. The template
	// emits `const subtitle = ""` when no captions exist — caller checks
	// for non-empty submatch before appending a Track.
	vibePlayerSubRegex = regexp.MustCompile(`const\s+subtitle\s*=\s*"([^"]*)"`)
)

// VibePlayerExtractor pulls the inline `const src="...m3u8"` URL out of a
// vibeplayer.site wrapper page. Pure regex — no goja.
type VibePlayerExtractor struct {
	http    *http.Client
	timeout time.Duration
}

// NewVibePlayerExtractor returns a VibePlayerExtractor with default HTTP /
// runtime timeouts.
func NewVibePlayerExtractor() *VibePlayerExtractor {
	return &VibePlayerExtractor{
		http:    &http.Client{Timeout: defaultVibePlayerHTTPTimeout},
		timeout: defaultVibePlayerHTTPTimeout,
	}
}

// Name implements domain.EmbedExtractor.
func (e *VibePlayerExtractor) Name() string { return "vibeplayer" }

// Matches reports whether embedURL points to vibeplayer.site (or a strict
// subdomain). T-18-14 / T-18-15.
func (e *VibePlayerExtractor) Matches(embedURL string) bool {
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
	for _, known := range vibePlayerHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract fetches the wrapper page, regex-pulls `const src="...m3u8"` (and
// optional `const subtitle="..."`), and returns *domain.Stream. Returns a
// wrapped ErrExtractFailed / ErrProviderDown on failure.
func (e *VibePlayerExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !e.Matches(embedURL) {
		return nil, domain.WrapExtractFailed(
			errors.New("host not in allowlist"),
			"vibeplayer: Matches gate",
		)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "vibeplayer: build request")
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Referer") == "" {
		// Use the gogoanime upstream as the default Referer when the caller
		// doesn't supply one — vibeplayer's CDN accepts either origin.
		req.Header.Set("Referer", "https://anitaku.to/")
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "vibeplayer: fetch")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d", resp.StatusCode),
			"vibeplayer: HTTP status",
		)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVibePlayerBody))
	if err != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues("vibeplayer", selectorVibePlayerBodyRead).Inc()
		return nil, domain.WrapProviderDown(err, "vibeplayer: read")
	}

	srcM := vibePlayerSrcRegex.FindSubmatch(body)
	if srcM == nil {
		metrics.ParserZeroMatchTotal.WithLabelValues("vibeplayer", selectorVibePlayerSrcRegex).Inc()
		return nil, domain.WrapExtractFailed(
			errors.New("no `const src = \"...m3u8\"` in body"),
			"vibeplayer: src regex",
		)
	}
	stream := &domain.Stream{
		Sources: []domain.Source{{URL: string(srcM[1]), Type: "hls"}},
		Headers: map[string]string{"Referer": vibePlayerReferer},
	}
	// Optional captions: emit a Track only when subtitle const is non-empty.
	if subM := vibePlayerSubRegex.FindSubmatch(body); subM != nil && len(subM[1]) > 0 {
		stream.Tracks = []domain.Track{
			{
				File:    string(subM[1]),
				Label:   "English",
				Kind:    "captions",
				Default: true,
			},
		}
	}
	return stream, nil
}

// Compile-time assertion: VibePlayerExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*VibePlayerExtractor)(nil)
