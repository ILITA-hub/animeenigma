package capability

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// --- tunable weights (single source; adjust here) ---------------------------
// Applied to max-normalized terms in [0,1]; the owner ranks recent this-title
// watch success highest, then recent probe-up + health, then global watch.
const (
	wThisAnime = 3.0
	wGlobal    = 1.0
	wRecentUp  = 1.5
	wHealth    = 1.0

	hUp   = 1.0 // health term multipliers (pre-wHealth)
	hRec  = 0.3
	hDown = -1.0

	// promoteFloorValue: decayed this-anime watch weight (~one successful watch
	// within the last week) that flips a has-content degraded provider to
	// active/selectable for that title. The single hard boundary in the model.
	promoteFloorValue = 0.5
)

func promoteFloor() float64 { return promoteFloorValue }

// providerScore is one provider's decayed weights (catalog-side mirror of the
// analytics wire shape).
type providerScore struct {
	ThisAnimeWatch float64 `json:"this_anime_watch"`
	GlobalWatch    float64 `json:"global_watch"`
	RecentUp       float64 `json:"recent_up"`
}

// providerAliases maps a source-specific provider name to its canonical
// capability id. probe_runs uses "kodik-noads" where the cap id is "kodik".
var providerAliases = map[string]string{"kodik-noads": "kodik"}

// applyProviderAliases merges alias keys into their canonical cap id, summing
// each term (an alias only carries the term its source produces).
func applyProviderAliases(raw map[string]providerScore) map[string]providerScore {
	if len(raw) == 0 {
		return raw
	}
	out := make(map[string]providerScore, len(raw))
	for k, v := range raw {
		canon := k
		if a, ok := providerAliases[k]; ok {
			canon = a
		}
		cur := out[canon]
		cur.ThisAnimeWatch += v.ThisAnimeWatch
		cur.GlobalWatch += v.GlobalWatch
		cur.RecentUp += v.RecentUp
		out[canon] = cur
	}
	return out
}

// blendData carries the fetched, aliased scores plus the normalization maxima
// for one report build. A nil *blendData means "analytics unavailable" and
// yields a health-only index (graceful degradation).
type blendData struct {
	scores                        map[string]providerScore
	maxThis, maxGlobal, maxRecent float64
}

func newBlendData(scores map[string]providerScore) *blendData {
	b := &blendData{scores: scores}
	for _, s := range scores {
		if s.ThisAnimeWatch > b.maxThis {
			b.maxThis = s.ThisAnimeWatch
		}
		if s.GlobalWatch > b.maxGlobal {
			b.maxGlobal = s.GlobalWatch
		}
		if s.RecentUp > b.maxRecent {
			b.maxRecent = s.RecentUp
		}
	}
	return b
}

func norm(x, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return x / max
}

func healthTerm(h domain.ProviderHealth) float64 {
	switch h {
	case domain.HealthUp:
		return hUp
	case domain.HealthRecovering:
		return hRec
	case domain.HealthDown:
		return hDown
	default:
		return 0
	}
}

// indexFor blends one provider's normalized watch/probe terms with its health.
// Nil-safe: a nil blend (or an absent provider) yields the health-only term.
func (b *blendData) indexFor(provider string, h domain.ProviderHealth) float64 {
	ht := wHealth * healthTerm(h)
	if b == nil {
		return ht
	}
	s := b.scores[provider]
	return wThisAnime*norm(s.ThisAnimeWatch, b.maxThis) +
		wGlobal*norm(s.GlobalWatch, b.maxGlobal) +
		wRecentUp*norm(s.RecentUp, b.maxRecent) +
		ht
}

// thisAnimeWatch returns the raw decayed this-title watch weight (drives
// promotion). Nil-safe → 0.
func (b *blendData) thisAnimeWatch(provider string) float64 {
	if b == nil {
		return 0
	}
	return b.scores[provider].ThisAnimeWatch
}

// --- ctx plumbing: blendData is per-request, carried through applyFeedFields --
type blendCtxKey struct{}

func withBlend(ctx context.Context, b *blendData) context.Context {
	return context.WithValue(ctx, blendCtxKey{}, b)
}

// blendFrom returns the request's blendData, or nil if none was seeded.
func blendFrom(ctx context.Context) *blendData {
	b, _ := ctx.Value(blendCtxKey{}).(*blendData)
	return b
}
