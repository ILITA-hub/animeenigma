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
	db          *gorm.DB
	health      HealthSource
	catalog     CatalogSource // may be nil (skips RU/Hanime families)
	cache       cache.Cache   // may be nil (skips caching)
	log         *logger.Logger
	library     LibrarySource     // may be nil (then `ae` is always no_content)
	playability PlayabilitySource // may be nil (then blend is health-only, no promotion)
}

// NewService constructs a capability Service. catalog, cache, log, library and
// playability may be nil.
func NewService(db *gorm.DB, health HealthSource, catalog CatalogSource, c cache.Cache, log *logger.Logger, library LibrarySource, playability PlayabilitySource) *Service {
	return &Service{db: db, health: health, catalog: catalog, cache: c, log: log, library: library, playability: playability}
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
// (its error fails the report); the first-party (ae/adult) and RU/Hanime
// families are best-effort — omitted on a fetch error, an absent/disabled DB
// row, or (AnimeJoy only) a discovery error. When the anime simply has no
// content on a per-title provider (kodik/animelib/hanime/animejoy legs), that
// family still surfaces tinted as no_content (see noContentFamily) rather than
// being omitted, so the hacker-mode selector is a full diagnostic view. Order is
// stable: ae, ourenglish, adult, kodik, animelib, hanime, animejoy-sibnet,
// animejoy-allvideo — first-party leads. The ae/adult families are DB-row-driven
// (no CatalogSource needed) so they run regardless of whether catalog is wired.
func (s *Service) buildFamilies(ctx context.Context, animeID string) ([]domain.SourceFamily, error) {
	// Fetch playability scores ONCE per report (best-effort) and seed them into
	// ctx before the fan-out below, so every family goroutine that closes over
	// ctx sees the same seeded blend (newBlendData(nil) is the graceful nil-safe
	// health-only path when playability is nil or the fetch failed).
	var raw map[string]providerScore
	if s.playability != nil {
		raw = s.playability.Scores(ctx, animeID)
	}
	ctx = withBlend(ctx, newBlendData(raw))

	type slot struct {
		fam domain.SourceFamily
		ok  bool
	}
	var (
		en                      domain.SourceFamily
		enErr                   error
		ae, adult               slot
		kodik, animelib, hanime slot
		ajSibnet, ajAllVideo    slot
		wg                      sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		en, enErr = s.BuildENFamily(ctx)
	}()
	go func() { defer wg.Done(); ae.fam, ae.ok = s.aeFamily(ctx, animeID) }()
	go func() { defer wg.Done(); adult.fam, adult.ok = s.dbRowFamily(ctx, "18anime", "18anime", "adult") }()

	if s.catalog != nil {
		wg.Add(4)
		go func() { defer wg.Done(); kodik.fam, kodik.ok = s.kodikFamily(ctx, animeID) }()
		go func() { defer wg.Done(); animelib.fam, animelib.ok = s.animelibFamily(ctx, animeID) }()
		go func() { defer wg.Done(); hanime.fam, hanime.ok = s.hanimeFamily(ctx, animeID) }()
		// AnimeJoy: resolve discovery ONCE, then build BOTH leg families from the
		// shared teams (no second network call). A discovery ERROR keeps both legs
		// absent (a transient failure must never surface a misleading no_content);
		// only a successful-but-empty discovery lets animejoyLegFamily surface
		// no_content.
		go func() {
			defer wg.Done()
			teams, ajErr := s.catalog.GetAnimejoyTeams(ctx, animeID)
			if ajErr != nil {
				return // discovery error → both legs absent (not a misleading no_content)
			}
			ajSibnet.fam, ajSibnet.ok = s.animejoyLegFamily(ctx, teams, "animejoy-sibnet", "Sibnet", "sibnet")
			ajAllVideo.fam, ajAllVideo.ok = s.animejoyLegFamily(ctx, teams, "animejoy-allvideo", "AllVideo", "allvideo")
		}()
	}
	wg.Wait()

	if enErr != nil {
		return nil, enErr
	}
	// ae leads (first-party first), then EN, then the rest in stable order.
	families := make([]domain.SourceFamily, 0, 9)
	if ae.ok {
		families = append(families, ae.fam)
	}
	families = append(families, en)
	for _, sl := range []slot{adult, kodik, animelib, hanime, ajSibnet, ajAllVideo} {
		if sl.ok {
			families = append(families, sl.fam)
		}
	}
	return regroupFamilies(families), nil
}

// BuildENFamily reads registered EN providers (enabled + degraded; disabled are
// excluded), joins live health, ranks, returns the "ourenglish" family. Degraded
// providers are included so the player can offer them in hacker mode, but rankEN
// pushes them last. Health failure degrades to "unknown" per provider.
func (s *Service) BuildENFamily(ctx context.Context) (domain.SourceFamily, error) {
	var rows []domain.ScraperProvider
	// status <> 'disabled' keeps enabled + degraded; "group" is double-quoted so
	// the reserved word parses on both SQLite (tests) and Postgres (prod).
	if err := s.db.WithContext(ctx).
		Where(`status <> ? AND "group" = ?`, domain.StatusDisabled, "en").
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
		cap := domain.ProviderCap{
			Provider:    row.Name,
			DisplayName: displayName(row.Name),
			Rank:        rankEN(row, hstatus, playable),
			Variants:    variantsFromTraits(row),
		}
		// EN rows are loaded with status<>'disabled', so IsRegistered() is always
		// true here ⇒ ok is always true. hasContent=true for EN in Phase 1.
		applyFeedFields(ctx, &cap, row, true)
		caps = append(caps, cap)
	}
	sort.SliceStable(caps, func(i, j int) bool {
		if caps[i].Rank != caps[j].Rank {
			return caps[i].Rank > caps[j].Rank
		}
		return caps[i].Provider < caps[j].Provider
	})
	return domain.SourceFamily{Family: "ourenglish", Providers: caps}, nil
}

// familyLabel maps an internally-assembled family string to the collapsed wire
// taxonomy: "18+" (adult sources), "aeProvider" (first-party standalone), or
// "others" (every language provider — EN chain, kodik, animelib, animejoy legs).
func familyLabel(internal string) string {
	switch internal {
	case "hanime", "adult":
		return "18+"
	case "ae":
		return "aeProvider"
	default:
		return "others"
	}
}

// regroupFamilies collapses the internally-assembled per-source families into the
// three wire families {aeProvider, others, 18+}. Providers are bucketed by
// familyLabel, preserving input order within a bucket; buckets are emitted in
// first-seen order (deterministic). PURE — the FE re-sorts by state/order, so the
// merged intra-family order is not display-authoritative.
func regroupFamilies(in []domain.SourceFamily) []domain.SourceFamily {
	order := []string{}
	byLabel := map[string][]domain.ProviderCap{}
	for _, fam := range in {
		label := familyLabel(fam.Family)
		if _, seen := byLabel[label]; !seen {
			order = append(order, label)
		}
		byLabel[label] = append(byLabel[label], fam.Providers...)
	}
	out := make([]domain.SourceFamily, 0, len(order))
	for _, label := range order {
		out = append(out, domain.SourceFamily{Family: label, Providers: byLabel[label]})
	}
	return out
}
