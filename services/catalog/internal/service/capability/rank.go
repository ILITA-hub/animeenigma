// Package capability assembles the ranked per-provider capability report.
package capability

import (
	"strings"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// rankEN scores an EN provider for ordering (higher = better). Pure.
// Degraded providers are forced below every enabled provider (even a down one)
// so the player always ranks them last (AUTO-484); they stay in the list so
// hacker mode can still offer them (preference_weight only breaks ties among
// degraded providers).
func rankEN(row domain.ScraperProvider, health string, playable *bool) float64 {
	if row.IsDegraded() {
		return -1000 + float64(row.PreferenceWeight)
	}
	score := float64(row.PreferenceWeight)
	if health == "down" {
		score -= 100
	}
	if playable != nil {
		if *playable {
			score += 25
		} else {
			score -= 25
		}
	}
	score += qualityCeilingScore(row.QualityCeiling)
	switch row.SubDelivery {
	case "soft":
		score += 10
	case "hard":
		score -= 5
	}
	return score
}

func qualityCeilingScore(q string) float64 {
	switch strings.ToLower(strings.TrimSpace(q)) {
	case "2160p":
		return 20
	case "1080p":
		return 15
	case "720p":
		return 8
	default:
		return 0
	}
}

// variantsFromTraits derives the watchable variants a provider claims (no live
// per-title confirmation — Source="trait"). Dub is audio → sub_delivery "none".
func variantsFromTraits(row domain.ScraperProvider) []domain.Variant {
	var out []domain.Variant
	q := []string{}
	if row.QualityCeiling != "" {
		q = []string{row.QualityCeiling}
	}
	mk := func(cat, delivery string) domain.Variant {
		return domain.Variant{
			Category: cat, SubDelivery: delivery, Qualities: q,
			QualitySource: "trait", Source: "trait",
		}
	}
	if row.SupportsSub {
		out = append(out, mk("sub", row.SubDelivery))
	}
	if row.SupportsDub {
		out = append(out, mk("dub", "none"))
	}
	if row.SupportsRaw {
		out = append(out, mk("raw", row.SubDelivery))
	}
	return out
}

// displayName title-cases a provider id for UI.
func displayName(provider string) string {
	known := map[string]string{
		"allanime": "AllAnime", "okru": "OK.ru", "gogoanime": "GogoAnime", "animepahe": "AnimePahe",
		"animefever": "AnimeFever", "miruro": "Miruro", "nineanime": "9anime",
		"animekai": "AnimeKai", "18anime": "18anime",
	}
	if d, ok := known[provider]; ok {
		return d
	}
	if provider == "" {
		return provider
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}
