package probe

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type AnimeRef struct {
	UUID string
	Slot AnimeSlot
}

type AnimeSetResolver interface {
	Resolve(ctx context.Context) ([]AnimeRef, error)
}

type HTTPAnimeSet struct {
	base   string
	anchor string
	hc     *http.Client
	rng    *rand.Rand
}

func NewHTTPAnimeSet(catalogBaseURL, anchorUUID string, hc *http.Client, rng *rand.Rand) *HTTPAnimeSet {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPAnimeSet{base: strings.TrimRight(catalogBaseURL, "/"), anchor: anchorUUID, hc: hc, rng: rng}
}

func (a *HTTPAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	refs := []AnimeRef{{UUID: a.anchor, Slot: SlotAnchor}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/api/home/spotlight", nil)
	if err != nil {
		return refs, nil
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return refs, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return refs, nil
	}
	var env struct {
		Data struct {
			Cards []struct {
				AnimeID string `json:"anime_id"`
			} `json:"cards"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return refs, nil
	}
	var ids []string
	for _, c := range env.Data.Cards {
		if c.AnimeID != "" && c.AnimeID != a.anchor {
			ids = append(ids, c.AnimeID)
		}
	}
	if len(ids) == 0 {
		return refs, nil
	}
	refs = append(refs, AnimeRef{UUID: ids[0], Slot: SlotFeatured})
	if len(ids) > 1 {
		pick := ids[1+a.rng.Intn(len(ids)-1)]
		refs = append(refs, AnimeRef{UUID: pick, Slot: SlotSpotlightRandom})
		other := ids[a.rng.Intn(len(ids))]
		refs = append(refs, AnimeRef{UUID: other, Slot: SlotRandom})
	}
	return refs, nil
}
