// megaplay.go — MegaplayExtractor for the megaplay.buzz HLS player that the
// 9anime.me.uk brand-jack migrated its popular catalog to (2026-05/06).
//
// Upstream chain (verified live 2026-06-01):
//
//	9anime episode <iframe src="https://1anime.site/megaplay/stream/s-2/<id>/sub">
//	  → wrapper page nests <iframe src="https://megaplay.buzz/stream/s-2/<id>/sub">
//	  → player page exposes data-id="<realId>"   (NOT the path id — see note)
//	  → GET https://megaplay.buzz/stream/getSources?id=<realId>
//	       Referer: <player page URL>, X-Requested-With: XMLHttpRequest
//	  → JSON {"sources":{"file":"<master.m3u8>"},"tracks":[…],"intro":…,"outro":…}
//
// CRITICAL id semantics: getSources accepts ANY numeric id and returns *a*
// stream, so reusing the path id (94554) silently returns the WRONG episode.
// The canonical id is the data-id parsed from the player page — that is the
// value megaplay's own JS feeds to getSources.
//
// The master.m3u8 lives on cdn.mewstream.buzz (stable), but its child
// playlists reference segments on a rotating pool of throwaway
// .click/.buzz/.club domains. The streaming HLS proxy handles those via the
// HMAC provenance token (libs/videoutils/proxy.go) — this extractor only
// needs to return the master URL + the megaplay.buzz Referer the CDN
// enforces (master.m3u8 returns 403 without it).
//
// Pure stdlib — plaintext JSON, NO decryption (unlike megacloud). SSRF
// mitigation: host-allowlist Matches, body caps, absolute-URL validation,
// JSON-shape validation.
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
	defaultMegaplayHTTPTimeout = 15 * time.Second
	maxMegaplayBody            = 2 << 20 // 2 MiB DoS guard.

	// megaplayReferer — value returned in Stream.Headers so the HLS proxy
	// replays it when fetching master/variant playlists. cdn.mewstream.buzz
	// returns 403 on the master.m3u8 without it (verified 2026-06-01). Also
	// the default outgoing Referer for the player + getSources hops.
	megaplayReferer = "https://megaplay.buzz/"

	// Selector labels for parser_zero_match_total (low-cardinality).
	selectorMegaplayWrapperIframe = "megaplay_wrapper_iframe"
	selectorMegaplayDataID        = "megaplay_data_id"
	selectorMegaplaySourcesJSON   = "megaplay_sources_json"
	selectorMegaplayBodyRead      = "megaplay_body_read"
)

// megaplayExactHosts match by host EQUALITY only.
//   - `1anime.site` is the bare-apex wrapper origin embedded by 9anime; it must
//     NOT match its subdomain `my.1anime.site`, which is the legacy direct-MP4
//     host the nineanime provider still handles inline (a different service).
//   - `gogoanime.me.uk` is the gogoanime (gogoanimes.fi mirror) `newplayer.php`
//     wrapper, which nests the SAME `megaplay.buzz` player iframe — so Extract's
//     generic wrapper path resolves it identically (2026-06-05 gogoanime revival).
var megaplayExactHosts = []string{"1anime.site", "gogoanime.me.uk"}

// megaplaySubdomainHosts match by host equality OR strict subdomain.
// `megaplay.buzz` is the real player + getSources origin and may front via
// CDN subdomains. `vidwish.live` is the API-identical sister player the
// gogoanime/9anime chains migrated to (2026-06) — same data-id attribute,
// same /stream/getSources endpoint, same JSON shape (sources.file + tracks +
// intro/outro). Extract handles it identically; only the active Referer
// differs (derived per-request from the resolved player origin).
var megaplaySubdomainHosts = []string{"megaplay.buzz", "vidwish.live"}

// megaplayNestedIframeRegex pulls the player URL out of the thin wrapper page
// (`<iframe src="https://megaplay.buzz/…">` or the API-identical
// `<iframe src="https://vidwish.live/…">` the chain migrated to in 2026-06).
var megaplayNestedIframeRegex = regexp.MustCompile(`<iframe[^>]+src="(https?://[^"]*(?:megaplay\.buzz|vidwish\.live)/[^"]+)"`)

// megaplayDataIDRegex extracts the canonical numeric stream id from the
// player page (`data-id="19015"`).
var megaplayDataIDRegex = regexp.MustCompile(`data-id="(\d+)"`)

// MegaplayExtractor resolves a megaplay.buzz HLS stream from a 1anime.site or
// megaplay.buzz embed URL. Pure stdlib — no goja, no sidecar.
type MegaplayExtractor struct {
	http    *http.Client
	timeout time.Duration
}

// NewMegaplayExtractor returns a MegaplayExtractor with default timeouts and an
// UNRECORDED transport (back-compat, zero-arg). Use NewRecordingMegaplayExtractor
// in production so the megaplay.buzz / 1anime.site / getSources hops emit egress
// effects (WR-07).
func NewMegaplayExtractor() *MegaplayExtractor {
	return NewRecordingMegaplayExtractor(nil)
}

// NewRecordingMegaplayExtractor builds a MegaplayExtractor and lets the owning
// service WRAP the http.Client transport for egress recording WITHOUT this leaf
// module importing the tracing module (mirrors kodikextract.NewRecordingClient,
// T-02-LEAF / WR-07). wrap may be nil (no recording, current behavior). Without
// this seam the extractor's three outbound hops (1anime.site wrapper fetch,
// megaplay.buzz player fetch, getSources XHR) bypass the shared egress recorder
// and emit no effect rows — the blind spot confirmed live (zero megaplay.buzz
// rows in ClickHouse). Callers pass:
//
//	embeds.NewRecordingMegaplayExtractor(func(base http.RoundTripper) http.RoundTripper {
//		return tracing.WrapTransport(base)
//	})
func NewRecordingMegaplayExtractor(wrap func(base http.RoundTripper) http.RoundTripper) *MegaplayExtractor {
	var rt http.RoundTripper = http.DefaultTransport
	if wrap != nil {
		rt = wrap(rt)
	}
	return &MegaplayExtractor{
		http:    &http.Client{Timeout: defaultMegaplayHTTPTimeout, Transport: rt},
		timeout: defaultMegaplayHTTPTimeout,
	}
}

// Name implements domain.EmbedExtractor.
func (e *MegaplayExtractor) Name() string { return "megaplay" }

// Hosts implements embeds.HostingExtractor — returns the lowercase host allowlist.
func (e *MegaplayExtractor) Hosts() []string {
	out := make([]string, 0, len(megaplayExactHosts)+len(megaplaySubdomainHosts))
	out = append(out, megaplayExactHosts...)
	out = append(out, megaplaySubdomainHosts...)
	return out
}

// Matches reports whether embedURL points to the bare `1anime.site` wrapper
// or `megaplay.buzz` (host or strict subdomain). It deliberately does NOT
// match `my.1anime.site` (legacy MP4 host — see megaplayExactHosts).
func (e *MegaplayExtractor) Matches(embedURL string) bool {
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
	for _, known := range megaplayExactHosts {
		if host == known {
			return true
		}
	}
	for _, known := range megaplaySubdomainHosts {
		if host == known || strings.HasSuffix(host, "."+known) {
			return true
		}
	}
	return false
}

// Extract resolves embedURL to a playable HLS Stream. Transport error / 5xx →
// ErrProviderDown; 4xx / regex miss / malformed JSON / non-absolute URL →
// ErrExtractFailed.
func (e *MegaplayExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !e.Matches(embedURL) {
		return nil, domain.WrapExtractFailed(
			errors.New("host not in allowlist"),
			"megaplay: Matches gate",
		)
	}

	// (1) Resolve the megaplay.buzz player URL. A 1anime.site wrapper nests
	// it in an <iframe>; a megaplay.buzz URL is already the player.
	playerURL := embedURL
	if u, _ := url.Parse(embedURL); u != nil && !strings.Contains(strings.ToLower(u.Hostname()), "megaplay.buzz") {
		body, err := e.fetch(ctx, embedURL, megaplayReferer, headers)
		if err != nil {
			return nil, err
		}
		m := megaplayNestedIframeRegex.FindSubmatch(body)
		if len(m) < 2 {
			metrics.ParserZeroMatchTotal.WithLabelValues("megaplay", selectorMegaplayWrapperIframe).Inc()
			return nil, domain.WrapExtractFailed(
				errors.New("no nested megaplay.buzz iframe in wrapper"),
				"megaplay: wrapper iframe regex",
			)
		}
		playerURL = string(m[1])
	}

	// Derive the Referer the ACTIVE player domain expects from the resolved
	// player origin (megaplay.buzz OR the API-identical vidwish.live). Both
	// the player page and the getSources XHR — plus the CDN that ultimately
	// serves master.m3u8 — enforce a same-origin Referer, so hardcoding
	// megaplay.buzz would 403 every vidwish.live stream. For the megaplay
	// path this derives "https://megaplay.buzz/" (identical to the old
	// megaplayReferer constant), so that path is unchanged.
	pu, perr := url.Parse(playerURL)
	if perr != nil {
		return nil, domain.WrapExtractFailed(perr, "megaplay: parse player url")
	}
	playerReferer := pu.Scheme + "://" + pu.Host + "/"

	// (2) Fetch the player page and parse the canonical data-id.
	playerBody, err := e.fetch(ctx, playerURL, playerReferer, headers)
	if err != nil {
		return nil, err
	}
	idm := megaplayDataIDRegex.FindSubmatch(playerBody)
	if len(idm) < 2 {
		metrics.ParserZeroMatchTotal.WithLabelValues("megaplay", selectorMegaplayDataID).Inc()
		return nil, domain.WrapExtractFailed(
			errors.New(`no data-id="…" on player page`),
			"megaplay: data-id regex",
		)
	}
	dataID := string(idm[1])

	// (3) GET getSources?id=<dataID> off the player origin. X-Requested-With
	// is required (the endpoint 404s on a plain navigation).
	sourcesURL := fmt.Sprintf("%s://%s/stream/getSources?id=%s", pu.Scheme, pu.Host, url.QueryEscape(dataID))
	srcBody, err := e.fetchXHR(ctx, sourcesURL, playerReferer, headers)
	if err != nil {
		return nil, err
	}

	var gs struct {
		Sources struct {
			File string `json:"file"`
		} `json:"sources"`
		Tracks []struct {
			File    string `json:"file"`
			Label   string `json:"label"`
			Kind    string `json:"kind"`
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
	if err := json.Unmarshal(srcBody, &gs); err != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues("megaplay", selectorMegaplaySourcesJSON).Inc()
		return nil, domain.WrapExtractFailed(err, "megaplay: decode getSources json")
	}
	if !strings.HasPrefix(gs.Sources.File, "http://") && !strings.HasPrefix(gs.Sources.File, "https://") {
		metrics.ParserZeroMatchTotal.WithLabelValues("megaplay", selectorMegaplaySourcesJSON).Inc()
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("non-absolute sources.file %q", gs.Sources.File),
			"megaplay: source url shape",
		)
	}

	stream := &domain.Stream{
		Sources: []domain.Source{{URL: gs.Sources.File, Type: "hls", Quality: "auto"}},
		Headers: map[string]string{"Referer": playerReferer},
	}
	for _, t := range gs.Tracks {
		if !strings.HasPrefix(t.File, "http://") && !strings.HasPrefix(t.File, "https://") {
			continue // skip relative/garbage subtitle URLs rather than fail the whole stream
		}
		kind := t.Kind
		if kind == "" {
			kind = "captions"
		}
		stream.Tracks = append(stream.Tracks, domain.Track{
			File:    t.File,
			Label:   t.Label,
			Kind:    kind,
			Default: t.Default,
		})
	}
	if gs.Intro.End > 0 {
		stream.Intro = &domain.TimeRange{Start: gs.Intro.Start, End: gs.Intro.End}
	}
	if gs.Outro.End > 0 {
		stream.Outro = &domain.TimeRange{Start: gs.Outro.Start, End: gs.Outro.End}
	}
	return stream, nil
}

// fetch GETs url with the given Referer (caller headers win if they set one),
// returning the capped body. 5xx → ErrProviderDown, 4xx → ErrExtractFailed.
func (e *MegaplayExtractor) fetch(ctx context.Context, url, referer string, headers http.Header) ([]byte, error) {
	return e.do(ctx, url, referer, false, headers)
}

// fetchXHR is fetch with X-Requested-With: XMLHttpRequest (megaplay's
// getSources endpoint requires it).
func (e *MegaplayExtractor) fetchXHR(ctx context.Context, url, referer string, headers http.Header) ([]byte, error) {
	return e.do(ctx, url, referer, true, headers)
}

func (e *MegaplayExtractor) do(ctx context.Context, target, referer string, xhr bool, headers http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "megaplay: build request")
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", referer)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0")
	}
	if xhr {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "megaplay: fetch")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(fmt.Errorf("upstream %d", resp.StatusCode), "megaplay: HTTP status")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domain.WrapExtractFailed(fmt.Errorf("upstream %d", resp.StatusCode), "megaplay: HTTP status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMegaplayBody))
	if err != nil {
		metrics.ParserZeroMatchTotal.WithLabelValues("megaplay", selectorMegaplayBodyRead).Inc()
		return nil, domain.WrapProviderDown(err, "megaplay: read body")
	}
	return body, nil
}

// Compile-time assertion: MegaplayExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*MegaplayExtractor)(nil)
