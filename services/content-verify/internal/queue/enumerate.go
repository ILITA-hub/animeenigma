// Package queue computes the virtual content-verify queue: candidates,
// scores, unit enumeration, and the pending diff.
package queue

import (
	"context"
	"sort"
	"strconv"

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

// EnumerateUnits lists every probeable unit for one anime from live catalog
// structure. Adult providers are skipped in v1 (no membership source ranks
// them; a visited hentai title still gets its non-adult providers probed).
// log may be nil (e.g. in tests); when set, every best-effort provider skip
// is recorded so a silently-crumbling provider is visible in logs rather
// than just quietly shrinking the queue.
func EnumerateUnits(ctx context.Context, c *catalogclient.Client, animeID string, log *logger.Logger) ([]Unit, error) {
	caps, err := c.Capabilities(ctx, animeID)
	if err != nil {
		return nil, err
	}
	var units []Unit
	for _, pc := range caps {
		if pc.State == "no_content" || pc.Group == "adult" {
			continue
		}
		rank := stateRank(pc.State)
		switch {
		case pc.Group == "firstparty":
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
					continue
				}
				key := domain.UnitKey{Team: strconv.Itoa(tr.ID), Category: "dub"}
				units = append(units, Unit{AnimeID: animeID, Provider: "kodik",
					Key: key, Episode: ep, Episodes: ep, StateRank: rank,
					Synth: &domain.UnitVerdict{Key: key, Episode: ep, Status: domain.StatusVerified,
						Audio: &domain.AudioVerdict{Lang: "ru", Confidence: 1.0, Verified: true}}})
			}

		case scraperProviders[pc.Provider]:
			eps, err := c.ScraperEpisodes(ctx, animeID, pc.Provider)
			if err != nil {
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
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].StateRank < units[j].StateRank })
	return units, nil
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
