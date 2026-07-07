package service

import "github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"

// ProviderScore is one provider's decayed playability weights (JSON wire shape).
type ProviderScore struct {
	ThisAnimeWatch float64 `json:"this_anime_watch"`
	GlobalWatch    float64 `json:"global_watch"`
	RecentUp       float64 `json:"recent_up"`
}

// PlayabilityScores maps provider id → decayed weights.
type PlayabilityScores map[string]ProviderScore

// BuildPlayabilityScores merges the watch and probe query rows into one
// per-provider map, filtered to the known roster. Pure — no CH access.
func BuildPlayabilityScores(watch []repo.PlayabilityWatchRow, probe []repo.PlayabilityProbeRow, inRoster func(string) bool) PlayabilityScores {
	out := PlayabilityScores{}
	for _, w := range watch {
		if w.Provider == "" || !inRoster(w.Provider) {
			continue
		}
		s := out[w.Provider]
		s.ThisAnimeWatch = w.ThisAnimeWatch
		s.GlobalWatch = w.GlobalWatch
		out[w.Provider] = s
	}
	for _, p := range probe {
		if p.Provider == "" || !inRoster(p.Provider) {
			continue
		}
		s := out[p.Provider]
		s.RecentUp = p.RecentUp
		out[p.Provider] = s
	}
	return out
}
