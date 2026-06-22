package kodik

import (
	"context"
	"fmt"
)

// resultEpisodeCount returns the episode count a single search result implies,
// using Kodik's field precedence: last_episode → episodes_count → summed
// season episodes → 1 for a bare anime entry.
func resultEpisodeCount(r SearchResult) int {
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
	return count
}

// maxAnyTeamEpisode returns the highest episode count across ALL translations
// (team-agnostic) — the anime-level "latest episode" for notification detection.
func maxAnyTeamEpisode(results []SearchResult) int {
	best := 0
	for _, r := range results {
		if c := resultEpisodeCount(r); c > best {
			best = c
		}
	}
	return best
}

// LatestEpisodeAnyTranslation returns the latest episode available across ANY
// translation for the anime (used by the notifications detector for aePlayer
// kodik combos, which carry no specific translation_id). Returns 0 + nil when
// the anime has no kodik results (caller maps that to NotFound/skip).
func (c *Client) LatestEpisodeAnyTranslation(ctx context.Context, shikimoriID string) (int, error) {
	results, err := c.SearchByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, fmt.Errorf("kodik: search by shikimori_id %q: %w", shikimoriID, err)
	}
	return maxAnyTeamEpisode(results), nil
}

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
// The second return value is the translation's display title
// (Translation.Title, e.g. "AniLibria.TV") — it rides along in the same
// upstream search response, so surfacing it costs no extra HTTP. Empty when
// Kodik omits it.
//
// Returns 0 + a descriptive error when no result for the translation is
// found. Callers (the detector) treat that as a per-combo failure (skip,
// don't abort the run) — see services/notifications/internal/job/detector.go.
func (c *Client) LatestEpisodeForTranslation(ctx context.Context, shikimoriID string, translationID int) (int, string, error) {
	results, err := c.SearchByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, "", fmt.Errorf("kodik: search by shikimori_id %q: %w", shikimoriID, err)
	}

	best := 0
	title := ""
	matched := false
	for _, r := range results {
		if r.Translation == nil || r.Translation.ID != translationID {
			continue
		}
		matched = true
		if title == "" {
			title = r.Translation.Title
		}

		// Same precedence as GetTranslations (kodik/client.go ~L522).
		count := resultEpisodeCount(r)

		if count > best {
			best = count
		}
	}

	if !matched {
		return 0, "", fmt.Errorf("kodik: no translation %d for shikimori %s", translationID, shikimoriID)
	}
	return best, title, nil
}
