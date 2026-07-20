package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// englishDubRepo is the minimal anime-repository surface the backfiller needs.
// Production wiring satisfies it via *repo.AnimeRepository.
type englishDubRepo interface {
	ListEnglishDubCandidates(ctx context.Context, limit int, ongoingAge, staleAge time.Duration) ([]domain.EnglishDubCandidate, error)
	TouchEnglishDubChecked(ctx context.Context, animeID string) error
	CountEnglishDubUnchecked(ctx context.Context) (int64, error)
	PromoteVerifiedEnglishDubs(ctx context.Context) (int64, error)
}

// englishDubProbe is the scraper-episodes surface the backfiller drives.
// Production wiring passes *scraperOps, whose GetScraperEpisodes performs the
// has_english / has_english_dub write as a side effect — the backfiller never
// writes a verdict itself, it only decides when to ask.
type englishDubProbe interface {
	GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error)
}

// shedChecker is satisfied by *cache.DegradationWatcher.
type shedChecker interface {
	ShouldShed(min int) bool
}

// EnglishDubBackfillConfig tunes the loop.
type EnglishDubBackfillConfig struct {
	Interval     time.Duration
	OngoingAge   time.Duration
	StaleAge     time.Duration
	PromoteEvery time.Duration
}

// EnglishDubBackfiller keeps animes.has_english_dub fresh. It probes exactly
// ONE title per tick: the lazy hook on the scraper-episodes path covers titles
// users actually open, and this loop exists only to reach the long tail
// without putting meaningful load on providers (each probe fans out to real
// upstreams, some through the Camoufox sidecar).
type EnglishDubBackfiller struct {
	repo  englishDubRepo
	probe englishDubProbe
	shed  shedChecker
	cfg   EnglishDubBackfillConfig
	log   *logger.Logger
}

func NewEnglishDubBackfiller(repo englishDubRepo, probe englishDubProbe, shed shedChecker, cfg EnglishDubBackfillConfig, log *logger.Logger) *EnglishDubBackfiller {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.OngoingAge <= 0 {
		cfg.OngoingAge = 7 * 24 * time.Hour
	}
	if cfg.StaleAge <= 0 {
		cfg.StaleAge = 30 * 24 * time.Hour
	}
	if cfg.PromoteEvery <= 0 {
		cfg.PromoteEvery = time.Hour
	}
	return &EnglishDubBackfiller{repo: repo, probe: probe, shed: shed, cfg: cfg, log: log}
}

// Start runs until ctx is cancelled.
func (b *EnglishDubBackfiller) Start(ctx context.Context) {
	b.log.Infow("english dub backfiller started",
		"interval", b.cfg.Interval.String(),
		"ongoing_age", b.cfg.OngoingAge.String(),
		"stale_age", b.cfg.StaleAge.String(),
	)

	b.promote(ctx)

	ticker := time.NewTicker(b.cfg.Interval)
	defer ticker.Stop()
	promoteTicker := time.NewTicker(b.cfg.PromoteEvery)
	defer promoteTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("english dub backfiller stopped")
			return
		case <-promoteTicker.C:
			b.promote(ctx)
		case <-ticker.C:
			b.tick(ctx)
		}
	}
}

// promote runs the network-free content-verify pass. Audio-verified English
// beats provider metadata, so this may flip a title the scraper pass
// concluded false on. Non-fatal: content-verify is a separate service and its
// table may not exist in every deployment.
func (b *EnglishDubBackfiller) promote(ctx context.Context) {
	n, err := b.repo.PromoteVerifiedEnglishDubs(ctx)
	if err != nil {
		b.log.Warnw("english dub promote from content-verify failed", "error", err)
		return
	}
	if n > 0 {
		englishDubPromotedTotal.Add(float64(n))
		b.log.Infow("english dub promoted from verified audio", "count", n)
	}
}

// tick probes at most one title.
func (b *EnglishDubBackfiller) tick(ctx context.Context) {
	if b.shed != nil && b.shed.ShouldShed(1) {
		englishDubBackfillTotal.WithLabelValues("shed").Inc()
		return
	}

	if n, err := b.repo.CountEnglishDubUnchecked(ctx); err == nil {
		englishDubUnchecked.Set(float64(n))
	}

	candidates, err := b.repo.ListEnglishDubCandidates(ctx, 1, b.cfg.OngoingAge, b.cfg.StaleAge)
	if err != nil {
		englishDubBackfillTotal.WithLabelValues("error").Inc()
		b.log.Warnw("english dub candidate query failed", "error", err)
		return
	}
	if len(candidates) == 0 {
		return
	}
	c := candidates[0]

	// prefer="" on purpose: only a chain-wide answer earns the right to write
	// a NEGATIVE verdict (see backfillEnglishFlags' honesty rule).
	status, body, err := b.probe.GetScraperEpisodes(ctx, c.ID, "", false)
	if err != nil || status != 200 {
		b.stamp(ctx, c, "probe unreachable", err, status)
		return
	}
	_, hasDub, ok := parseScraperEpisodes(body)
	if !ok {
		b.stamp(ctx, c, "no episodes in response", nil, status)
		return
	}

	// The verdict was already persisted by the hook inside GetScraperEpisodes.
	result := "nodub"
	if hasDub {
		result = "dub"
	}
	englishDubBackfillTotal.WithLabelValues(result).Inc()
	b.log.Infow("english dub verdict", "anime_id", c.ID, "name", c.Name, "has_dub", hasDub)
}

// stamp records an inconclusive probe so the loop rotates to the next title
// instead of retrying this one every tick.
func (b *EnglishDubBackfiller) stamp(ctx context.Context, c domain.EnglishDubCandidate, reason string, err error, status int) {
	englishDubBackfillTotal.WithLabelValues("stamped").Inc()
	if terr := b.repo.TouchEnglishDubChecked(ctx, c.ID); terr != nil {
		b.log.Warnw("english dub stamp failed", "anime_id", c.ID, "error", terr)
	}
	b.log.Debugw("english dub probe inconclusive",
		"anime_id", c.ID, "name", c.Name, "reason", reason, "status", status, "error", err)
}
