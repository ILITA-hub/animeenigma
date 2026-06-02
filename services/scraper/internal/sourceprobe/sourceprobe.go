// Package sourceprobe classifies a candidate media URL as a real video stream
// vs an HTML embed-page, by probing the actual response — not by matching a
// hardcoded host list. AllAnime (and others) rotate embed CDNs constantly, so
// a static blocklist is whack-a-mole; this looks at what the URL actually
// serves.
//
// Classification rules (checked in order against a small ranged GET):
//   - mpegurl Content-Type, or body starting with #EXTM3U  → Stream (HLS)
//   - response sniffs as HTML (Content-Type text/html, or an HTML-ish body)
//     → Embed (an embed player page, not a stream)
//   - video/* Content-Type                                  → Stream
//   - application/octet-stream AND body is NOT HTML          → Stream (direct mp4)
//   - anything else / probe error                            → Unknown (skip)
package sourceprobe

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Kind is the classification of a probed URL.
type Kind int

const (
	// Unknown means the probe could not confidently classify the URL (error,
	// ambiguous content type). Callers MUST treat Unknown as not-playable.
	Unknown Kind = iota
	// Stream means the URL serves a direct playable stream (HLS manifest or
	// direct video file).
	Stream
	// Embed means the URL serves an HTML embed-player page, not a stream.
	Embed
)

func (k Kind) String() string {
	switch k {
	case Stream:
		return "stream"
	case Embed:
		return "embed"
	default:
		return "unknown"
	}
}

// probeBytes bounds how much of the body we read for sniffing.
const probeBytes = 2048

// utf8BOM is the UTF-8 byte order mark some upstreams prepend.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Classify probes rawURL (with the given Referer) and reports whether it is a
// real Stream, an HTML Embed page, or Unknown. It reuses the provider's
// BaseHTTPClient (force-IPv4, retries, redirects followed) so the probe sees
// exactly what the proxy will later fetch from the same URL.
func Classify(ctx context.Context, client *domain.BaseHTTPClient, rawURL, referer string) Kind {
	if client == nil || strings.TrimSpace(rawURL) == "" {
		return Unknown
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Unknown
	}
	// Range-limit the probe; hosts that ignore it just return 200 + full body,
	// which we cap via io.LimitReader below.
	req.Header.Set("Range", "bytes=0-"+strconv.Itoa(probeBytes-1))
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		return Unknown
	}
	defer resp.Body.Close()

	// A hard error status tells us nothing about stream-vs-embed.
	if resp.StatusCode >= 400 {
		return Unknown
	}

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	body, _ := io.ReadAll(io.LimitReader(resp.Body, probeBytes))
	htmlish := sniffsHTML(body)

	switch {
	case strings.Contains(ct, "mpegurl"): // application/vnd.apple.mpegurl, audio/x-mpegurl
		return Stream
	case isM3U8Body(body):
		return Stream
	case strings.Contains(ct, "text/html") || htmlish:
		return Embed
	case strings.HasPrefix(ct, "video/"):
		return Stream
	case strings.Contains(ct, "application/octet-stream"):
		// octet-stream is used by both direct mp4 AND disguised embed/challenge
		// pages — only trust it when the body is clearly not HTML.
		if htmlish {
			return Embed
		}
		return Stream
	default:
		return Unknown
	}
}

// isM3U8Body reports whether body begins with the #EXTM3U sentinel (tolerating
// a BOM + leading whitespace). Mirrors libs/streamprobe.bytesIsM3U8.
func isM3U8Body(body []byte) bool {
	body = bytes.TrimPrefix(body, utf8BOM)
	s := strings.TrimLeft(string(body), " \t\r\n")
	return strings.HasPrefix(s, "#EXTM3U")
}

// sniffsHTML reports whether body looks like an HTML document/embed page.
func sniffsHTML(body []byte) bool {
	body = bytes.TrimPrefix(body, utf8BOM)
	s := strings.ToLower(strings.TrimLeft(string(body), " \t\r\n"))
	for _, p := range []string{"<!doctype html", "<html", "<head", "<script", "<!--"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
