package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrProbeNotFound signals the preferred provider has no match for the anime
// (scraper returned 404 not_found under exclusive routing). The engine treats
// this as "skip + re-roll", never a provider failure.
var ErrProbeNotFound = errors.New("probe: provider has no match for anime")

type Resolver interface {
	// Resolve produces the candidate streams for one (anime, provider). episode
	// is the specific episode to probe (0 = default/first); the scraper resolver
	// ignores it, the ae/kodik resolvers honor it.
	Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error)
}

type HTTPResolver struct {
	base string
	hc   *http.Client
}

func NewHTTPResolver(catalogBaseURL string, hc *http.Client) *HTTPResolver {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPResolver{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

type envelope struct {
	Data struct {
		Episodes []struct {
			ID     string `json:"id"`
			Number int    `json:"number"`
		} `json:"episodes"`
		Servers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"servers"`
		Stream struct {
			Headers map[string]string `json:"headers"`
			Sources []struct {
				URL  string `json:"url"`
				Exp  string `json:"exp"`
				Sig  string `json:"sig"`
				Type string `json:"type"`
			} `json:"sources"`
		} `json:"stream"`
	} `json:"data"`
}

func (r *HTTPResolver) get(ctx context.Context, path string, q url.Values) (*envelope, error) {
	u := r.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrProbeNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> %d", path, resp.StatusCode)
	}
	var e envelope
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *HTTPResolver) Resolve(ctx context.Context, animeUUID, animeName string, _ int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	// episode is unused: the scraper set always probes episode 1 (the first
	// listed episode), so the per-ref Episode hint does not apply here.
	base := "/api/anime/" + animeUUID + "/scraper"
	eps, err := r.get(ctx, base+"/episodes", url.Values{"prefer": {provider}, "exclusive": {"true"}})
	if err != nil {
		if errors.Is(err, ErrProbeNotFound) {
			return nil, StageSearch, ErrProbeNotFound
		}
		return nil, StageEpisodes, err
	}
	if len(eps.Data.Episodes) == 0 {
		return nil, StageEpisodes, fmt.Errorf("no episodes")
	}
	epID := eps.Data.Episodes[0].ID

	sv, err := r.get(ctx, base+"/servers", url.Values{"episode": {epID}, "prefer": {provider}, "exclusive": {"true"}})
	if err != nil {
		if errors.Is(err, ErrProbeNotFound) {
			// A 404 at the servers stage is NOT a "provider lacks the anime"
			// signal (only the episodes/search stage is) — surface it as a
			// generic failure so the engine treats it as CDNUnreachable, not re-roll.
			err = fmt.Errorf("%s/servers -> not found", base)
		}
		return nil, StageServers, err
	}
	if len(sv.Data.Servers) == 0 {
		return nil, StageServers, fmt.Errorf("no servers")
	}

	var out []ResolvedStream
	for _, s := range sv.Data.Servers {
		st, err := r.get(ctx, base+"/stream", url.Values{
			"episode": {epID}, "server": {s.ID}, "category": {"sub"}, "prefer": {provider}, "exclusive": {"true"},
		})
		if err != nil || len(st.Data.Stream.Sources) == 0 {
			continue
		}
		src := st.Data.Stream.Sources[0]
		out = append(out, ResolvedStream{
			Provider: provider, AnimeUUID: animeUUID, AnimeName: animeName, Slot: slot, Server: s.ID,
			MasterURL: src.URL, Exp: src.Exp, Sig: src.Sig,
			Referer: st.Data.Stream.Headers["Referer"], Stage: StageStream,
		})
	}
	if len(out) == 0 {
		return nil, StageStream, fmt.Errorf("no resolvable stream")
	}
	return out, StageStream, nil
}
