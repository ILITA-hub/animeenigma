package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// KodikNoadsResolver resolves the ad-free (scraped) Kodik HLS for one anime via
// catalog's kodik routes:
//
//	GET /api/anime/{uuid}/kodik/translations        → pick pinned, else first
//	GET /api/anime/{uuid}/kodik/stream?episode=1&translation=ID → {stream_url, referer, exp, sig}
//
// The catalog signs the CDN URL (solodcdn.com) with the proxy's provenance
// HMAC (streamsign, Track S), so the validator replays exp/sig + Referer
// through the hls-proxy like animejoy — no static allowlist entry required.
type KodikNoadsResolver struct {
	base string
	hc   *http.Client
}

func NewKodikNoadsResolver(catalogBaseURL string, hc *http.Client) *KodikNoadsResolver {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &KodikNoadsResolver{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

func (r *KodikNoadsResolver) get(ctx context.Context, url string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("%s -> %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}

func (r *KodikNoadsResolver) Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	base := "/api/anime/" + animeUUID + "/kodik"

	// 1) translations → pick pinned, else first.
	var tEnv struct {
		Data []struct {
			ID     int  `json:"id"`
			Pinned bool `json:"pinned"`
		} `json:"data"`
	}
	if _, err := r.get(ctx, r.base+base+"/translations", &tEnv); err != nil {
		return nil, StageServers, err
	}
	if len(tEnv.Data) == 0 {
		return nil, StageServers, fmt.Errorf("kodik: no translations")
	}
	chosen := tEnv.Data[0].ID
	for _, tr := range tEnv.Data {
		if tr.Pinned {
			chosen = tr.ID
			break
		}
	}

	// 2) stream for episode (default 1).
	ep := episode
	if ep <= 0 {
		ep = 1
	}
	var sEnv struct {
		Data struct {
			StreamURL string `json:"stream_url"`
			Referer   string `json:"referer"`
			Exp       string `json:"exp"`
			Sig       string `json:"sig"`
		} `json:"data"`
	}
	streamURL := fmt.Sprintf("%s%s/stream?episode=%d&translation=%d", r.base, base, ep, chosen)
	if _, err := r.get(ctx, streamURL, &sEnv); err != nil {
		return nil, StageStream, err
	}
	if sEnv.Data.StreamURL == "" {
		return nil, StageStream, fmt.Errorf("kodik: empty stream_url")
	}

	return []ResolvedStream{{
		Provider: provider, AnimeUUID: animeUUID, AnimeName: animeName, Slot: slot,
		Server: "kodik-" + strconv.Itoa(chosen), MasterURL: sEnv.Data.StreamURL,
		Exp: sEnv.Data.Exp, Sig: sEnv.Data.Sig, Referer: sEnv.Data.Referer, Stage: StageStream,
	}}, StageStream, nil
}

var _ Resolver = (*KodikNoadsResolver)(nil)
