// resolver.go — animepahe-resolver sidecar HTTP client.
//
// Phase 27 SCRAPER-HEAL-30: the Go parser no longer talks to the animepahe
// upstream directly. The stealth-Chromium sidecar (`services/animepahe-
// resolver/`, owned by Plan 27-01) handles the DDoS-Guard + browser-
// challenge stack; this client is a thin HTTP wrapper that POSTs the
// (search | release | play) calls and decodes responses.
//
// Mapping conventions:
//
//   - 200 → unmarshal JSON / return HTML body
//   - 404 → domain.ErrNotFound (anime / episode / play page missing upstream)
//   - 502 → domain.ErrProviderDown ("animepahe-resolver: stealth challenge
//     un-solvable" — the sidecar maps upstream block/captcha/timeout here)
//   - other non-200 → domain.ErrProviderDown
//
// Response body limits mirror client.go's pre-existing `maxBodyAPI` (4 MiB)
// for JSON shapes and `maxBodyHTML` (2 MiB) for the play HTML.
package animepahe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// kwikReferer is the public animepahe origin sent as the Referer header to
// the Kwik extractor; aligns with Phase 27 D2 (animepahe.pw exclusive). The
// Kwik upstream requires the parent-site Referer chain — this constant
// replaces the deleted Provider.baseURL field in GetStream.
const kwikReferer = "https://animepahe.pw/"

// resolverClient is the HTTP client to the animepahe-resolver sidecar.
//
// Field rules:
//
//   - baseURL: trimmed of trailing slash. Defaults to
//     `http://animepahe-resolver:3000` in main.go.
//   - http: shared BaseHTTPClient so the per-host RPS limiter on
//     `animepahe-resolver` is enforced (set in main.go).
type resolverClient struct {
	baseURL string
	http    *domain.BaseHTTPClient
}

// newResolverClient constructs a resolverClient. `baseURL` is trimmed; a
// nil HTTP client is a programmer error caught at construction time in
// New().
func newResolverClient(baseURL string, hc *domain.BaseHTTPClient) *resolverClient {
	return &resolverClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    hc,
	}
}

// Search GETs /search?q=<q> on the sidecar; returns the parsed searchResponse.
func (r *resolverClient) Search(ctx context.Context, q string) (*searchResponse, error) {
	u := fmt.Sprintf("%s/search?q=%s", r.baseURL, url.QueryEscape(q))
	resp, err := r.http.Get(ctx, u)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animepahe-resolver: /search fetch")
	}
	defer drainAndClose(resp.Body)

	if err := mapStatus(resp.StatusCode, "search"); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animepahe-resolver: /search read body")
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, domain.WrapExtractFailed(err, "animepahe-resolver: /search decode")
	}
	return &sr, nil
}

// Release GETs /release?session=<animeSession>&page=<page> on the sidecar.
func (r *resolverClient) Release(ctx context.Context, animeSession string, page int) (*releaseResponse, error) {
	u := fmt.Sprintf("%s/release?session=%s&page=%d", r.baseURL, url.QueryEscape(animeSession), page)
	resp, err := r.http.Get(ctx, u)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animepahe-resolver: /release fetch")
	}
	defer drainAndClose(resp.Body)

	if err := mapStatus(resp.StatusCode, "release"); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyAPI))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animepahe-resolver: /release read body")
	}
	var rr releaseResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, domain.WrapExtractFailed(err, "animepahe-resolver: /release decode")
	}
	return &rr, nil
}

// Play GETs /play?animeSession=<a>&episodeSession=<e> on the sidecar; returns
// the raw HTML body (bounded by maxBodyHTML) for goquery parsing in
// Provider.ListServers.
func (r *resolverClient) Play(ctx context.Context, animeSession, episodeSession string) (string, error) {
	u := fmt.Sprintf("%s/play?animeSession=%s&episodeSession=%s",
		r.baseURL,
		url.QueryEscape(animeSession),
		url.QueryEscape(episodeSession),
	)
	resp, err := r.http.Get(ctx, u)
	if err != nil {
		return "", domain.WrapProviderDown(err, "animepahe-resolver: /play fetch")
	}
	defer drainAndClose(resp.Body)

	if err := mapStatus(resp.StatusCode, "play"); err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyHTML))
	if err != nil {
		return "", domain.WrapProviderDown(err, "animepahe-resolver: /play read body")
	}
	return string(body), nil
}

// mapStatus converts a non-200 status into the canonical domain error.
//
// op is short ("search", "release", "play") and used in the wrap message so
// the orchestrator's error log surfaces WHICH endpoint failed.
func mapStatus(code int, op string) error {
	switch {
	case code == http.StatusOK:
		return nil
	case code == http.StatusNotFound:
		return domain.WrapNotFound(
			fmt.Errorf("status %d", code),
			fmt.Sprintf("animepahe-resolver: /%s not found upstream", op),
		)
	case code == http.StatusBadGateway:
		return domain.WrapProviderDown(
			fmt.Errorf("status %d", code),
			fmt.Sprintf("animepahe-resolver: /%s stealth challenge un-solvable", op),
		)
	default:
		return domain.WrapProviderDown(
			fmt.Errorf("status %d", code),
			fmt.Sprintf("animepahe-resolver: /%s non-200", op),
		)
	}
}

// drainAndClose drains and closes the response body. Centralized so each
// method's defer is one line.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
