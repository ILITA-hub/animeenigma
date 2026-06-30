package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AnimejoyResolver resolves the AnimeJoy RU-sub progressive MP4 for one anime via
// catalog's per-leg animejoy routes. The provider name IS the leg
// ("animejoy-sibnet" / "animejoy-allvideo"), so ONE resolver instance serves both
// targets:
//
//	GET /api/anime/{uuid}/{provider}/episodes         → {episodes:[]int, teams:[…]}
//	GET /api/anime/{uuid}/{provider}/stream?episode=N → {url, referer, exp, sig, type:"mp4"}
//
// The returned URL is a signed progressive MP4 (video.sibnet.ru / incvideo1.online),
// NOT in the proxy allowlist, so the validator replays exp/sig + Referer through
// the hls-proxy. AnimeJoy serves no HLS manifest, so the validator's
// progressive-media path ffprobes the mp4 head directly (no segment chain).
type AnimejoyResolver struct {
	base string
	hc   *http.Client
}

// NewAnimejoyResolver builds the resolver. A generous client timeout matters: a
// cold resolve does live AnimeJoy discovery (search → news_id → playlist) plus a
// leg fetch (sibnet shell / fsst embed) on our egress before the catalog responds
// (the nil-client fallback is 30s; main wires 45s).
func NewAnimejoyResolver(catalogBaseURL string, hc *http.Client) *AnimejoyResolver {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &AnimejoyResolver{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

func (r *AnimejoyResolver) get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s -> %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (r *AnimejoyResolver) Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	base := "/api/anime/" + animeUUID + "/" + provider

	// 1) episodes → pick the requested one, else the first listed.
	var eEnv struct {
		Data struct {
			Episodes []int `json:"episodes"`
		} `json:"data"`
	}
	if err := r.get(ctx, r.base+base+"/episodes", &eEnv); err != nil {
		return nil, StageEpisodes, err
	}
	if len(eEnv.Data.Episodes) == 0 {
		// AnimeJoy has no match for this title — re-roll, never a provider failure.
		return nil, StageSearch, ErrProbeNotFound
	}
	ep := episode
	if ep <= 0 {
		ep = eEnv.Data.Episodes[0]
	}

	// 2) stream for the chosen episode.
	var sEnv struct {
		Data struct {
			URL     string `json:"url"`
			Referer string `json:"referer"`
			Exp     string `json:"exp"`
			Sig     string `json:"sig"`
		} `json:"data"`
	}
	if err := r.get(ctx, fmt.Sprintf("%s%s/stream?episode=%d", r.base, base, ep), &sEnv); err != nil {
		return nil, StageStream, err
	}
	if sEnv.Data.URL == "" {
		return nil, StageStream, fmt.Errorf("animejoy: empty stream url")
	}

	return []ResolvedStream{{
		Provider: provider, AnimeUUID: animeUUID, AnimeName: animeName, Slot: slot,
		Server: provider, MasterURL: sEnv.Data.URL,
		Exp: sEnv.Data.Exp, Sig: sEnv.Data.Sig, Referer: sEnv.Data.Referer, Stage: StageStream,
	}}, StageStream, nil
}

var _ Resolver = (*AnimejoyResolver)(nil)
