package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AeAnimeSet selects the ae probe targets: the newest distinct-anime library
// uploads, via catalog GET /internal/probe/ae-targets?limit=N. Each target
// carries its own episode (the uploaded one), so ae probes real freshly-ingested
// content rather than episode 1.
type AeAnimeSet struct {
	base  string
	limit int
	hc    *http.Client
}

func NewAeAnimeSet(catalogBaseURL string, limit int, hc *http.Client) *AeAnimeSet {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	if limit <= 0 {
		limit = 3
	}
	return &AeAnimeSet{base: strings.TrimRight(catalogBaseURL, "/"), limit: limit, hc: hc}
}

// Resolve returns the ae target refs. Degrades gracefully: any failure (catalog
// down, empty library) yields nil refs + nil error — the engine then emits a
// synthetic empty_response so ae reads "down", never vanishes.
func (a *AeAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	u := fmt.Sprintf("%s/internal/probe/ae-targets?limit=%d", a.base, a.limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	// httputil.OK envelope: {"success":true,"data":{"targets":[...]}}.
	var env struct {
		Data struct {
			Targets []struct {
				UUID    string `json:"uuid"`
				Name    string `json:"name"`
				Episode int    `json:"episode"`
			} `json:"targets"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&env) != nil {
		return nil, nil
	}
	refs := make([]AnimeRef, 0, len(env.Data.Targets))
	for _, t := range env.Data.Targets {
		if t.UUID == "" {
			continue
		}
		refs = append(refs, AnimeRef{UUID: t.UUID, Name: t.Name, Episode: t.Episode, Slot: SlotLibraryLatest})
	}
	return refs, nil
}

// AeResolver resolves the self-hosted ae stream for one (anime, episode) via
// catalog GET /api/anime/{uuid}/ae/stream?episode=N, which returns a
// provenance-signed MinIO master URL the streaming proxy presigns + serves.
type AeResolver struct {
	base string
	hc   *http.Client
}

func NewAeResolver(catalogBaseURL string, hc *http.Client) *AeResolver {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &AeResolver{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

func (r *AeResolver) Resolve(ctx context.Context, animeUUID, animeName string, episode int, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	ep := episode
	if ep <= 0 {
		ep = 1
	}
	u := fmt.Sprintf("%s/api/anime/%s/ae/stream?episode=%d", r.base, animeUUID, ep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, StageStream, err
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, StageStream, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, StageStream, fmt.Errorf("ae/stream -> %d", resp.StatusCode)
	}
	// Defensive decode: handle both the httputil.OK envelope ({data:{...}}) and
	// a flat body, mirroring the anchor-name fetch in animeset.go.
	type aeStream struct {
		URL string `json:"url"`
		Exp string `json:"exp"`
		Sig string `json:"sig"`
	}
	var env struct {
		Data *aeStream `json:"data"`
		aeStream
	}
	if json.NewDecoder(resp.Body).Decode(&env) != nil {
		return nil, StageStream, fmt.Errorf("ae/stream decode failed")
	}
	s := env.Data
	if s == nil {
		s = &env.aeStream
	}
	if s.URL == "" {
		return nil, StageStream, fmt.Errorf("ae/stream empty url")
	}
	return []ResolvedStream{{
		Provider: provider, AnimeUUID: animeUUID, AnimeName: animeName, Slot: slot,
		Server: "library", MasterURL: s.URL, Exp: s.Exp, Sig: s.Sig, Stage: StageStream,
	}}, StageStream, nil
}

// ensure interface satisfaction at compile time.
var (
	_ AnimeSetResolver = (*AeAnimeSet)(nil)
	_ Resolver         = (*AeResolver)(nil)
)
