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

// LocalizeHLS downloads master → first variant → media playlist through the
// proxy, absolutizes every URI against the gateway (segments AND EXT-X-KEY
// URIs — AES key fetches must also ride the proxy), writes a local .m3u8,
// and returns its path plus the summed EXTINF duration.
func LocalizeHLS(ctx context.Context, hc *http.Client, gatewayBase, masterURL, dir string) (string, float64, error) {
	body, err := fetch(ctx, hc, masterURL)
	if err != nil {
		return "", 0, err
	}
	media := body
	mediaURL := masterURL
	if !strings.Contains(body, "#EXTINF") { // master playlist → hop to first variant
		variant := firstNonComment(body)
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
