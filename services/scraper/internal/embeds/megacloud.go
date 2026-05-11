// Package embeds wraps third-party embed extractors. Each extractor is a
// thin HTTP client around an external process (sidecar) that holds the
// decryption / scraping logic.
//
// SCRAPER-FOUND-08: MegacloudClient is the first concrete EmbedExtractor and
// HTTP-wraps the existing Node sidecar at docker/megacloud-extractor/server.js.
// No decryption logic lives in Go — the sidecar continues to own that.
package embeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// megacloudKnownHosts lists every embed host the megacloud-extractor sidecar
// is expected to handle. New hosts are added here as Aniyomi / HiAnime
// upstream rotate.
//
// Match policy: case-insensitive equality OR strict subdomain (i.e. a host
// whose etld+N hierarchy ends in ".<known>"). The strict subdomain check
// prevents "evilmegacloud.tv" from matching megacloud.tv.
var megacloudKnownHosts = []string{
	"megacloud.tv",
	"megacloud.blog",
	"megacloud.club",
	"megaup.live",
	"megaup.cc",
}

// defaultMegacloudTimeout is the per-request timeout against the sidecar.
// Per the plan: the sidecar internally does up to four HTTP hops (embed page,
// /getSources, decryption key, optional retry) at 15s each — Go-side timeout
// matches the sidecar's own setTimeout(15000).
const defaultMegacloudTimeout = 15 * time.Second

// maxSidecarBody caps the response body the megacloud-extractor sidecar may
// stream back. Real sidecar responses are <50 KiB in practice; a misbehaving
// sidecar streaming gigabytes would OOM the scraper without this guard.
// See REVIEW.md CR-03.
const maxSidecarBody = 2 << 20 // 2 MiB

// MegacloudClient is a domain.EmbedExtractor that delegates to the
// megacloud-extractor sidecar over HTTP. The Go side does NO decryption; it
// is purely an HTTP wrapper + DTO translator.
type MegacloudClient struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

// NewMegacloudClient constructs a client targeting the sidecar at baseURL
// (e.g. "http://megacloud-extractor:3200"). If timeout is zero, the default
// (15s, matching the sidecar's internal timeout) applies.
//
// The returned client uses an independent http.Client (NOT domain.BaseHTTPClient)
// because the sidecar is a sibling service we trust: there is no per-host
// rate limit, retry, or cookie-jar concern. The trust boundary is the
// docker-compose network, not the public internet.
func NewMegacloudClient(baseURL string, timeout time.Duration) *MegacloudClient {
	if timeout <= 0 {
		timeout = defaultMegacloudTimeout
	}
	return &MegacloudClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
		timeout:    timeout,
	}
}

// Name implements domain.EmbedExtractor.
func (c *MegacloudClient) Name() string { return "megacloud" }

// Matches reports whether embedURL points to a megacloud-family host.
// See megacloudKnownHosts for the list. The check is host-only — substrings
// in the path or query are NOT matched (so a URL like
// "https://example.com/megacloud-imposter" returns false).
func (c *MegacloudClient) Matches(embedURL string) bool {
	u, err := url.Parse(embedURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	for _, known := range megacloudKnownHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// sidecarResponse mirrors the JSON shape returned by
// docker/megacloud-extractor/server.js on a 200. Field names follow the
// sidecar's output convention (url/lang) rather than our domain DTO (URL/Label).
type sidecarResponse struct {
	Sources []struct {
		URL    string `json:"url"`
		Type   string `json:"type"`
		IsM3U8 bool   `json:"isM3U8"`
	} `json:"sources"`
	Tracks []struct {
		URL     string `json:"url"`
		Lang    string `json:"lang"`
		Default bool   `json:"default"`
	} `json:"tracks"`
	Intro struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"intro"`
	Outro struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"outro"`
}

// sidecarError is the error-body shape the sidecar emits on non-2xx.
type sidecarError struct {
	Error string `json:"error"`
}

// Extract fetches the playable Stream for embedURL via the sidecar.
// Caller-supplied headers (e.g. Referer for AnimeKai Phase 19) are forwarded
// on the request TO the sidecar; the sidecar manages its own headers to the
// upstream embed page.
//
// Errors are always wrapped as domain.ErrExtractFailed so the orchestrator
// failover loop can match via errors.Is(err, domain.ErrExtractFailed).
func (c *MegacloudClient) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	target := fmt.Sprintf("%s/extract?url=%s", c.baseURL, url.QueryEscape(embedURL))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "megacloud: build sidecar request")
	}
	// Forward caller-supplied headers verbatim. We do NOT inject a default
	// Referer here — that's the sidecar's responsibility (it hardcodes
	// "https://aniwatchtv.to/" today for HiAnime-flavor pages).
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, domain.WrapExtractFailed(err, "megacloud: sidecar request failed")
	}
	defer func() {
		// Drain any unread bytes so the keep-alive connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// REVIEW.md CR-03: do NOT discard the io.ReadAll error. A truncated body
	// from a network blip / sidecar OOM-mid-response would otherwise surface
	// as a misleading "decode" error instead of the actual transport failure.
	// Also cap the body so a misbehaving sidecar can't OOM the scraper.
	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSidecarBody))
	if readErr != nil {
		// Body read failure is a transport-level issue — the sidecar IS up,
		// but the network between us was disrupted. Surface as ProviderDown so
		// upstream incident dashboards observe it correctly.
		return nil, domain.WrapProviderDown(readErr, "megacloud: read sidecar response body")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to decode the JSON error body; fall back to raw status if it
		// doesn't parse.
		var errBody sidecarError
		_ = json.Unmarshal(bodyBytes, &errBody)
		cause := errBody.Error
		if cause == "" {
			cause = strings.TrimSpace(string(bodyBytes))
		}
		if cause == "" {
			cause = http.StatusText(resp.StatusCode)
		}
		return nil, domain.WrapExtractFailed(
			errors.New(cause),
			fmt.Sprintf("megacloud: sidecar status %d", resp.StatusCode),
		)
	}

	var sr sidecarResponse
	if err := json.Unmarshal(bodyBytes, &sr); err != nil {
		return nil, domain.WrapExtractFailed(err, "megacloud: decode sidecar response")
	}

	return convertSidecarToStream(sr), nil
}

// convertSidecarToStream translates the sidecar's JSON shape into our
// domain.Stream DTO. Intro/Outro are only set when End > 0 (the sidecar
// always emits {Start:0,End:0} when the upstream has no markers).
func convertSidecarToStream(sr sidecarResponse) *domain.Stream {
	stream := &domain.Stream{}

	if len(sr.Sources) > 0 {
		stream.Sources = make([]domain.Source, 0, len(sr.Sources))
		for _, s := range sr.Sources {
			srcType := s.Type
			if srcType == "" {
				if s.IsM3U8 {
					srcType = "hls"
				} else {
					srcType = "mp4"
				}
			}
			stream.Sources = append(stream.Sources, domain.Source{
				URL:  s.URL,
				Type: srcType,
			})
		}
	}

	if len(sr.Tracks) > 0 {
		stream.Tracks = make([]domain.Track, 0, len(sr.Tracks))
		for _, t := range sr.Tracks {
			stream.Tracks = append(stream.Tracks, domain.Track{
				File:    t.URL,
				Label:   t.Lang,
				Kind:    "captions",
				Default: t.Default,
			})
		}
	}

	if sr.Intro.End > 0 {
		stream.Intro = &domain.TimeRange{Start: sr.Intro.Start, End: sr.Intro.End}
	}
	if sr.Outro.End > 0 {
		stream.Outro = &domain.TimeRange{Start: sr.Outro.Start, End: sr.Outro.End}
	}

	return stream
}
