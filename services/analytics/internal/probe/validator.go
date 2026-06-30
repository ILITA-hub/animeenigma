package probe

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

const (
	validatorBudget = 10 * time.Second
	maxSegmentBytes = 8 << 20
)

type Validator interface {
	Validate(ctx context.Context, rs ResolvedStream) Verdict
}

type HTTPValidator struct {
	streaming string
	hc        *http.Client
	prober    VideoProber
}

func NewHTTPValidator(streamingBaseURL string, hc *http.Client, vp VideoProber) *HTTPValidator {
	if hc == nil {
		hc = &http.Client{Timeout: validatorBudget}
	}
	return &HTTPValidator{streaming: strings.TrimRight(streamingBaseURL, "/"), hc: hc, prober: vp}
}

// hlsProxyPath is the streaming service's NATIVE proxy route. The probe calls
// streaming directly (http://streaming:8082), bypassing the gateway — so it must
// use /api/v1/hls-proxy, NOT the public /api/streaming/hls-proxy path that the
// gateway rewrites for browsers. Hitting the public path against the service
// directly 404s (route not found), which the validator would misread as a dead
// stream. See services/streaming/internal/transport/router.go.
const hlsProxyPath = "/api/v1/hls-proxy"

// proxyURL builds the streaming hls-proxy URL for a raw upstream URL. A manifest
// child the proxy already rewrote is a root-absolute PUBLIC path
// (/api/streaming/hls-proxy?url=...&exp=...&sig=...) — the probe maps that public
// prefix to the streaming-native route, preserving the proxy-minted query, so
// the variant/segment hops also reach the service directly.
func (v *HTTPValidator) proxyURL(rs ResolvedStream, raw string) string {
	for _, pfx := range []string{"/api/streaming/hls-proxy", hlsProxyPath} {
		if strings.HasPrefix(raw, pfx) {
			return v.streaming + hlsProxyPath + strings.TrimPrefix(raw, pfx)
		}
	}
	q := url.Values{"url": {raw}}
	if rs.Exp != "" {
		q.Set("exp", rs.Exp)
	}
	if rs.Sig != "" {
		q.Set("sig", rs.Sig)
	}
	if rs.Referer != "" {
		q.Set("referer", rs.Referer)
	}
	return v.streaming + hlsProxyPath + "?" + q.Encode()
}

func (v *HTTPValidator) fetch(ctx context.Context, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := v.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSegmentBytes))
	return body, resp.StatusCode, nil
}

func firstNonComment(manifest []byte) string {
	for _, ln := range strings.Split(string(manifest), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		return ln
	}
	return ""
}

// hasAES128 reports whether an HLS manifest declares AES-128 segment encryption.
// AES-128 segments are valid content but ffprobe cannot decode the ciphertext
// without the key, so callers should skip the video-decode gate for such streams.
func hasAES128(manifest []byte) bool {
	return strings.Contains(string(manifest), "#EXT-X-KEY:METHOD=AES-128")
}

// looksLikeManifest reports whether the fetched bytes are an HLS playlist (the
// #EXTM3U tag is, per spec, the first line). Used to distinguish an HLS master
// from a progressive media file (e.g. an AnimeJoy mp4) and, mid-walk, a sub-
// manifest from a media segment.
func looksLikeManifest(b []byte) bool {
	return strings.Contains(string(b[:min(len(b), 64)]), "#EXTM3U")
}

func (v *HTTPValidator) Validate(ctx context.Context, rs ResolvedStream) Verdict {
	ctx, cancel := context.WithTimeout(ctx, validatorBudget)
	defer cancel()
	verdict := Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, AnimeName: rs.AnimeName, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback}

	master, status, err := v.fetch(ctx, v.proxyURL(rs, rs.MasterURL))
	if err != nil {
		verdict.Reason = streamprobe.ReasonCDNUnreachable
		return verdict
	}
	if status == http.StatusForbidden {
		verdict.Reason = streamprobe.ReasonStatus403
		return verdict
	}
	if status != http.StatusOK || len(master) == 0 {
		verdict.Reason = streamprobe.ReasonEmptyResponse
		return verdict
	}

	// Progressive media (e.g. an AnimeJoy mp4): the upstream URL is the playable
	// file itself, not an HLS manifest — there is no variant/segment chain to
	// walk. ffprobe the fetched head directly: reachability (200 + bytes) plus a
	// decodable video stream = playable.
	if !looksLikeManifest(master) {
		if v.prober.Probe(ctx, master) != nil {
			verdict.Reason = streamprobe.ReasonDecodeFailed
			return verdict
		}
		verdict.Reason = streamprobe.ReasonPlayable
		return verdict
	}

	// Track AES-128 encryption across the manifest chain. When segments are
	// AES-128 encrypted the ciphertext is opaque to ffprobe; we trust
	// reachability (HTTP 200 + non-empty bytes) instead of video decode.
	encrypted := hasAES128(master)

	// master -> first variant (if present) -> first segment.
	cur := master
	for hops := 0; hops < 2; hops++ {
		line := firstNonComment(cur)
		if line == "" {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		body, st, err := v.fetch(ctx, v.proxyURL(rs, line))
		if err != nil {
			verdict.Reason = streamprobe.ReasonCDNUnreachable
			return verdict
		}
		if st == http.StatusForbidden {
			verdict.Reason = streamprobe.ReasonStatus403
			return verdict
		}
		if st != http.StatusOK || len(body) == 0 {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		if !looksLikeManifest(body) {
			// reached a media segment
			if encrypted {
				// Segment is AES-128 ciphertext — ffprobe would see random bytes
				// and report no video stream. Reachability is sufficient here.
				verdict.Reason = streamprobe.ReasonPlayable
				return verdict
			}
			if perr := v.prober.Probe(ctx, body); perr != nil {
				verdict.Reason = streamprobe.ReasonDecodeFailed
				return verdict
			}
			verdict.Reason = streamprobe.ReasonPlayable
			return verdict
		}
		// Propagate encryption flag from sub-manifests (variant playlists can
		// carry their own #EXT-X-KEY even when the master does not).
		if hasAES128(body) {
			encrypted = true
		}
		cur = body
	}
	verdict.Reason = streamprobe.ReasonInvalidVideo
	return verdict
}
