package service

import (
	"context"
	"encoding/json"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// Anime-level (team-agnostic) players: aePlayer persists translation_id=” for
// these, so "latest available episode" is resolved at the anime level — no
// translation/team id. See spec 2026-06-18-aeplayer-notification-coverage.
func isAnimeLevelPlayer(player string) bool {
	switch player {
	case "english", "ae", "raw":
		return true
	default:
		return false
	}
}

// Narrow dependency seams so the resolver is unit-testable without a DB or
// real HTTP. *repo.AnimeRepository, *CatalogService and *RawResolver satisfy
// these respectively.
type animeFinder interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}
type scraperEpisodeLister interface {
	GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error)
}
type rawEpisodeLister interface {
	GetEpisodes(ctx context.Context, animeID string) (*EpisodesResponse, error)
	GetLibraryEpisodes(ctx context.Context, animeID string) (*EpisodesResponse, error)
}

// animeLevelResolver answers "latest available episode" for an empty-
// translation_id combo, dispatched by player family.
type animeLevelResolver struct {
	finder  animeFinder
	scraper scraperEpisodeLister
	raw     rawEpisodeLister
}

// Latest returns (latest episode number, translation title "", error). NotFound-
// like errors (message contains "no episode"/"not found") tell the caller to
// skip the combo silently; other errors are infra failures.
func (r *animeLevelResolver) Latest(ctx context.Context, shikimoriID, player, watchType string) (int, string, error) {
	anime, err := r.finder.GetByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, "", err
	}
	if anime == nil {
		return 0, "", apperrors.NotFound("anime not found")
	}

	switch player {
	case "english":
		return r.latestEnglish(ctx, anime.ID, watchType)
	case "ae":
		return maxRawEpisode(r.raw.GetLibraryEpisodes(ctx, anime.ID))
	case "raw":
		return maxRawEpisode(r.raw.GetEpisodes(ctx, anime.ID))
	default:
		return 0, "", apperrors.InvalidInput("player is not anime-level")
	}
}

// latestEnglish resolves the latest episode for the english (EN-scraper) family.
// Sub = max episode number in the merged list. Dub = max number among episodes
// the scraper tagged has_dub (none ⇒ NotFound, so the detector skips silently
// rather than claiming the sub count for dub).
func (r *animeLevelResolver) latestEnglish(ctx context.Context, animeID, watchType string) (int, string, error) {
	status, body, err := r.scraper.GetScraperEpisodes(ctx, animeID, "", false)
	if err != nil {
		return 0, "", apperrors.Wrap(err, apperrors.CodeUnavailable, "scraper episodes lookup failed")
	}
	if status != 200 {
		return 0, "", apperrors.NotFound("no english episodes for anime")
	}
	var resp struct {
		Data struct {
			Episodes []struct {
				Number int  `json:"number"`
				HasDub bool `json:"has_dub"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, "", apperrors.Wrap(err, apperrors.CodeInternal, "decode scraper episodes")
	}
	eps := resp.Data.Episodes
	if watchType == "dub" {
		max := 0
		for _, e := range eps {
			if e.HasDub && e.Number > max {
				max = e.Number
			}
		}
		if max == 0 {
			return 0, "", apperrors.NotFound("no english dub episodes returned")
		}
		return max, "", nil
	}
	max := maxEpisodeNum(len(eps), func(i int) int { return eps[i].Number })
	if max == 0 {
		return 0, "", apperrors.NotFound("no english episodes returned")
	}
	return max, "", nil
}

// maxRawEpisode adapts a RawResolver call result. Empty / unavailable → NotFound.
func maxRawEpisode(resp *EpisodesResponse, err error) (int, string, error) {
	if err != nil {
		return 0, "", err
	}
	if resp == nil || !resp.Available || len(resp.Episodes) == 0 {
		return 0, "", apperrors.NotFound("no episodes available for anime")
	}
	max := maxEpisodeNum(len(resp.Episodes), func(i int) int { return resp.Episodes[i].Number })
	if max == 0 {
		return 0, "", apperrors.NotFound("no numbered episodes returned")
	}
	return max, "", nil
}

// maxEpisodeNum returns the largest episode number over n items via getter.
func maxEpisodeNum(n int, get func(i int) int) int {
	max := 0
	for i := 0; i < n; i++ {
		if v := get(i); v > max {
			max = v
		}
	}
	return max
}
