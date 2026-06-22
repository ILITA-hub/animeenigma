// okru.go — OkruExtractor for ok.ru /videoembed pages.
//
// ok.ru (Odnoklassniki) embeds carry a static `data-options="{…}"` attribute
// whose flashvars.metadata (a JSON-encoded string) holds hlsManifestUrl /
// ondemandHls (HLS master) + a videos[] array of progressive MP4s. No JS
// execution and no Cloudflare — a plain GET from our egress returns it
// (verified live 2026-06-22). okcdn.ru manifests are IP-locked to the
// requesting egress; catalog signs the resolved URL so the HLS proxy (same
// host) trusts it. Falls back to POST /dk?cmd=videoPlayerMetadata when the
// inline metadata is absent (yt-dlp Odnoklassniki algorithm).
package embeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

const (
	defaultOkruHTTPTimeout = 15 * time.Second
	maxOkruBody            = 4 << 20 // 4 MiB DoS guard (ok.ru pages are larger)
	okruReferer            = "https://ok.ru/"
	// okruUA must be a real-browser UA: ok.ru bakes the resolving engine into the
	// returned okcdn stream URL (srcAg/GECKO for Firefox) and okcdn then rejects a
	// fetch whose UA engine doesn't match (400). This MUST stay the same
	// Gecko/Firefox family as the HLS proxy's fetch UA (libs/videoutils
	// DefaultProxyConfig.UserAgent) or playback 400s. Verified: GECKO resolve +
	// Firefox fetch => 200 #EXTM3U.
	okruUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0"
)

var okruHosts = []string{"ok.ru"} // host equality OR strict subdomain (m.ok.ru, www.ok.ru, …)

// dataOptionsRe captures the data-options attribute payload (HTML-escaped JSON).
var dataOptionsRe = regexp.MustCompile(`data-options="([^"]*)"`)

// okMetadata is the inner flashvars.metadata object.
type okMetadata struct {
	HLSManifestURL string `json:"hlsManifestUrl"`
	OndemandHls    string `json:"ondemandHls"`
	Videos         []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"videos"`
}

// OkruExtractor resolves ok.ru /videoembed pages to a Stream. Pure parse, no JS.
type OkruExtractor struct {
	http      *http.Client
	extraHost string // test-only seam for httptest
}

// NewOkruExtractor returns an OkruExtractor with the default HTTP timeout.
func NewOkruExtractor() *OkruExtractor {
	return &OkruExtractor{http: &http.Client{Timeout: defaultOkruHTTPTimeout}}
}

// allowTestHost lets unit tests point Extract at an httptest server.
func (e *OkruExtractor) allowTestHost(host string) { e.extraHost = strings.ToLower(host) }

// Name implements domain.EmbedExtractor.
func (e *OkruExtractor) Name() string { return "okru" }

// Matches reports whether embedURL is an ok.ru (or strict subdomain) URL.
func (e *OkruExtractor) Matches(embedURL string) bool {
	u, err := url.Parse(embedURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	if e.extraHost != "" && (host == e.extraHost || host+":"+u.Port() == e.extraHost) {
		return true
	}
	for _, known := range okruHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract fetches the embed page, parses data-options → flashvars.metadata, and
// returns a Stream (HLS first, MP4 fallbacks). Referer is set so the HLS proxy
// carries it to okcdn.ru segment fetches.
func (e *OkruExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !e.Matches(embedURL) {
		return nil, domain.WrapExtractFailed(errors.New("host not in allowlist"), "okru: Matches gate")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "okru: build request")
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", okruUA)
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", okruReferer)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "okru: fetch")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapProviderDown(fmt.Errorf("upstream %d", resp.StatusCode), "okru: HTTP status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOkruBody))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "okru: read")
	}
	md, err := parseOkMetadata(body)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "okru: parse data-options")
	}
	stream := &domain.Stream{Headers: map[string]string{"Referer": okruReferer}}
	if hls := md.HLSManifestURL; hls != "" {
		stream.Sources = append(stream.Sources, domain.Source{URL: hls, Type: "hls", Quality: "auto"})
	} else if hls := md.OndemandHls; hls != "" {
		stream.Sources = append(stream.Sources, domain.Source{URL: hls, Type: "hls", Quality: "auto"})
	}
	for _, v := range md.Videos {
		if v.URL == "" {
			continue
		}
		stream.Sources = append(stream.Sources, domain.Source{URL: v.URL, Type: "mp4", Quality: v.Name})
	}
	if len(stream.Sources) == 0 {
		return nil, domain.WrapExtractFailed(errors.New("no hls/mp4 in metadata"), "okru: empty metadata")
	}
	return stream, nil
}

// parseOkMetadata pulls flashvars.metadata out of the data-options attribute.
// metadata is itself a JSON-encoded string, so it is unmarshalled twice.
func parseOkMetadata(body []byte) (okMetadata, error) {
	var md okMetadata
	m := dataOptionsRe.FindSubmatch(body)
	if m == nil {
		return md, errors.New("no data-options attribute")
	}
	raw := html.UnescapeString(string(m[1]))
	var opts struct {
		Flashvars struct {
			Metadata json.RawMessage `json:"metadata"`
		} `json:"flashvars"`
	}
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		return md, fmt.Errorf("data-options json: %w", err)
	}
	if len(opts.Flashvars.Metadata) == 0 {
		return md, errors.New("no flashvars.metadata")
	}
	// metadata is usually a JSON-encoded string; try string-decode first.
	var mdStr string
	if err := json.Unmarshal(opts.Flashvars.Metadata, &mdStr); err == nil {
		if err := json.Unmarshal([]byte(mdStr), &md); err != nil {
			return md, fmt.Errorf("metadata string json: %w", err)
		}
		return md, nil
	}
	if err := json.Unmarshal(opts.Flashvars.Metadata, &md); err != nil {
		return md, fmt.Errorf("metadata object json: %w", err)
	}
	return md, nil
}

// Compile-time assertion: OkruExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*OkruExtractor)(nil)
