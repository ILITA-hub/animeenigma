package kodik

import (
	"fmt"
)

// LatestEpisodeForTranslation returns the highest available episode number on
// Kodik for a single (shikimori_id, translation_id) combination.
//
// Phase 2 v1.0 Notifications Engine (NOTIF-DET-01 / D-DET-04) wraps this from
// services/catalog/internal/service/episodes_lookup.go to answer the
// notifications detector's "what is the latest episode currently published for
// this user's combo?" question without going through the broader
// GetTranslations path (which iterates every translation and is wasted work
// for the detector's per-combo lookup).
//
// Episode count fallback ladder mirrors GetTranslations EXACTLY so the two
// surfaces never disagree on the same upstream payload:
//
//  1. r.LastEpisode (Kodik's authoritative "last airing aired" pointer)
//  2. r.EpisodesCount (per-translation count Kodik attaches when the show
//     finished airing — distinct from the search-level field of the same
//     name)
//  3. sum(season.Episodes) — derived from the per-season episode map when
//     Kodik does not surface either of the above
//  4. 1 if r.Type == "anime" (movies / single-episode releases)
//
// Returns 0 + a descriptive error when no result for the translation is
// found. Callers (the detector) treat that as a per-combo failure (skip,
// don't abort the run) — see services/notifications/internal/job/detector.go.
func (c *Client) LatestEpisodeForTranslation(shikimoriID string, translationID int) (int, error) {
	results, err := c.SearchByShikimoriID(shikimoriID)
	if err != nil {
		return 0, fmt.Errorf("kodik: search by shikimori_id %q: %w", shikimoriID, err)
	}

	best := 0
	matched := false
	for _, r := range results {
		if r.Translation == nil || r.Translation.ID != translationID {
			continue
		}
		matched = true

		// Same precedence as GetTranslations (kodik/client.go ~L522).
		count := r.LastEpisode
		if count == 0 {
			count = r.EpisodesCount
		}
		if count == 0 && r.Seasons != nil {
			for _, season := range r.Seasons {
				if season != nil && season.Episodes != nil {
					count += len(season.Episodes)
				}
			}
		}
		if count == 0 && r.Type == "anime" {
			count = 1
		}

		if count > best {
			best = count
		}
	}

	if !matched {
		return 0, fmt.Errorf("kodik: no translation %d for shikimori %s", translationID, shikimoriID)
	}
	return best, nil
}
