// transport.go — animepahe.pw upstream transport.
//
// Revival (2026-06-26): the dedicated animepahe-resolver sidecar (retired
// 2026-06-24, see AnimepaheSidecarRetired) is GONE. Discovery now hits
// animepahe.pw DIRECTLY through the Camoufox stealth-scraper sidecar's warm
// /fetch session, which SOLVES animepahe.pw's Cloudflare managed (interactive
// Turnstile) challenge — proven live on this server's own datacenter IP (~10s
// to cf_clearance, no residential proxy needed). Owner-locked Approach 2: Go
// builds every URL and parses every response; the Python sidecar is a pure
// browser-fetch execution layer.
//
//   - search:  <base>/api?m=search&q=<q>                            → searchResponse
//   - release: <base>/api?m=release&id=<s>&sort=episode_asc&page=<n> → releaseResponse
//   - play:    <base>/play/<animeSession>/<episodeSession>           → HTML (kwik embeds)
//
// When the DB engine is "browser" the GET routes through BrowserFetch (the
// sidecar /fetch closure injected by main.go). engine=http is a DEGRADED
// fallback shape only — a curl-class client cannot pass the Cloudflare
// challenge, so plain GETs 403 — kept so the provider still behaves when the
// browser route is off and so unit tests can drive the parser against a local
// httptest server.
package animepahe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// kwikReferer is the public animepahe origin sent as the Referer to the Kwik
// extractor; the Kwik upstream requires the parent-site Referer chain.
const kwikReferer = "https://animepahe.pw/"

// defaultBaseURL is used when the DB roster supplies no base_url override.
const defaultBaseURL = "https://animepahe.pw"

// BrowserFetchFunc routes a discovery GET through the Camoufox stealth-scraper
// sidecar's warm /fetch session, returning (upstreamStatus, body, err). Mirrors
// the nineanime closure injected in main.go.
type BrowserFetchFunc func(ctx context.Context, provider, url string) (int, []byte, error)

// browserEnabled reports whether discovery should route through the sidecar
// (DB engine="browser"). Requires both the live gate and the fetch closure to
// be wired — a partial wiring degrades to the plain-HTTP fallback rather than
// panicking on a nil closure.
func (p *Provider) browserEnabled() bool {
	return p.useBrowser != nil && p.browserFetch != nil && p.useBrowser()
}

// httpGetBody fetches urlStr's body (capped at cap bytes), routing through the
// browser sidecar when enabled and falling back to a plain HTTP GET otherwise.
// op is a short tag ("search"/"release"/"play") used in the error wrap message.
func (p *Provider) httpGetBody(ctx context.Context, urlStr string, cap int64, op string) ([]byte, error) {
	if p.browserEnabled() {
		status, body, err := p.browserFetch(ctx, providerName, urlStr)
		if err != nil {
			// The sidecar client already wrapped this (ErrNotFound /
			// ErrProviderDown / provider-wedged) — pass it through unchanged.
			return nil, err
		}
		if err := mapStatus(status, op); err != nil {
			return nil, err
		}
		if int64(len(body)) > cap {
			body = body[:cap]
		}
		return body, nil
	}
	resp, err := p.http.Get(ctx, urlStr)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animepahe: "+op+" fetch")
	}
	defer drainAndClose(resp.Body)
	if err := mapStatus(resp.StatusCode, op); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cap))
	if err != nil {
		return nil, domain.WrapProviderDown(err, "animepahe: "+op+" read body")
	}
	return body, nil
}

// Search GETs <base>/api?m=search&q=<q> and parses the searchResponse.
func (p *Provider) Search(ctx context.Context, q string) (*searchResponse, error) {
	u := fmt.Sprintf("%s/api?m=search&q=%s", p.baseURL, url.QueryEscape(q))
	body, err := p.httpGetBody(ctx, u, maxBodyAPI, "search")
	if err != nil {
		return nil, err
	}
	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, domain.WrapExtractFailed(err, "animepahe: /search decode")
	}
	return &sr, nil
}

// Release GETs <base>/api?m=release&id=<animeSession>&sort=episode_asc&page=<page>.
func (p *Provider) Release(ctx context.Context, animeSession string, page int) (*releaseResponse, error) {
	u := fmt.Sprintf("%s/api?m=release&id=%s&sort=episode_asc&page=%d",
		p.baseURL, url.QueryEscape(animeSession), page)
	body, err := p.httpGetBody(ctx, u, maxBodyAPI, "release")
	if err != nil {
		return nil, err
	}
	var rr releaseResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, domain.WrapExtractFailed(err, "animepahe: /release decode")
	}
	return &rr, nil
}

// Play GETs <base>/play/<animeSession>/<episodeSession> and returns raw HTML
// (bounded by maxBodyHTML) for goquery parsing in Provider.ListServers.
func (p *Provider) Play(ctx context.Context, animeSession, episodeSession string) (string, error) {
	u := fmt.Sprintf("%s/play/%s/%s",
		p.baseURL, url.PathEscape(animeSession), url.PathEscape(episodeSession))
	body, err := p.httpGetBody(ctx, u, maxBodyHTML, "play")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// mapStatus converts a non-200 upstream status into the canonical domain error.
// op is short ("search"/"release"/"play") and used in the wrap message so the
// orchestrator's error log surfaces WHICH endpoint failed.
func mapStatus(code int, op string) error {
	switch {
	case code == http.StatusOK:
		return nil
	case code == http.StatusNotFound:
		return domain.WrapNotFound(
			fmt.Errorf("status %d", code),
			fmt.Sprintf("animepahe: /%s not found upstream", op),
		)
	case code == http.StatusForbidden, code == http.StatusServiceUnavailable:
		// 403/503 from animepahe.pw means the Cloudflare challenge was not
		// solved (or the edge is blocking) — provider-down so the orchestrator
		// fails over.
		return domain.WrapProviderDown(
			fmt.Errorf("status %d", code),
			fmt.Sprintf("animepahe: /%s challenge/blocked upstream", op),
		)
	default:
		// Includes the abnormal code==0 case (an in-page fetch that returned an
		// opaque/zero status with no challenge marker): treat it as provider-down
		// so the orchestrator fails over. (nineanime instead passes 0 through as
		// success and lets it fail downstream as ExtractFailed — both fail over;
		// only the error kind differs.)
		return domain.WrapProviderDown(
			fmt.Errorf("status %d", code),
			fmt.Sprintf("animepahe: /%s non-200", op),
		)
	}
}

// drainAndClose drains and closes the response body. Centralized so each
// fallback-path defer is one line.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
