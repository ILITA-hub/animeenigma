package animejoy

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const (
	// sibnetVideoBase is the absolute origin the shell page's relative /v/ path is
	// resolved against — iv.sibnet.ru/shell.php 301-redirects here, and the mp4 is
	// served from this same host.
	sibnetVideoBase = "https://video.sibnet.ru"
	// sibnetReferer is the Referer the CDN expects when the /v/<hash>/<id>.mp4 is
	// fetched (the shell's own origin), NOT animejoy.ru.
	sibnetReferer = "https://video.sibnet.ru/"
)

// sibnetSrcRe pulls the progressive mp4 path out of the shell's player.src call:
//
//	player.src([{src: "/v/6462ad80a5d17783fef2c185bd5eab61/5263892.mp4", type: "video/mp4"},]);
//
// We bind to the player.src(...) {src: "..."} shape (rather than any quoted /v/
// string) so unrelated assets never match, and capture the video.sibnet.ru-
// relative path verbatim. The path is token-less; Sibnet mints the per-request
// dvNN ?e= token at fetch time, so this captured path is stable (no TTL).
var sibnetSrcRe = regexp.MustCompile(
	`(?is)player\.src\s*\(\s*\[\s*\{\s*src\s*:\s*"(/v/[^"]+\.mp4)"`)

// parseSibnetShell extracts the relative progressive-mp4 path from a Sibnet
// shell.php page. PURE: takes the raw HTML bytes, returns the video.sibnet.ru-
// relative "/v/<hash>/<id>.mp4" path. Returns an error (never panics) when no
// player.src src is present.
func parseSibnetShell(body []byte) (string, error) {
	m := sibnetSrcRe.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("animejoy: sibnet shell: no player.src mp4 found")
	}
	path := strings.TrimSpace(string(m[1]))
	if path == "" {
		return "", fmt.Errorf("animejoy: sibnet shell: empty mp4 path")
	}
	return path, nil
}

// sibnetShellURL builds the shell.php URL for a numeric videoid. The caller may
// alternatively pass a full shell URL to ResolveSibnet, which is used verbatim.
func sibnetShellURL(videoID string) string {
	return fmt.Sprintf("https://iv.sibnet.ru/shell.php?videoid=%s", videoID)
}

// ResolveSibnet resolves a Sibnet leg to a concrete progressive MP4. Thin HTTP
// wrapper: GET the shell page (iv.sibnet.ru/shell.php?videoid=<id>, which
// 301-redirects to video.sibnet.ru — followed by the default client), parse the
// player.src path, and build the absolute video.sibnet.ru URL. videoIDOrURL may
// be a bare numeric videoid ("5263892") or a full shell.php URL (e.g. the
// iv.sibnet.ru embed found in the playlist).
//
// The returned URL is token-less by design: Sibnet mints the per-request ?e=
// token at fetch time, so the path is stable and carries no TTL. The proxy MUST
// replay Referer = https://video.sibnet.ru/ when fetching it.
func (c *Client) ResolveSibnet(ctx context.Context, videoIDOrURL string) (ResolvedLeg, error) {
	target := strings.TrimSpace(videoIDOrURL)
	if target == "" {
		return ResolvedLeg{}, fmt.Errorf("animejoy: ResolveSibnet called with empty videoid/url")
	}
	if !strings.Contains(target, "://") {
		target = sibnetShellURL(target)
	}

	// AnimeJoy is the page that embeds the shell; Sibnet keys the 301 + shell on it.
	body, err := c.getBody(ctx, target, map[string]string{"Referer": DefaultBaseURL + "/"})
	if err != nil {
		return ResolvedLeg{}, fmt.Errorf("animejoy: sibnet shell request: %w", err)
	}

	path, err := parseSibnetShell(body)
	if err != nil {
		return ResolvedLeg{}, err
	}
	return ResolvedLeg{
		URL:     sibnetVideoBase + path,
		Referer: sibnetReferer,
	}, nil
}
