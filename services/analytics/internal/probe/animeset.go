package probe

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"sort"
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
	// Score is the Shikimori/MAL score (0–10). Used to order probe titles
	// most-popular-first so high-visibility anime surface failures first.
	Score float64
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

// animeDetail bundles the fields returned by the catalog anime-detail endpoint
// that the probe cares about: display name + popularity score.
type animeDetail struct {
	animeName
	Score float64 `json:"score"`
}

// fetchDetail best-effort resolves a single anime's display title and score via
// the catalog detail endpoint. Returns zero values on any failure.
func (a *HTTPAnimeSet) fetchDetail(ctx context.Context, uuid string) (name string, score float64) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/api/anime/"+uuid, nil)
	if err != nil {
		return "", 0
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return "", 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0
	}
	var env struct {
		Data *animeDetail `json:"data"`
		animeDetail
	}
	if json.NewDecoder(resp.Body).Decode(&env) != nil {
		return "", 0
	}
	if env.Data != nil {
		return env.Data.pick(), env.Data.Score
	}
	return env.animeDetail.pick(), env.animeDetail.Score
}

// fetchName is kept for call sites that only need the display title.
func (a *HTTPAnimeSet) fetchName(ctx context.Context, uuid string) string {
	name, _ := a.fetchDetail(ctx, uuid)
	return name
}

func (a *HTTPAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	anchorName, anchorScore := a.fetchDetail(ctx, a.anchor)
	refs := []AnimeRef{{UUID: a.anchor, Name: anchorName, Slot: SlotAnchor, Score: anchorScore}}

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
	scores := map[string]float64{}
	for _, c := range env.Cards {
		var h struct {
			Anime struct {
				ID    string  `json:"id"`
				Score float64 `json:"score"`
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
			scores[id] = h.Anime.Score
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
	refs = append(refs, AnimeRef{UUID: slotFeaturedID, Name: names[slotFeaturedID], Slot: SlotFeatured, Score: scores[slotFeaturedID]})

	// SlotSpotlightRandom: random pick from pool.
	pick := pool[a.rng.Intn(len(pool))]
	refs = append(refs, AnimeRef{UUID: pick, Name: names[pick], Slot: SlotSpotlightRandom, Score: scores[pick]})

	// SlotRandom: another random pick from pool (may coincide — acceptable).
	other := pool[a.rng.Intn(len(pool))]
	refs = append(refs, AnimeRef{UUID: other, Name: names[other], Slot: SlotRandom, Score: scores[other]})

	return sortByPopularity(refs), nil
}

// sortByPopularity returns a copy of refs sorted by Score descending (stable,
// so refs with identical scores retain their original relative order).
func sortByPopularity(refs []AnimeRef) []AnimeRef {
	out := make([]AnimeRef, len(refs))
	copy(out, refs)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
