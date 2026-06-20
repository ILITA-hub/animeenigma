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

// proxyURL builds the streaming hls-proxy URL for a raw upstream URL. When the
// upstream is already a proxied (rewritten) path returned in a manifest, it is
// absolute-from-root and used as-is against the streaming base.
func (v *HTTPValidator) proxyURL(rs ResolvedStream, raw string) string {
	if strings.HasPrefix(raw, "/api/streaming/") {
		return v.streaming + raw
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
	return v.streaming + "/api/streaming/hls-proxy?" + q.Encode()
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

func (v *HTTPValidator) Validate(ctx context.Context, rs ResolvedStream) Verdict {
	ctx, cancel := context.WithTimeout(ctx, validatorBudget)
	defer cancel()
	verdict := Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback}

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

	// master -> first variant (if present) -> first segment.
	cur := master
	for hops := 0; hops < 2; hops++ {
		line := firstNonComment(cur)
		if line == "" {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		body, st, err := v.fetch(ctx, v.proxyURL(rs, line))
		if err != nil || st == http.StatusForbidden {
			verdict.Reason = streamprobe.ReasonStatus403
			return verdict
		}
		if st != http.StatusOK || len(body) == 0 {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		if !strings.Contains(string(body[:min(len(body), 64)]), "#EXTM3U") {
			// reached a media segment
			if perr := v.prober.Probe(ctx, body); perr != nil {
				verdict.Reason = streamprobe.ReasonDecodeFailed
				return verdict
			}
			verdict.Reason = streamprobe.ReasonPlayable
			return verdict
		}
		cur = body
	}
	verdict.Reason = streamprobe.ReasonInvalidVideo
	return verdict
}
