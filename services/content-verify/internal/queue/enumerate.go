// Package queue computes the virtual content-verify queue: candidates,
// scores, unit enumeration, and the pending diff.
package queue

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// scraperProviders is the EN chain resolved via /scraper/* with prefer=.
var scraperProviders = map[string]bool{
	"gogoanime": true, "animepahe": true, "allanime-okru": true, "miruro": true, "nineanime": true,
}

func isAnimejoyLeg(p string) bool { return p == "animejoy-sibnet" || p == "animejoy-allvideo" }

// Unit is one probeable (anime × provider × internal-structure) tuple.
type Unit struct {
	AnimeID   string
	Provider  string
	Key       domain.UnitKey
	Episode   int    // sample episode (latest available on the provider)
	Episodes  int    // episodes ready on the provider for this unit; 0 = unknown
	EpisodeID string // scraper episode id; "" for kodik/animejoy
	// Synth: non-nil → persist this verdict as-is, no probe. Used where
	// provider-native metadata already answers with high confidence: ae
	// (library-ingest audio lang) and kodik (its own translation roster —
	// voice = RU dub, subtitles = original audio + burned RU subs), so the
	// throttled probe budget is spent only where the answer is unknown.
	Synth     *domain.UnitVerdict
	StateRank int // active=0 recovering=1 degraded=2 — probe order
}

func stateRank(s string) int {
	switch s {
	case "active":
		return 0
	case "recovering":
		return 1
	default:
		return 2
	}
}

// SkipUnit is one skip-probe target. TeamID resolves kodik streams; Team is
// the translation TITLE persisted in rows (FE combo carries titles).
type SkipUnit struct {
	AnimeID   string
	Provider  string
	Team      string // kodik: title; "" otherwise
	TeamID    int    // kodik: numeric id; 0 otherwise
	Episode   int
	EpisodeID string // scraper: per-episode opaque id; "" otherwise
	StateRank int
}

// Enumeration is the result of one catalog pass: verify units (content
// probing — audio lang / burned subs) AND skip units (intro/outro probing),
// built from the SAME already-fetched provider data. No extra network calls.
type Enumeration struct {
	Verify []Unit
	Skip   []SkipUnit
}

// EnumerateUnits lists every probeable unit for one anime from live catalog
// structure. Adult providers are skipped in v1 (no membership source ranks
// them; a visited hentai title still gets its non-adult providers probed).
// log may be nil (e.g. in tests); when set, every best-effort provider skip
// is recorded so a silently-crumbling provider is visible in logs rather
// than just quietly shrinking the queue.
//
// It is a thin wrapper over EnumerateAll for existing callers/tests that
// only need verify units.
func EnumerateUnits(ctx context.Context, c *catalogclient.Client, animeID string, log *logger.Logger) ([]Unit, error) {
	all, err := EnumerateAll(ctx, c, animeID, log)
	if err != nil {
		return nil, err
	}
	return all.Verify, nil
}

// EnumOpts are the optional availability hooks for one enumeration pass
// (owner directive 2026-07-20 — content-verify must not keep asking for
// providers that are down or negative-cached).
type EnumOpts struct {
	// SkipProvider gates each provider BEFORE its per-provider fetches run —
	// return true to skip it this pass (deferred by an earlier 503, or the
	// roster says its health is down). Nil = no gating.
	SkipProvider func(provider string) bool
	// OnUnavailable fires when a per-provider fetch fails with a typed 503
	// (catalogclient.UnavailableError) so the engine can defer that
	// (anime, provider) until the upstream negative-cache entry expires.
	OnUnavailable func(provider string, retryAfter time.Duration)
}

func (o EnumOpts) skip(provider string) bool {
	return o.SkipProvider != nil && o.SkipProvider(provider)
}

func (o EnumOpts) noteUnavailable(provider string, err error) {
	if o.OnUnavailable == nil {
		return
	}
	if ue, ok := catalogclient.AsUnavailable(err); ok {
		o.OnUnavailable(provider, ue.RetryAfter)
	}
}

// EnumerateAll lists every probeable verify unit AND every skip-probe unit
// for one anime, from a single catalog pass (capabilities + per-provider
// translations/episodes fetched once, reused for both).
func EnumerateAll(ctx context.Context, c *catalogclient.Client, animeID string, log *logger.Logger) (Enumeration, error) {
	return EnumerateAllWith(ctx, c, animeID, log, EnumOpts{})
}

// EnumerateAllWith is EnumerateAll with availability hooks (see EnumOpts).
func EnumerateAllWith(ctx context.Context, c *catalogclient.Client, animeID string, log *logger.Logger, opts EnumOpts) (Enumeration, error) {
	caps, err := c.Capabilities(ctx, animeID)
	if err != nil {
		return Enumeration{}, err
	}
	var units []Unit
	var skips []SkipUnit
	for _, pc := range caps {
		if pc.State == "no_content" || pc.Group == "adult" {
			continue
		}
		rank := stateRank(pc.State)
		switch {
		case pc.Group == "firstparty":
			// ae has no episode list in the capabilities pass (only a single
			// synth unit above) — v1 skips skip-unit enumeration here and
			// relies on the AniSkip fallback instead.
			lang := pc.Lang
			if lang == "" {
				lang = "ja"
			}
			key := domain.UnitKey{Track: "default"}
			units = append(units, Unit{AnimeID: animeID, Provider: pc.Provider,
				Key: key, StateRank: rank,
				Synth: &domain.UnitVerdict{Key: key, Status: domain.StatusVerified,
					Audio: &domain.AudioVerdict{Lang: lang, Confidence: 1.0, Verified: true}}})

		case pc.Provider == "kodik":
			// Kodik is never probed (owner decision 2026-07-17): its own
			// translation roster is the high-confidence truth. voice = RU dub;
			// subtitles = original audio + burned-in RU subs (RawAudio, not a
			// language guess — stays correct for non-JA originals).
			translations, err := c.KodikTranslations(ctx, animeID)
			if err != nil {
				logSkip(log, animeID, pc.Provider, "kodik translations fetch failed", err)
				continue // enumeration is best-effort per provider
			}
			for _, tr := range translations {
				ep := maxInt(tr.EpisodesCount, 1)
				if tr.Type == "subtitles" {
					key := domain.UnitKey{Team: strconv.Itoa(tr.ID), Category: "sub"}
					units = append(units, Unit{AnimeID: animeID, Provider: "kodik",
						Key: key, Episode: ep, Episodes: ep, StateRank: rank,
						Synth: &domain.UnitVerdict{Key: key, Episode: ep, Status: domain.StatusVerified,
							RawAudio: true,
							Hardsub:  &domain.HardsubVerdict{Present: true, Verified: true, Lang: "ru", Confidence: 1.0}}})
				} else {
					key := domain.UnitKey{Team: strconv.Itoa(tr.ID), Category: "dub"}
					units = append(units, Unit{AnimeID: animeID, Provider: "kodik",
						Key: key, Episode: ep, Episodes: ep, StateRank: rank,
						Synth: &domain.UnitVerdict{Key: key, Episode: ep, Status: domain.StatusVerified,
							Audio: &domain.AudioVerdict{Lang: "ru", Confidence: 1.0, Verified: true}}})
				}
				for episode := 1; episode <= ep; episode++ {
					skips = append(skips, SkipUnit{AnimeID: animeID, Provider: "kodik",
						Team: tr.Title, TeamID: tr.ID, Episode: episode, StateRank: rank})
				}
			}

		case scraperProviders[pc.Provider]:
			if opts.skip(pc.Provider) {
				logSkip(log, animeID, pc.Provider, "provider unavailable (deferred/down)", nil)
				continue
			}
			eps, err := c.ScraperEpisodes(ctx, animeID, pc.Provider)
			if err != nil {
				opts.noteUnavailable(pc.Provider, err)
				logSkip(log, animeID, pc.Provider, "scraper episodes fetch failed", err)
				continue
			}
			if len(eps) == 0 {
				logSkip(log, animeID, pc.Provider, "no episodes", nil)
				continue
			}
			latest := eps[0]
			for _, e := range eps {
				if e.Number > latest.Number {
					latest = e
				}
			}
			servers, err := c.ScraperServers(ctx, animeID, latest.ID, pc.Provider)
			if err != nil {
				opts.noteUnavailable(pc.Provider, err)
				logSkip(log, animeID, pc.Provider, "scraper servers fetch failed", err)
				continue
			}
			for _, s := range servers {
				cat := s.Type
				if cat != "dub" {
					cat = "sub"
				}
				units = append(units, Unit{AnimeID: animeID, Provider: pc.Provider,
					Key:     domain.UnitKey{Server: s.ID, Category: cat},
					Episode: latest.Number, Episodes: len(eps), EpisodeID: latest.ID, StateRank: rank})
			}
			epsSorted := make([]catalogclient.ScraperEpisode, len(eps))
			copy(epsSorted, eps)
			sort.SliceStable(epsSorted, func(i, j int) bool { return epsSorted[i].Number < epsSorted[j].Number })
			for _, e := range epsSorted {
				skips = append(skips, SkipUnit{AnimeID: animeID, Provider: pc.Provider,
					Episode: e.Number, EpisodeID: e.ID, StateRank: rank})
			}

		case isAnimejoyLeg(pc.Provider):
			eps, err := c.AnimejoyEpisodes(ctx, animeID, pc.Provider)
			if err != nil {
				logSkip(log, animeID, pc.Provider, "animejoy episodes fetch failed", err)
				continue
			}
			if len(eps) == 0 {
				logSkip(log, animeID, pc.Provider, "no episodes", nil)
				continue
			}
			latest := eps[0]
			for _, n := range eps {
				if n > latest {
					latest = n
				}
			}
			units = append(units, Unit{AnimeID: animeID, Provider: pc.Provider,
				Key: domain.UnitKey{Server: pc.Provider}, Episode: latest, Episodes: len(eps), StateRank: rank})
			epsSorted := make([]int, len(eps))
			copy(epsSorted, eps)
			sort.Ints(epsSorted)
			for _, n := range epsSorted {
				skips = append(skips, SkipUnit{AnimeID: animeID, Provider: pc.Provider,
					Episode: n, StateRank: rank})
			}
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].StateRank < units[j].StateRank })
	sort.SliceStable(skips, func(i, j int) bool { return skips[i].StateRank < skips[j].StateRank })
	return Enumeration{Verify: units, Skip: skips}, nil
}

// logSkip records a best-effort provider skip during enumeration. err is nil
// for the "no episodes" (real-empty, not a failure) case; the message alone
// carries the reason then.
func logSkip(log *logger.Logger, animeID, provider, msg string, err error) {
	if log == nil {
		return
	}
	if err != nil {
		log.Warnw("verify enumerate: provider skipped", "anime_id", animeID, "provider", provider, "error", err)
		return
	}
	log.Warnw("verify enumerate: provider skipped", "anime_id", animeID, "provider", provider, "reason", msg)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
