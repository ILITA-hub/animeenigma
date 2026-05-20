// vidstream_vip.go — VidstreamVipExtractor for am.vidstream.vip.
//
// Phase 28 SCRAPER-HEAL-38. The embed page is a JWPlayer wrapper with an
// inline `sources: [{"file":"...m3u8","type":"mp4","label":"HD"}]` literal.
// Plain regex + json.Unmarshal — NO Dean-Edwards-packer, NO goja runtime
// (RESEARCH.md Discretion). Threat model: PLAN.md §threat_model T-28-03-01
// through T-28-03-05. SSRF mitigation lives at the streaming-proxy
// allowlist; this file enforces host-allowlist Matches, 2 MiB body cap,
// JSON-shape validation, and absolute-URL rejection.
package embeds

import (
	"context"
	"encoding/json"
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
	defaultVidstreamVipHTTPTimeout = 15 * time.Second
	maxVidstreamVipBody            = 2 << 20 // 2 MiB DoS guard.

	// vidstreamVipReferer — value returned in Stream.Headers so the HLS
	// proxy replays it when fetching variant playlists + segments. Also
	// used as the default outgoing Referer when the caller did not set one.
	vidstreamVipReferer = "https://am.vidstream.vip/"

	// Selector labels for parser_zero_match_total (low-cardinality).
	selectorVidstreamVipSourcesRegex = "vidstream_vip_sources_regex"
	selectorVidstreamVipSourcesJSON  = "vidstream_vip_sources_json"
	selectorVidstreamVipBodyRead     = "vidstream_vip_body_read"
)

// vidstreamVipHosts is the case-insensitive allowlist. Match policy: host
// equality OR strict subdomain (HasSuffix on "."+known) — T-28-03-05.
var vidstreamVipHosts = []string{"am.vidstream.vip", "vidstream.vip"}

// vidstreamVipSourcesRegex captures the first `{...}` object inside the
// inline `sources: [...]` literal. Shape verified live 2026-05-20:
// `sources: [{"file":"https://...m3u8","type":"mp4","label":"HD"}],`.
var vidstreamVipSourcesRegex = regexp.MustCompile(`sources\s*:\s*\[\s*({[^}]+})`)

// VidstreamVipExtractor pulls the m3u8 URL out of an am.vidstream.vip embed
// page via plain regex + json.Unmarshal. Pure stdlib — no goja, no chromedp.
type VidstreamVipExtractor struct {
	http    *http.Client
	timeout time.Duration
}

// NewVidstreamVipExtractor returns a VidstreamVipExtractor with default
// HTTP / runtime timeouts.
func NewVidstreamVipExtractor() *VidstreamVipExtractor {
	return &VidstreamVipExtractor{
		http:    &http.Client{Timeout: defaultVidstreamVipHTTPTimeout},
		timeout: defaultVidstreamVipHTTPTimeout,
	}
}

// Name implements domain.EmbedExtractor.
func (e *VidstreamVipExtractor) Name() string { return "vidstream_vip" }

// Hosts implements embeds.HostingExtractor — returns the lowercase host allowlist.
func (e *VidstreamVipExtractor) Hosts() []string {
	out := make([]string, len(vidstreamVipHosts))
	copy(out, vidstreamVipHosts)
	return out
}

// Matches reports whether embedURL points to am.vidstream.vip (or a strict subdomain).
func (e *VidstreamVipExtractor) Matches(embedURL string) bool {
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
	for _, known := range vidstreamVipHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract fetches the embed page, regex-pulls the inline
// `sources: [{"file":"...m3u8"}]` object, and returns *domain.Stream with
// one HLS Source + Referer header. Transport error / 5xx → ErrProviderDown;
// 4xx / regex miss / malformed JSON / non-absolute URL → ErrExtractFailed.
func (e *VidstreamVipExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !e.Matches(embedURL) {
		return nil, domain.WrapExtractFailed(
			errors.New("host not in allowlist"),
			"vidstream_vip: Matches gate",
		)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "vidstream_vip: build request")
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Referer") == "" {
		// AnimeFever is the expected caller; default keeps extractor usable standalone.
		req.Header.Set("Referer", "https://animefever.cc/")
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "vidstream_vip: fetch")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d", resp.StatusCode),
			"vidstream_vip: HTTP status",
		)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("upstream %d", resp.StatusCode),
			"vidstream_vip: HTTP status",
		)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVidstreamVipBody))
	if err != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues("vidstream_vip", selectorVidstreamVipBodyRead).Inc()
		return nil, domain.WrapProviderDown(err, "vidstream_vip: read")
	}

	m := vidstreamVipSourcesRegex.FindSubmatch(body)
	if len(m) < 2 {
		metrics.ParserZeroMatchTotal.WithLabelValues("vidstream_vip", selectorVidstreamVipSourcesRegex).Inc()
		return nil, domain.WrapExtractFailed(
			errors.New("no `sources: [{...}]` literal in body"),
			"vidstream_vip: sources regex",
		)
	}
	var src struct {
		File  string `json:"file"`
		Type  string `json:"type"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal(m[1], &src); err != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues("vidstream_vip", selectorVidstreamVipSourcesJSON).Inc()
		return nil, domain.WrapExtractFailed(err, "vidstream_vip: parse source json")
	}
	if !strings.HasPrefix(src.File, "http://") && !strings.HasPrefix(src.File, "https://") {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("non-absolute URL %q", src.File),
			"vidstream_vip: url shape",
		)
	}
	return &domain.Stream{
		Sources: []domain.Source{{
			URL:     src.File,
			Type:    "hls",
			Quality: src.Label,
		}},
		Headers: map[string]string{"Referer": vidstreamVipReferer},
	}, nil
}

// Compile-time assertion: VidstreamVipExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*VidstreamVipExtractor)(nil)
