package animelib

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"
)

// LatestEpisodeForTeam returns the highest episode number on AnimeLib that has
// at least one PlayerData matching the given (team_id, watch_type) combo.
//
// Phase 2 v1.0 Notifications Engine (NOTIF-DET-01 / D-DET-04). The detector
// uses translation_id (= AnimeLib team_id) as its stable per-combo key —
// because (a) AnimeLib's translation surface IS the team, and (b) the
// watch_history rows the detector consumes already store translation_id as
// the team id (see services/player/internal/handler/watch.go's AniLib
// player-data persistence path).
//
// watchType mapping (AnimeLib's TranslationType.ID is a small enum):
//   - "dub" → TranslationType.ID == 2 ("Озвучка" / voice)
//   - "sub" → TranslationType.ID == 1 ("Субтитры" / subtitles)
//
// Algorithm:
//  1. GetEpisodes(animelibID) — cheap O(1) HTTP, returns episode IDs +
//     numeric labels.
//  2. Sort newest-first by parsed Number (string -> int / fallback float).
//  3. Fan out GetEpisodeStreams (one HTTP per episode) with errgroup cap 5.
//     Newest-first ordering means we can prune cheaply: if the highest
//     matching episode so far is N, we no longer need to query episodes
//     with Number < N. (We DO still query equal-or-higher because the
//     pre-sort may not be monotonic when AnimeLib emits decimals like
//     "10.5".)
//  4. Per-episode: scan PlayerData for the first match → record episode
//     number → continue to other goroutines (no early-abort because
//     errgroup is best-effort, not strict).
//
// The second return value is the team's display name (Team.Name, e.g.
// "AniRise") taken from the matching PlayerData — same upstream payload,
// no extra HTTP. Empty when no match (alongside the error).
//
// Returns 0 + a not-found-ish error when no episode has a matching
// (team, watch_type) PlayerData. Detector treats that as a per-combo
// failure-and-skip — see services/notifications/internal/job/detector.go.
//
// PERFORMANCE NOTE (R-02-06 in 02-PLAN.md): worst case is 24 GetEpisodeStreams
// HTTP calls per combo. Capped at 5 in-flight via errgroup. The catalog
// service layer wraps this in a 5-min Redis cache so cron-storm scenarios
// stay bounded. v1.0.x can swap this for an upstream batch endpoint without
// changing the public signature.
func (c *Client) LatestEpisodeForTeam(ctx context.Context, animelibID int, teamID int, watchType string) (int, string, error) {
	wantType, err := translationTypeForWatchType(watchType)
	if err != nil {
		return 0, "", err
	}

	episodes, err := c.GetEpisodes(animelibID)
	if err != nil {
		return 0, "", fmt.Errorf("animelib: get_episodes anime_id=%d: %w", animelibID, err)
	}
	if len(episodes) == 0 {
		return 0, "", fmt.Errorf("animelib: no episodes returned for anime_id=%d", animelibID)
	}

	// Sort newest-first by parsed number so the prune comment above holds.
	type indexedEp struct {
		ep  Episode
		num int
	}
	sortable := make([]indexedEp, 0, len(episodes))
	for _, ep := range episodes {
		sortable = append(sortable, indexedEp{ep: ep, num: parseEpisodeNumber(ep.Number)})
	}
	sort.Slice(sortable, func(i, j int) bool {
		return sortable[i].num > sortable[j].num
	})

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	var (
		highest  int
		teamName string
		mu       sync.Mutex
	)

	for _, ep := range sortable {
		ep := ep
		g.Go(func() error {
			// Best-effort: if context already cancelled (e.g. parent deadline),
			// stop early.
			if gctx.Err() != nil {
				return nil
			}
			detail, err := c.GetEpisodeStreams(ep.ep.ID)
			if err != nil {
				// Per-episode failures are not fatal — the rest of the fan-out
				// can still find a matching higher-numbered episode.
				return nil
			}
			if detail == nil {
				return nil
			}
			for _, pd := range detail.Players {
				if pd.Team.ID == teamID && pd.TranslationType.ID == wantType {
					mu.Lock()
					if ep.num > highest {
						highest = ep.num
					}
					if teamName == "" {
						teamName = pd.Team.Name
					}
					mu.Unlock()
					return nil
				}
			}
			return nil
		})
	}

	_ = g.Wait()

	if highest == 0 {
		return 0, "", fmt.Errorf("animelib: no episode for team=%d watch_type=%q on anime_id=%d",
			teamID, watchType, animelibID)
	}
	return highest, teamName, nil
}

// maxAnimeLibEpisode returns the highest integer episode number in the list.
// Non-numeric/fractional Number strings are skipped (AnimeLib uses string
// numbers; specials like "3.5" don't advance the "latest episode" count).
func maxAnimeLibEpisode(eps []Episode) int {
	best := 0
	for _, e := range eps {
		n, err := strconv.Atoi(e.Number)
		if err != nil {
			continue
		}
		if n > best {
			best = n
		}
	}
	return best
}

// LatestEpisodeAnyTeam returns the latest episode number across the full
// (team-agnostic) episode list for an AnimeLib anime id. Used by the
// notifications detector for aePlayer animelib combos (no specific team).
// Returns 0 + nil when the list is empty (caller maps to NotFound/skip).
func (c *Client) LatestEpisodeAnyTeam(ctx context.Context, animelibID int) (int, error) {
	eps, err := c.GetEpisodes(animelibID)
	if err != nil {
		return 0, fmt.Errorf("animelib: episodes for id %d: %w", animelibID, err)
	}
	return maxAnimeLibEpisode(eps), nil
}

// translationTypeForWatchType maps the notifications' watch_type string into
// the AnimeLib TranslationType ID enum.
func translationTypeForWatchType(watchType string) (int, error) {
	switch watchType {
	case "dub", "voice":
		return 2, nil
	case "sub", "subtitles":
		return 1, nil
	default:
		return 0, fmt.Errorf("animelib: unsupported watch_type %q (expected sub|dub)", watchType)
	}
}

// parseEpisodeNumber turns AnimeLib's stringly-typed Episode.Number into a
// comparable int. Decimal episodes (10.5) get truncated to 10 — acceptable
// because we sort newest-first and recompute true max from successful
// matches.
func parseEpisodeNumber(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}
