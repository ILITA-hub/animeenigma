package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/parser/animethemes"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/repo"
)

type SyncService struct {
	themeRepo *repo.ThemeRepository
	client    *animethemes.Client
	status    *domain.SyncStatus
	log       *logger.Logger
}

func NewSyncService(themeRepo *repo.ThemeRepository, client *animethemes.Client, log *logger.Logger) *SyncService {
	return &SyncService{
		themeRepo: themeRepo,
		client:    client,
		status:    domain.NewSyncStatus(),
		log:       log,
	}
}

// CurrentSeason returns the current year and season name.
func CurrentSeason() (int, string) {
	now := time.Now()
	year := now.Year()
	month := now.Month()

	switch {
	case month >= 1 && month <= 3:
		return year, "winter"
	case month >= 4 && month <= 6:
		return year, "spring"
	case month >= 7 && month <= 9:
		return year, "summer"
	default:
		return year, "fall"
	}
}

// StartSync triggers a sync in a background goroutine.
// If year/season are zero/empty, defaults to the current season.
func (s *SyncService) StartSync(year int, season string) error {
	status := s.status.Get()
	if status.Running {
		return fmt.Errorf("sync already in progress")
	}

	if year == 0 || season == "" {
		year, season = CurrentSeason()
	}
	s.status.Start(year, season)

	go func() {
		if err := s.syncSeason(year, season); err != nil {
			s.log.Errorw("sync failed", "year", year, "season", season, "error", err)
			s.status.SetError(err.Error())
			return
		}
		s.status.Done()
	}()

	return nil
}

func (s *SyncService) syncSeason(year int, season string) error {
	s.log.Infow("starting theme sync", "year", year, "season", season)

	themes, err := s.client.FetchSeason(year, season)
	if err != nil {
		return fmt.Errorf("fetch season: %w", err)
	}

	s.status.SetTotal(len(themes))
	s.log.Infow("fetched themes from API", "count", len(themes))

	ctx := context.Background()
	for i := range themes {
		if err := s.themeRepo.Upsert(ctx, &themes[i]); err != nil {
			s.log.Errorw("failed to upsert theme",
				"external_id", themes[i].ExternalID,
				"anime", themes[i].AnimeName,
				"error", err,
			)
			continue
		}
		s.status.Increment()
	}

	s.log.Infow("theme sync completed", "year", year, "season", season, "total", len(themes))
	return nil
}

// GetStatus returns the current sync status.
func (s *SyncService) GetStatus() domain.SyncStatus {
	return s.status.Get()
}
