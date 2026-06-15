package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// collapseByFranchise keeps one entry per non-empty franchise (the first one
// seen — callers pass earliest-aired first), and keeps every standalone
// (empty-franchise) anime individually.
func collapseByFranchise(animes []*domain.Anime) []*domain.Anime {
	seen := make(map[string]bool)
	out := make([]*domain.Anime, 0, len(animes))
	for _, a := range animes {
		if a.Franchise == "" {
			out = append(out, a)
			continue
		}
		if seen[a.Franchise] {
			continue
		}
		seen[a.Franchise] = true
		out = append(out, a)
	}
	return out
}

// PoolTaxon is an id+name pair for a genre/studio/tag (anidle compares by id,
// displays by name).
type PoolTaxon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GuessPoolEntry is one guessable anime with the 8 comparison attributes.
type GuessPoolEntry struct {
	ID        string      `json:"id"`
	NameRU    string      `json:"name_ru"`
	NameEN    string      `json:"name_en"`
	NameJP    string      `json:"name_jp"`
	PosterURL string      `json:"poster_url"`
	Year      int         `json:"year"`
	Episodes  int         `json:"episodes"`
	Score     float64     `json:"score"`
	Status    string      `json:"status"`
	Rating    string      `json:"rating"`
	Genres    []PoolTaxon `json:"genres"`
	Studios   []PoolTaxon `json:"studios"`
	Tags      []PoolTaxon `json:"tags"`
}

type guessPoolRepo interface {
	ListGuessPoolCandidates(ctx context.Context, minScore float64) ([]*domain.Anime, error)
	SetFranchise(ctx context.Context, id, franchise string) error
}

type franchiseFetcher interface {
	GetAnimeFranchise(ctx context.Context, shikimoriID string) (string, error)
}

// GuessPoolService builds the franchise-collapsed score>minScore pool.
type GuessPoolService struct {
	repo     guessPoolRepo
	fetcher  franchiseFetcher
	log      *logger.Logger
	minScore float64
}

func NewGuessPoolService(repo guessPoolRepo, fetcher franchiseFetcher, log *logger.Logger) *GuessPoolService {
	return &GuessPoolService{repo: repo, fetcher: fetcher, log: log, minScore: 8.0}
}

// BuildPool lists candidates, backfills any missing franchise via REST (persisted
// once), collapses by franchise, and maps to the DTO.
func (s *GuessPoolService) BuildPool(ctx context.Context) ([]GuessPoolEntry, error) {
	candidates, err := s.repo.ListGuessPoolCandidates(ctx, s.minScore)
	if err != nil {
		return nil, err
	}

	for _, a := range candidates {
		if a.FranchiseChecked || a.Franchise != "" || a.ShikimoriID == "" {
			continue
		}
		fr, ferr := s.fetcher.GetAnimeFranchise(ctx, a.ShikimoriID)
		if ferr != nil {
			if s.log != nil {
				s.log.Debugw("franchise backfill failed; will retry next build",
					"anime_id", a.ID, "shikimori_id", a.ShikimoriID, "error", ferr)
			}
			continue // not marked checked -> retried on the next build
		}
		// Persist the franchise (possibly empty for a standalone) AND mark the
		// row checked, so a standalone anime is not re-fetched on every build.
		a.Franchise = fr
		if serr := s.repo.SetFranchise(ctx, a.ID, fr); serr != nil {
			if s.log != nil {
				s.log.Warnw("persist franchise failed", "anime_id", a.ID, "error", serr)
			}
		}
	}

	collapsed := collapseByFranchise(candidates)

	out := make([]GuessPoolEntry, 0, len(collapsed))
	for _, a := range collapsed {
		out = append(out, toPoolEntry(a))
	}
	return out, nil
}

func toPoolEntry(a *domain.Anime) GuessPoolEntry {
	e := GuessPoolEntry{
		ID:        a.ID,
		NameRU:    a.NameRU,
		NameEN:    a.NameEN,
		NameJP:    a.NameJP,
		PosterURL: a.PosterURL,
		Year:      a.Year,
		Episodes:  a.EpisodesCount,
		Score:     a.Score,
		Status:    string(a.Status),
		Rating:    a.Rating,
		Genres:    make([]PoolTaxon, 0),
		Studios:   make([]PoolTaxon, 0),
		Tags:      make([]PoolTaxon, 0),
	}
	for _, g := range a.Genres {
		name := g.NameRU
		if name == "" {
			name = g.Name
		}
		e.Genres = append(e.Genres, PoolTaxon{ID: g.ID, Name: name})
	}
	for _, st := range a.Studios {
		e.Studios = append(e.Studios, PoolTaxon{ID: st.ID, Name: st.Name})
	}
	for _, tg := range a.Tags {
		e.Tags = append(e.Tags, PoolTaxon{ID: tg.ID, Name: tg.Name})
	}
	return e
}
