package capability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
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

// Service assembles capability reports. EN family (trait+health) plus the
// per-title RU (kodik/animelib) and Hanime families when a CatalogSource is wired.
type Service struct {
	db      *gorm.DB
	health  HealthSource
	catalog CatalogSource // may be nil (skips RU/Hanime families)
	cache   cache.Cache   // may be nil (skips caching)
	log     *logger.Logger
}

// NewService constructs a capability Service. catalog, cache and log may be nil.
func NewService(db *gorm.DB, health HealthSource, catalog CatalogSource, c cache.Cache, log *logger.Logger) *Service {
	return &Service{db: db, health: health, catalog: catalog, cache: c, log: log}
}

// Report assembles the full per-anime capability report, cache-first. The report
// now carries per-title signals (RU translation teams, Hanime quality), so the
// cache key is per-anime — NOT the P4a global key.
func (s *Service) Report(ctx context.Context, animeID string) (domain.CapabilityReport, error) {
	key := "capabilities:" + animeID
	if s.cache != nil {
		var cached domain.CapabilityReport
		if err := s.cache.Get(ctx, key, &cached); err == nil {
			return cached, nil
		} else if !errors.Is(err, cache.ErrNotFound) && s.log != nil {
			s.log.Warnw("capability cache get failed", "error", err)
		}
	}
	families, err := s.buildFamilies(ctx, animeID)
	if err != nil {
		return domain.CapabilityReport{}, err
	}
	report := domain.CapabilityReport{AnimeID: animeID, Families: families}
	if s.cache != nil {
		if err := s.cache.Set(ctx, key, report, reportTTL); err != nil && s.log != nil {
			s.log.Warnw("capability cache set failed", "error", err)
		}
	}
	return report, nil
}

// buildFamilies assembles every family concurrently. The EN family is required
// (its error fails the report); the RU/Hanime families are best-effort (omitted
// on error or when the anime isn't on that provider). Order is stable:
// ourenglish, kodik, animelib, hanime.
func (s *Service) buildFamilies(ctx context.Context, animeID string) ([]domain.SourceFamily, error) {
	type slot struct {
		fam domain.SourceFamily
		ok  bool
	}
	var (
		en                      domain.SourceFamily
		enErr                   error
		kodik, animelib, hanime slot
		wg                      sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		en, enErr = s.BuildENFamily(ctx)
	}()

	if s.catalog != nil {
		wg.Add(3)
		go func() { defer wg.Done(); kodik.fam, kodik.ok = s.kodikFamily(ctx, animeID) }()
		go func() { defer wg.Done(); animelib.fam, animelib.ok = s.animelibFamily(ctx, animeID) }()
		go func() { defer wg.Done(); hanime.fam, hanime.ok = s.hanimeFamily(ctx, animeID) }()
	}
	wg.Wait()

	if enErr != nil {
		return nil, enErr
	}
	families := []domain.SourceFamily{en}
	for _, sl := range []slot{kodik, animelib, hanime} {
		if sl.ok {
			families = append(families, sl.fam)
		}
	}
	return families, nil
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
