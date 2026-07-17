package prober

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const publicProxyPath = "/api/streaming/hls-proxy"

// ProxiedURL builds the gateway hls-proxy URL for a resolved stream. A URL
// already pointing at the proxy (masked /m/ or hls-proxy paths) is only
// re-based onto the gateway.
func ProxiedURL(gatewayBase string, rawURL, exp, sig, referer string) string {
	base := strings.TrimRight(gatewayBase, "/")
	if strings.HasPrefix(rawURL, "/api/streaming/") || strings.HasPrefix(rawURL, "/api/v1/") {
		p := strings.Replace(rawURL, "/api/v1/", "/api/streaming/", 1)
		return base + p
	}
	q := url.Values{"url": {rawURL}}
	if exp != "" {
		q.Set("exp", exp)
	}
	if sig != "" {
		q.Set("sig", sig)
	}
	if referer != "" {
		q.Set("referer", referer)
	}
	return base + publicProxyPath + "?" + q.Encode()
}

var extinfRe = regexp.MustCompile(`#EXTINF:([0-9.]+)`)
var uriAttrRe = regexp.MustCompile(`URI="([^"]+)"`)
var streamInfBandwidthRe = regexp.MustCompile(`BANDWIDTH=(\d+)`)

// LocalizeHLS downloads master → first variant → media playlist through the
// proxy, absolutizes every URI against the gateway (segments AND EXT-X-KEY
// URIs — AES key fetches must also ride the proxy), writes a local .m3u8,
// and returns its path plus the summed EXTINF duration.
//
// Thin wrapper over LocalizeHLSVariant(lowest=false) — keeps this call
// signature and today's first-listed-variant behavior untouched for every
// existing caller/test.
func LocalizeHLS(ctx context.Context, hc *http.Client, gatewayBase, masterURL, dir string) (string, float64, error) {
	return LocalizeHLSVariant(ctx, hc, gatewayBase, masterURL, dir, false)
}

// LocalizeHLSVariant is LocalizeHLS with explicit control over which master
// variant is selected. lowest=false keeps the original first-listed
// behavior; lowest=true parses BANDWIDTH=(\d+) from #EXT-X-STREAM-INF lines
// and picks the URI following the smallest one (falling back to the first
// variant when no BANDWIDTH attributes are present at all).
func LocalizeHLSVariant(ctx context.Context, hc *http.Client, gatewayBase, masterURL, dir string, lowest bool) (string, float64, error) {
	body, err := fetch(ctx, hc, masterURL)
	if err != nil {
		return "", 0, err
	}
	media := body
	mediaURL := masterURL
	if !strings.Contains(body, "#EXTINF") { // master playlist → hop to a variant
		variant := selectVariant(body, lowest)
		if variant == "" {
			return "", 0, fmt.Errorf("empty master playlist")
		}
		mediaURL = absolutize(gatewayBase, masterURL, variant)
		media, err = fetch(ctx, hc, mediaURL)
		if err != nil {
			return "", 0, err
		}
	}
	var out []string
	var duration float64
	for _, line := range strings.Split(media, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			if m := extinfRe.FindStringSubmatch(trimmed); m != nil {
				if d, err := strconv.ParseFloat(m[1], 64); err == nil {
					duration += d
				}
			}
			out = append(out, uriAttrRe.ReplaceAllStringFunc(trimmed, func(attr string) string {
				u := uriAttrRe.FindStringSubmatch(attr)[1]
				return `URI="` + absolutize(gatewayBase, mediaURL, u) + `"`
			}))
		case trimmed == "":
			out = append(out, trimmed)
		default:
			out = append(out, absolutize(gatewayBase, mediaURL, trimmed))
		}
	}
	local := filepath.Join(dir, "media_local.m3u8")
	if err := os.WriteFile(local, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		return "", 0, err
	}
	return local, duration, nil
}

// absolutize: root-absolute proxy paths (/api/streaming/... or /api/v1/...)
// go onto the gateway; scheme-full URLs pass through; anything else resolves
// relative to the playlist URL.
func absolutize(gatewayBase, baseURL, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if strings.HasPrefix(ref, "/") {
		return strings.TrimRight(gatewayBase, "/") + strings.Replace(ref, "/api/v1/", "/api/streaming/", 1)
	}
	b, err := url.Parse(baseURL)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func firstNonComment(manifest string) string {
	for _, line := range strings.Split(manifest, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") {
			return t
		}
	}
	return ""
}

// selectVariant picks a variant URI from a master playlist body. lowest=false
// keeps the original behavior: the first non-comment line. lowest=true scans
// #EXT-X-STREAM-INF lines for a BANDWIDTH attribute and returns the URI
// following the one with the smallest value; if no BANDWIDTH attributes are
// found at all, it falls back to firstNonComment (same as lowest=false).
func selectVariant(manifest string, lowest bool) string {
	if !lowest {
		return firstNonComment(manifest)
	}
	lines := strings.Split(manifest, "\n")
	bestBandwidth := -1
	bestURI := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF") {
			continue
		}
		m := streamInfBandwidthRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		bandwidth, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		uri := ""
		for j := i + 1; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			uri = t
			break
		}
		if uri == "" {
			continue
		}
		if bestBandwidth == -1 || bandwidth < bestBandwidth {
			bestBandwidth = bandwidth
			bestURI = uri
		}
	}
	if bestURI == "" {
		return firstNonComment(manifest)
	}
	return bestURI
}

func fetch(ctx context.Context, hc *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s -> %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}
