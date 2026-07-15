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

// familyBuilder builds one roster row's family. ok=false omits it (best-effort).
type familyBuilder func(ctx context.Context, animeID string, row domain.ScraperProvider) (domain.SourceFamily, bool)

// buildFamilies assembles the EN family (DB-driven, required) plus one family
// per registered non-EN stream_providers row (best-effort). AUTO-608: the
// per-row dispatch is a name-keyed registry with a GENERIC default
// (rowFamily), so a brand-new DB row surfaces in /capabilities without a code
// change. kodik-iframe maps to nil = intentionally no capability (it is the
// Classic-Kodik iframe surface, not an aePlayer source). Builders that need
// the per-title catalog parsers guard on s.catalog == nil themselves. When the
// anime simply has no content on a per-title provider (kodik/hanime/
// animejoy legs), that family still surfaces tinted as no_content (see
// noContentFamily) rather than being omitted, so the hacker-mode selector is a
// full diagnostic view. Order: ae leads (first-party first), then EN, then the
// rest in the roster's weight order.
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

	// Non-EN registered rows, best-first (weight desc mirrors the FE sort).
	var rows []domain.ScraperProvider
	if err := s.db.WithContext(ctx).
		Where(`status <> ? AND "group" <> ?`, domain.StatusDisabled, "en").
		Order("preference_weight desc, name asc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load non-EN providers: %w", err)
	}

	// AnimeJoy legs share ONE discovery call (title→news_id→playlist); resolve
	// it lazily at most once per report, from whichever leg runs first.
	var (
		ajOnce  sync.Once
		ajTeams []domain.AnimejoyTeam
		ajErr   error
	)
	animejoyLeg := func(display, leg string) familyBuilder {
		return func(ctx context.Context, animeID string, row domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			ajOnce.Do(func() { ajTeams, ajErr = s.catalog.GetAnimejoyTeams(ctx, animeID) })
			if ajErr != nil {
				return domain.SourceFamily{}, false // discovery error → leg absent, not no_content
			}
			return s.animejoyLegFamily(ctx, ajTeams, row.Name, displayOf(row, display), leg)
		}
	}

	builders := map[string]familyBuilder{
		"ae": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			return s.aeFamily(ctx, animeID)
		},
		"kodik-noads": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			return s.kodikFamily(ctx, animeID)
		},
		"kodik-iframe": nil, // Classic-Kodik iframe surface — no aePlayer capability
		"hanime": func(ctx context.Context, animeID string, _ domain.ScraperProvider) (domain.SourceFamily, bool) {
			if s.catalog == nil {
				return domain.SourceFamily{}, false
			}
			return s.hanimeFamily(ctx, animeID)
		},
		"animejoy-sibnet":   animejoyLeg("Sibnet", "sibnet"),
		"animejoy-allvideo": animejoyLeg("AllVideo", "allvideo"),
		// default (absent key): generic rowFamily — see loop below.
	}

	type slot struct {
		fam domain.SourceFamily
		ok  bool
	}
	slots := make([]slot, len(rows))
	var (
		en    domain.SourceFamily
		enErr error
		wg    sync.WaitGroup
	)
	wg.Add(1)
	go func() { defer wg.Done(); en, enErr = s.BuildENFamily(ctx) }()
	for i, row := range rows {
		b, has := builders[row.Name]
		if has && b == nil {
			continue // explicit skip (kodik-iframe)
		}
		if !has {
			b = func(ctx context.Context, _ string, row domain.ScraperProvider) (domain.SourceFamily, bool) {
				return s.rowFamily(ctx, row) // generic default — ANY new row is wired
			}
		}
		wg.Add(1)
		go func(i int, row domain.ScraperProvider, b familyBuilder) {
			defer wg.Done()
			slots[i].fam, slots[i].ok = b(ctx, animeID, row)
		}(i, row, b)
	}
	wg.Wait()
	if enErr != nil {
		return nil, enErr
	}

	// Assembly: ae leads (first-party first), then EN, then the rest in the
	// roster's weight order (slots is already weight-sorted via the query).
	families := make([]domain.SourceFamily, 0, len(rows)+1)
	var rest []domain.SourceFamily
	for i, sl := range slots {
		if !sl.ok {
			continue
		}
		if rows[i].Name == "ae" {
			families = append(families, sl.fam)
			continue
		}
		rest = append(rest, sl.fam)
	}
	families = append(families, en)
	families = append(families, rest...)
	return regroupFamilies(families), nil
}

// displayOf prefers the row's operator-editable DisplayName, falling back to
// the compiled default label.
func displayOf(row domain.ScraperProvider, fallback string) string {
	if row.DisplayName != "" {
		return row.DisplayName
	}
	return fallback
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
			DisplayName: displayOf(row, displayName(row.Name)),
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
