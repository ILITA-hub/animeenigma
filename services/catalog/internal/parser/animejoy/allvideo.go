package animejoy

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// allVideoFallbackReferer is used only when the resolved get_file URL can't be
// parsed for its origin. The real Referer is derived per-resolve from the
// get_file URL itself (see deriveAllVideoReferer): the get_file host 302-redirects
// to a*.filevideo1.com/remote_control.php, which 403s a fsst.online Referer but
// serves the mp4 when the Referer is the get_file origin (or absent). The proxy
// keeps this Referer across the get_file→filevideo1 302, so it MUST be the value
// the FINAL hop accepts — NOT the fsst embed origin (smoke-tested 2026-06-30).
const allVideoFallbackReferer = "https://www.incvideo1.online/"

// deriveAllVideoReferer returns the Referer the AllVideo CDN chain expects,
// taken from the resolved get_file URL's own origin. The get_file host is
// incvideo1.online today, but the network rotates mirrors, so deriving the
// origin (rather than hardcoding it) keeps the Referer correct across rotation.
func deriveAllVideoReferer(getFileURL string) string {
	if u, err := url.Parse(getFileURL); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host + "/"
	}
	return allVideoFallbackReferer
}

// allVideoFile is one rendition from the playerjs file:"…" list.
type allVideoFile struct {
	Quality string // "360p" / "720p" / "1080p"
	URL     string // absolute get_file mp4 URL (ends ".mp4/")
}

// allVideoFileCfgRe captures the whole playerjs file:"…" value:
//
//	file:"[360p]https://…/726858_360p.mp4/,[720p]https://…/726858_720p.mp4/,[1080p]https://…/726858.mp4/"
//
// The value is a single double-quoted string with no embedded quotes, so [^"]* is
// safe.
var allVideoFileCfgRe = regexp.MustCompile(`(?is)\bfile\s*:\s*"([^"]*)"`)

// allVideoQualityRe splits a file-config value into its [NNNp]URL renditions. The
// URLs carry no commas (hex hash + numeric path segments), so each "[quality]"
// tag starts a new rendition and the URL runs up to the next "[" or end-of-string.
var allVideoQualityRe = regexp.MustCompile(`(?is)\[(\d+p)\]\s*([^,\[]+)`)

// parseAllVideoFiles extracts the rendition list from an AllVideo (incvideo1)
// embed page. PURE: takes the raw HTML bytes, returns the renditions in
// file-config order. Returns an error (never panics) when no file:"…" config or
// no recognizable [NNNp] rendition is present.
func parseAllVideoFiles(body []byte) ([]allVideoFile, error) {
	cfg := allVideoFileCfgRe.FindSubmatch(body)
	if cfg == nil {
		return nil, fmt.Errorf("animejoy: allvideo: no playerjs file config found")
	}
	value := string(cfg[1])

	var out []allVideoFile
	for _, m := range allVideoQualityRe.FindAllStringSubmatch(value, -1) {
		quality := strings.ToLower(strings.TrimSpace(m[1]))
		u := strings.TrimSpace(m[2])
		if u == "" {
			continue
		}
		out = append(out, allVideoFile{Quality: quality, URL: u})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("animejoy: allvideo: no renditions in file config")
	}
	return out, nil
}

// qualityRank maps a quality tag to a sortable height; unknown tags rank 0 so a
// recognizable rendition always beats them.
func qualityRank(quality string) int {
	digits := strings.TrimRight(strings.ToLower(strings.TrimSpace(quality)), "p")
	return atoiSafe(digits)
}

// pickBestAllVideo selects the highest-quality rendition (1080 > 720 > 360).
// Returns ok=false for an empty list. Ties keep the first-seen entry (stable).
func pickBestAllVideo(list []allVideoFile) (allVideoFile, bool) {
	best := allVideoFile{}
	bestRank := -1
	for _, f := range list {
		if r := qualityRank(f.Quality); r > bestRank {
			bestRank = r
			best = f
		}
	}
	if bestRank < 0 {
		return allVideoFile{}, false
	}
	return best, true
}

// ResolveAllVideo resolves an AllVideo leg to the best progressive MP4. Thin HTTP
// wrapper: GET the embed page (fsst.online/embed/<id>/, which 301-redirects to
// incvideo1.online — followed by the default client), parse the playerjs file
// list, and pick the highest rendition. embedURL is the fsst.online embed URL
// found in the playlist's data-file.
//
// The proxy MUST replay the returned Referer (the get_file origin, e.g.
// https://www.incvideo1.online/) when fetching the returned URL: the get_file
// host 302-redirects to a*.filevideo1.com, which 403s a fsst.online Referer.
func (c *Client) ResolveAllVideo(ctx context.Context, embedURL string) (ResolvedLeg, error) {
	target := strings.TrimSpace(embedURL)
	if target == "" {
		return ResolvedLeg{}, fmt.Errorf("animejoy: ResolveAllVideo called with empty embed URL")
	}

	// AnimeJoy is the page that embeds the fsst player; the embed keys on it.
	body, err := c.getBody(ctx, target, map[string]string{"Referer": DefaultBaseURL + "/"})
	if err != nil {
		return ResolvedLeg{}, fmt.Errorf("animejoy: allvideo embed request: %w", err)
	}

	list, err := parseAllVideoFiles(body)
	if err != nil {
		return ResolvedLeg{}, err
	}
	best, ok := pickBestAllVideo(list)
	if !ok {
		return ResolvedLeg{}, fmt.Errorf("animejoy: allvideo: no rendition to pick")
	}
	return ResolvedLeg{
		URL:     best.URL,
		Referer: deriveAllVideoReferer(best.URL),
		Quality: best.Quality,
	}, nil
}
