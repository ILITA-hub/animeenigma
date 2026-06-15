package capability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

const reportTTL = 10 * time.Minute

// HealthInfo is one provider's liveness as seen by /scraper/health.
type HealthInfo struct {
	Up       bool
	Playable *bool
}

// HealthSource yields per-provider health (impl wraps the scraper client).
type HealthSource interface {
	ProviderHealth(ctx context.Context) (map[string]HealthInfo, error)
}

// Service assembles capability reports. EN family in P4a; RU/Hanime in P4b.
type Service struct {
	db     *gorm.DB
	health HealthSource
	cache  cache.Cache // may be nil (skips caching)
	log    *logger.Logger
}

// NewService constructs a capability Service. cache and log may be nil.
func NewService(db *gorm.DB, health HealthSource, c cache.Cache, log *logger.Logger) *Service {
	return &Service{db: db, health: health, cache: c, log: log}
}

// Report assembles the (P4a: EN-only) capability report for an anime, cache-first.
func (s *Service) Report(ctx context.Context, animeID string) (domain.CapabilityReport, error) {
	// EN family is anime-AGNOSTIC in P4a (a global trait+health ranking, not
	// per-title), so one cache entry serves all anime; the caller's AnimeID is
	// stamped onto the result on read. P4b adds per-title signals — it MUST switch
	// to a per-anime key (e.g. "capabilities:<animeID>") to avoid stale cross-anime data.
	key := "capabilities:en:global"
	if s.cache != nil {
		var cached domain.CapabilityReport
		if err := s.cache.Get(ctx, key, &cached); err == nil {
			cached.AnimeID = animeID
			return cached, nil
		} else if !errors.Is(err, cache.ErrNotFound) && s.log != nil {
			s.log.Warnw("capability cache get failed", "error", err)
		}
	}
	fam, err := s.BuildENFamily(ctx)
	if err != nil {
		return domain.CapabilityReport{}, err
	}
	report := domain.CapabilityReport{AnimeID: animeID, Families: []domain.SourceFamily{fam}}
	if s.cache != nil {
		if err := s.cache.Set(ctx, key, report, reportTTL); err != nil && s.log != nil {
			s.log.Warnw("capability cache set failed", "error", err)
		}
	}
	return report, nil
}

// BuildENFamily reads enabled EN providers, joins live health, ranks, returns
// the "ourenglish" family. Health failure degrades to "unknown" per provider.
func (s *Service) BuildENFamily(ctx context.Context) (domain.SourceFamily, error) {
	var rows []domain.ScraperProvider
	// Use map[string]any so GORM quotes the "group" column per-dialect
	// (SQLite uses double-quotes; Postgres uses the same — both are SQL-standard).
	if err := s.db.WithContext(ctx).
		Where(map[string]any{"enabled": true, "group": "en"}).
		Order("name asc").Find(&rows).Error; err != nil {
		return domain.SourceFamily{}, fmt.Errorf("load scraper providers: %w", err)
	}

	health := map[string]HealthInfo{}
	if s.health != nil {
		if h, err := s.health.ProviderHealth(ctx); err != nil {
			if s.log != nil {
				s.log.Warnw("scraper health unavailable; providers report unknown", "error", err)
			}
		} else {
			health = h
		}
	}

	caps := make([]domain.ProviderCap, 0, len(rows))
	for _, row := range rows {
		hstatus := "unknown"
		var playable *bool
		if hi, ok := health[row.Name]; ok {
			if hi.Up {
				hstatus = "up"
			} else {
				hstatus = "down"
			}
			playable = hi.Playable
		}
		caps = append(caps, domain.ProviderCap{
			Provider:    row.Name,
			DisplayName: displayName(row.Name),
			Enabled:     row.Enabled,
			Health:      hstatus,
			Playable:    playable,
			Rank:        rankEN(row, hstatus, playable),
			Variants:    variantsFromTraits(row),
		})
	}
	sort.SliceStable(caps, func(i, j int) bool {
		if caps[i].Rank != caps[j].Rank {
			return caps[i].Rank > caps[j].Rank
		}
		return caps[i].Provider < caps[j].Provider
	})
	return domain.SourceFamily{Family: "ourenglish", Providers: caps}, nil
}
