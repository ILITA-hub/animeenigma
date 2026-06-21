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
	Name string
	// Episode is the specific episode to probe (0 = default/first). The ae
	// target set sets this to the uploaded episode; scraper/kodik sets leave it 0.
	Episode int
	Slot    AnimeSlot
}

// animeName carries the title fields a catalog anime object exposes. The probe
// prefers Russian, then English, then the romaji/original primary name.
type animeName struct {
	Name   string `json:"name"`
	NameEN string `json:"name_en"`
	NameRU string `json:"name_ru"`
}

func (n animeName) pick() string {
	switch {
	case n.NameRU != "":
		return n.NameRU
	case n.NameEN != "":
		return n.NameEN
	default:
		return n.Name
	}
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

// fetchName best-effort resolves a single anime's display title via the catalog
// detail endpoint. Returns "" on any failure (the name column is decorative).
func (a *HTTPAnimeSet) fetchName(ctx context.Context, uuid string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/api/anime/"+uuid, nil)
	if err != nil {
		return ""
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var env struct {
		Data *animeName `json:"data"`
		animeName
	}
	if json.NewDecoder(resp.Body).Decode(&env) != nil {
		return ""
	}
	if env.Data != nil {
		return env.Data.pick()
	}
	return env.animeName.pick()
}

func (a *HTTPAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	refs := []AnimeRef{{UUID: a.anchor, Name: a.fetchName(ctx, a.anchor), Slot: SlotAnchor}}

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

	// Real spotlight envelope: {"cards":[{"type":"...","data":{...}}],"generated_at":"..."}
	// No "data" wrapper at the top level.
	var env struct {
		Cards []struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		} `json:"cards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return refs, nil
	}

	// Extract anime UUIDs from anime-bearing cards.
	// Anime-bearing cards have data.anime.id; non-anime cards (latest_news,
	// platform_stats, telegram_news) either lack "anime" or have an empty id.
	var featuredID string
	var pool []string
	names := map[string]string{}
	for _, c := range env.Cards {
		var h struct {
			Anime struct {
				ID string `json:"id"`
				animeName
			} `json:"anime"`
		}
		if json.Unmarshal(c.Data, &h) != nil {
			continue
		}
		id := h.Anime.ID
		if id == "" || id == a.anchor {
			continue
		}
		if c.Type == "featured" && featuredID == "" {
			featuredID = id
		}
		if _, seen := names[id]; !seen {
			names[id] = h.Anime.animeName.pick()
		}
		pool = append(pool, id)
	}

	if len(pool) == 0 {
		return refs, nil
	}

	// SlotFeatured: prefer the "featured"-type card; fall back to pool[0].
	slotFeaturedID := featuredID
	if slotFeaturedID == "" {
		slotFeaturedID = pool[0]
	}
	refs = append(refs, AnimeRef{UUID: slotFeaturedID, Name: names[slotFeaturedID], Slot: SlotFeatured})

	// SlotSpotlightRandom: random pick from pool.
	pick := pool[a.rng.Intn(len(pool))]
	refs = append(refs, AnimeRef{UUID: pick, Name: names[pick], Slot: SlotSpotlightRandom})

	// SlotRandom: another random pick from pool (may coincide — acceptable).
	other := pool[a.rng.Intn(len(pool))]
	refs = append(refs, AnimeRef{UUID: other, Name: names[other], Slot: SlotRandom})

	return refs, nil
}
