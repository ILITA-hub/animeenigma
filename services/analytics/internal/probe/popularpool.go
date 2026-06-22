package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PopularAnime is one re-roll candidate.
type PopularAnime struct{ UUID, Name string }

// PopularPool returns popular anime the probe re-rolls into when a provider has
// no match for a slot's anime.
type PopularPool interface {
	Pool(ctx context.Context) ([]PopularAnime, error)
}

type HTTPPopularPool struct {
	base string
	hc   *http.Client
}

func NewHTTPPopularPool(catalogBaseURL string, hc *http.Client) *HTTPPopularPool {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPPopularPool{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

func (p *HTTPPopularPool) Pool(ctx context.Context) ([]PopularAnime, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/api/anime/popular?page_size=100", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("popular -> %d", resp.StatusCode)
	}
	var real struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&real); err != nil {
		return nil, err
	}
	out := make([]PopularAnime, 0, len(real.Data))
	for _, a := range real.Data {
		if a.ID != "" {
			out = append(out, PopularAnime{UUID: a.ID, Name: a.Name})
		}
	}
	return out, nil
}
