package recs

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// attrLoader supplies per-anime attribute sets (namespaced "genre:{id}" /
// "studio:{id}") for similarity. Production: GormAttrLoader; tests inject a fake.
type attrLoader interface {
	LoadAttrSets(ctx context.Context, animeIDs []string) (map[string]map[string]struct{}, error)
}

// Diversifier is the S12 post-rank greedy MMR re-rank (spec 2026-06-11
// Phase 4). It never adds or removes items — it only reorders, trading a
// little Final score for variety so the row isn't 20 near-identical cards.
type Diversifier struct {
	loader attrLoader
}

// s12GenreSetCap: at most this many picked items may share an IDENTICAL
// genre-ID set — the closest stand-in for franchise dedup until a real
// franchise column exists (sequels share exact genre sets). Relaxed when
// every remaining item violates it (reordering must never drop items).
const s12GenreSetCap = 3

// NewDiversifier constructs a Diversifier backed by the given attrLoader.
func NewDiversifier(loader attrLoader) *Diversifier {
	return &Diversifier{loader: loader}
}

// Rerank greedily re-orders ranked by  score = Final − λ·maxSim(candidate,
// picked). seedAnimeID, when non-empty (the S6 pin), counts as already
// picked for similarity but is NOT part of the output. λ=0 degenerates to
// the input order (input is already Final-desc sorted). The full input
// always comes back, only reordered.
func (d *Diversifier) Rerank(ctx context.Context, ranked []Recommendation, seedAnimeID string, lambda float64) ([]Recommendation, error) {
	if len(ranked) <= 1 {
		return ranked, nil
	}

	ids := make([]string, 0, len(ranked)+1)
	for _, r := range ranked {
		ids = append(ids, string(r.AnimeID))
	}
	if seedAnimeID != "" {
		ids = append(ids, seedAnimeID)
	}
	attrs, err := d.loader.LoadAttrSets(ctx, ids)
	if err != nil {
		return nil, err
	}

	pickedSets := make([]map[string]struct{}, 0, len(ranked)+1)
	if seedAnimeID != "" {
		if s, ok := attrs[seedAnimeID]; ok {
			pickedSets = append(pickedSets, s)
		}
	}
	genreSigCount := make(map[string]int, len(ranked))

	remaining := make([]Recommendation, len(ranked))
	copy(remaining, ranked)
	out := make([]Recommendation, 0, len(ranked))

	for len(remaining) > 0 {
		bestIdx := -1
		bestScore := 0.0

		// Normal pass: respect the genre-set hard cap.
		for i, cand := range remaining {
			if genreSigCount[genreSignature(attrs[string(cand.AnimeID)])] >= s12GenreSetCap {
				continue
			}
			score := cand.Final - lambda*maxSim(attrs[string(cand.AnimeID)], pickedSets)
			// First eligible candidate (bestIdx == -1) always taken; subsequent
			// candidates need a strictly higher score so that ties preserve the
			// input (Final-desc) order via the first-wins property of stable sort.
			if bestIdx == -1 || score > bestScore {
				bestIdx = i
				bestScore = score
			}
		}

		if bestIdx == -1 {
			// Every remaining item violates the genre cap — relax it so we
			// never drop items. Pick by plain MMR score among all remaining.
			for i, cand := range remaining {
				score := cand.Final - lambda*maxSim(attrs[string(cand.AnimeID)], pickedSets)
				if bestIdx == -1 || score > bestScore {
					bestIdx = i
					bestScore = score
				}
			}
		}

		pick := remaining[bestIdx]
		out = append(out, pick)
		pickedSets = append(pickedSets, attrs[string(pick.AnimeID)])
		genreSigCount[genreSignature(attrs[string(pick.AnimeID)])]++
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}
	return out, nil
}

// maxSim is the max Jaccard similarity between set a and any picked set.
func maxSim(a map[string]struct{}, picked []map[string]struct{}) float64 {
	var best float64
	for _, p := range picked {
		if j := jaccardSets(a, p); j > best {
			best = j
		}
	}
	return best
}

func jaccardSets(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersect := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersect++
		}
	}
	union := len(a) + len(b) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

// genreSignature returns a canonical string for the genre subset of an
// attribute set ("genre:1|genre:5"), used for the identical-genre-set cap.
// Items with NO genres (empty signature) share one bucket — acceptable: a
// genreless item is metadata-poor and capping those is harmless.
func genreSignature(attrs map[string]struct{}) string {
	genres := make([]string, 0, len(attrs))
	for k := range attrs {
		if strings.HasPrefix(k, "genre:") {
			genres = append(genres, k)
		}
	}
	sort.Strings(genres)
	return strings.Join(genres, "|")
}

// GormAttrLoader loads namespaced genre+studio sets from the shared DB.
type GormAttrLoader struct{ db *gorm.DB }

// NewGormAttrLoader constructs a GormAttrLoader backed by db.
func NewGormAttrLoader(db *gorm.DB) *GormAttrLoader { return &GormAttrLoader{db: db} }

// LoadAttrSets fetches genre and studio attributes for the given anime IDs,
// returning a map of animeID → set of "genre:{id}" / "studio:{id}" strings.
func (l *GormAttrLoader) LoadAttrSets(ctx context.Context, animeIDs []string) (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{}, len(animeIDs))
	if len(animeIDs) == 0 {
		return out, nil
	}
	add := func(animeID, key string) {
		set, ok := out[animeID]
		if !ok {
			set = make(map[string]struct{})
			out[animeID] = set
		}
		set[key] = struct{}{}
	}
	var rows []struct {
		AnimeID string
		AttrID  string
	}
	if err := l.db.WithContext(ctx).Table("anime_genres").
		Select("anime_id, genre_id AS attr_id").Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		add(r.AnimeID, "genre:"+r.AttrID)
	}
	rows = nil
	if err := l.db.WithContext(ctx).Table("anime_studios").
		Select("anime_id, studio_id AS attr_id").Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		add(r.AnimeID, "studio:"+r.AttrID)
	}
	return out, nil
}
