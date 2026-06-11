package job

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/config"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/service"
	"golang.org/x/sync/errgroup"
)

// RunReport summarises a single detector run. Returned by Run and surfaced
// by the admin trigger endpoint (`POST /internal/detector/run-once`).
type RunReport struct {
	CombosScanned         int    `json:"combos_scanned"`
	AffectedCombos        int    `json:"affected_combos"`
	NotificationsUpserted int    `json:"notifications_upserted"`
	ParserFailures        int    `json:"parser_failures"`
	DurationMs            int64  `json:"duration_ms"`
	Outcome               string `json:"outcome"`
}

// NewEpisodeDetectorJob runs the design-doc §Detection Flow once per call:
//
//  1. Collect hot combos (DISTINCT join over watch_history × anime_list ×
//     animes; status='watching' + 'ongoing' + translation_id != '').
//  2. BulkLoad prior parser_episode_snapshots.
//  3. Fan-out per-combo parser lookups via the EpisodeChecker port
//     (errgroup cap WorkerLimit, per-call ParserTimeout). Per-combo
//     failures are LOGGED + COUNTED but DO NOT abort the run.
//  4. Diff each parser-reported latest against the prior snapshot:
//     - No prior snapshot → bootstrap (record snapshot, NO notification)
//       per NOTIF-DET-06.
//     - latest <= prior → never lower the snapshot; skip the combo
//       (NOTIF-DET-10).
//     - latest > prior → record affected.
//  5. BulkUpsert snapshots BEFORE notifications (mid-run crash recovery:
//     next run re-runs parser calls idempotently and the un-fired
//     notifications surface on the next diff).
//  6. For each affected combo, iterate (user, max_watched) pairs and
//     UPSERT a new_episode notification via the in-process
//     NotificationService (D-DET-01 — NOT HTTP self-loopback).
type NewEpisodeDetectorJob struct {
	hotCombos  *HotCombosCollector
	checker    service.EpisodeChecker
	snapshots  *repo.SnapshotRepository
	maxWatched *repo.MaxWatchedRepository
	animeRepo  *repo.AnimeViewRepository
	notif      *service.NotificationService
	cfg        *config.DetectorConfig
	log        *logger.Logger
}

// NewEpisodeDetectorJobNew constructs the job. Naming is `NewEpisodeDetectorJobNew`
// to disambiguate from the type literal `NewEpisodeDetectorJob` — Go's
// constructor convention says `NewX` for type `X`, but here `X` already
// begins with `New`. Project precedent (services/scheduler/internal/jobs/)
// uses the explicit `NewXJob` constructor when the type's own name starts
// with `New`. Following the precedent.
func NewEpisodeDetectorJobNew(
	hotCombos *HotCombosCollector,
	checker service.EpisodeChecker,
	snapshots *repo.SnapshotRepository,
	maxWatched *repo.MaxWatchedRepository,
	animeRepo *repo.AnimeViewRepository,
	notif *service.NotificationService,
	cfg *config.DetectorConfig,
	log *logger.Logger,
) *NewEpisodeDetectorJob {
	return &NewEpisodeDetectorJob{
		hotCombos:  hotCombos,
		checker:    checker,
		snapshots:  snapshots,
		maxWatched: maxWatched,
		animeRepo:  animeRepo,
		notif:      notif,
		cfg:        cfg,
		log:        log,
	}
}

// Run executes one detector pass. Returns a RunReport for observability +
// the admin endpoint; the returned error is non-nil only on infrastructure
// failures (DB unreachable, etc.) — per-combo parser failures are
// reflected in the report and metrics, not returned as errors.
func (j *NewEpisodeDetectorJob) Run(ctx context.Context) (RunReport, error) {
	report := RunReport{Outcome: "success"}
	start := time.Now()
	defer func() {
		report.DurationMs = time.Since(start).Milliseconds()
		NotificationsDetectorDurationSeconds.Observe(time.Since(start).Seconds())
	}()

	if j.log != nil {
		j.log.Infow("detector run started")
	}

	// Step 1 — collect hot combos.
	combos, err := j.hotCombos.Collect(ctx)
	if err != nil {
		j.recordOutcome("failed", &report)
		if j.log != nil {
			j.log.Errorw("detector hot-combos collect failed", "error", err)
		}
		return report, err
	}
	report.CombosScanned = len(combos)
	NotificationsDetectorCombosScanned.Set(float64(len(combos)))

	if len(combos) == 0 {
		// No active combos to scan — this is a clean success.
		j.recordOutcome("success", &report)
		j.logCompleted(report)
		return report, nil
	}

	// Step 2 — bulk-load prior snapshots.
	snapshotMap, err := j.snapshots.BulkLoad(ctx, combos)
	if err != nil {
		j.recordOutcome("failed", &report)
		if j.log != nil {
			j.log.Errorw("detector snapshot bulk-load failed", "error", err)
		}
		return report, err
	}

	// Step 3 — fan-out per-combo parser lookups.
	latestPerCombo := make(map[domain.Combo]int, len(combos))
	var (
		mu             sync.Mutex
		parserFailures atomic.Int64
	)
	g, gctx := errgroup.WithContext(ctx)
	workerLimit := j.cfg.WorkerLimit
	if workerLimit <= 0 {
		workerLimit = 5
	}
	g.SetLimit(workerLimit)

	timeout := j.cfg.ParserTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	for _, combo := range combos {
		combo := combo
		g.Go(func() error {
			callCtx, cancel := context.WithTimeout(gctx, timeout)
			defer cancel()
			latest, err := j.checker.LatestEpisode(callCtx, combo)
			if err != nil {
				if service.IsEpisodeNotFound(err) {
					// Not-found is a normal "no episode for this combo
					// yet" signal — silently skip, do NOT count as
					// parser failure.
					return nil
				}
				NotificationsDetectorParserFailuresTotal.WithLabelValues(combo.Player).Inc()
				parserFailures.Add(1)
				if j.log != nil {
					j.log.Warnw("parser failed",
						"anime_id", combo.AnimeID,
						"player", combo.Player,
						"translation_id", combo.TranslationID,
						"error", err,
					)
				}
				return nil
			}
			mu.Lock()
			latestPerCombo[combo] = latest
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	report.ParserFailures = int(parserFailures.Load())

	// Step 4 — diff + bootstrap protection (NOTIF-DET-06) + never-lower
	// snapshot invariant (NOTIF-DET-10).
	//
	// IMPORTANT: snapshotMap keys are built from parser_episode_snapshots
	// rows (which do NOT carry shikimori_id). The combos collected by
	// HotCombosCollector DO carry shikimori_id (for the catalog HTTP call).
	// We must drop shikimori_id before keying into snapshotMap, otherwise
	// every lookup returns hadSnapshot=false and we accidentally bootstrap
	// every run forever (silent NOTIF-DET-06 violation).
	affected := make([]domain.Combo, 0, len(latestPerCombo))
	snapUpdates := make(map[domain.Combo]int, len(latestPerCombo))
	snapKey := func(c domain.Combo) domain.Combo {
		c.ShikimoriID = ""
		return c
	}
	for combo, latest := range latestPerCombo {
		key := snapKey(combo)
		prev, hadSnapshot := snapshotMap[key]
		if !hadSnapshot {
			// BOOTSTRAP PROTECTION (NOTIF-DET-06): first time we see a
			// combo, record the snapshot but DO NOT fire a notification —
			// otherwise every existing user gets spammed about every
			// in-progress anime on first deploy.
			snapUpdates[key] = latest
			continue
		}
		if latest <= prev {
			// Never lower the snapshot. Refresh checked_at by re-writing
			// the same value.
			snapUpdates[key] = prev
			continue
		}
		// Genuine new episode discovered.
		snapUpdates[key] = latest
		affected = append(affected, combo)
	}
	report.AffectedCombos = len(affected)

	// Step 5 — BulkUpsert snapshots BEFORE notifications. A mid-run crash
	// re-runs parser calls (idempotent) but does not re-fire notifications
	// against a now-newer snapshot.
	if err := j.snapshots.BulkUpsert(ctx, snapUpdates); err != nil {
		j.recordOutcome("failed", &report)
		if j.log != nil {
			j.log.Errorw("detector snapshot bulk-upsert failed", "error", err)
		}
		return report, err
	}

	if len(affected) == 0 {
		// No diffs after bootstrap + never-lower filter — success even if
		// parser had some failures (the failures just mean we couldn't
		// confirm; next hour retries).
		if parserFailures.Load() > 0 {
			j.recordOutcome("partial", &report)
		} else {
			j.recordOutcome("success", &report)
		}
		j.logCompleted(report)
		return report, nil
	}

	// Step 6 — UPSERT notifications per (user, combo). MaxWatched queries
	// watch_history (no shikimori_id column), so normalise the affected
	// combo keys before passing in — keeps ForCombos's returned map keys
	// shape-stable with the snapKey lookup below.
	affectedForLookup := make([]domain.Combo, len(affected))
	for i, c := range affected {
		affectedForLookup[i] = snapKey(c)
	}
	maxByCombo, err := j.maxWatched.ForCombos(ctx, affectedForLookup)
	if err != nil {
		j.recordOutcome("failed", &report)
		if j.log != nil {
			j.log.Errorw("detector max-watched lookup failed", "error", err)
		}
		return report, err
	}

	// Memoize anime metadata across combos sharing animeID. Typical user
	// has multiple translation combos for the same anime — single fetch
	// covers them all.
	animeCache := make(map[string]*repo.AnimeView)
	getAnime := func(animeID string) (*repo.AnimeView, error) {
		if cached, ok := animeCache[animeID]; ok {
			return cached, nil
		}
		v, err := j.animeRepo.GetByID(ctx, animeID)
		if err != nil {
			return nil, err
		}
		animeCache[animeID] = v
		return v, nil
	}

	upserted := 0
	for _, combo := range affected {
		// maxByCombo is keyed by combo WITHOUT shikimori_id (watch_history
		// has no shikimori_id column). Same normalisation as snapshot
		// lookup above.
		users, ok := maxByCombo[snapKey(combo)]
		if !ok {
			continue
		}
		latest := latestPerCombo[combo]
		anime, err := getAnime(combo.AnimeID)
		if err != nil {
			if j.log != nil {
				j.log.Warnw("detector anime view lookup failed",
					"anime_id", combo.AnimeID, "error", err)
			}
			continue
		}
		for userID, maxWatched := range users {
			firstUnwatched := maxWatched + 1
			if firstUnwatched > latest {
				// Defensive race guard: the user already watched the new
				// episode between the parser call and now. NOTIF-DET-07.
				continue
			}
			payload, err := service.BuildNewEpisodePayload(combo, anime, maxWatched, latest, "")
			if err != nil {
				if j.log != nil {
					j.log.Warnw("detector payload build failed",
						"anime_id", combo.AnimeID,
						"player", combo.Player,
						"translation_id", combo.TranslationID,
						"error", err)
				}
				continue
			}
			dedupeKey := service.NewEpisodeDedupeKey(
				combo.AnimeID, combo.Player, combo.Language, combo.WatchType, combo.TranslationID)
			_, err = j.notif.Upsert(ctx, service.UpsertRequest{
				UserID:    userID,
				Type:      string(domain.TypeNewEpisode),
				DedupeKey: dedupeKey,
				Payload:   payload,
			})
			if err != nil {
				if j.log != nil {
					j.log.Warnw("detector notification upsert failed",
						"user_id", userID,
						"anime_id", combo.AnimeID,
						"error", err)
				}
				continue
			}
			NotificationsCreatedTotal.WithLabelValues(string(domain.TypeNewEpisode), "detector").Inc()
			upserted++
		}
	}
	report.NotificationsUpserted = upserted

	// Outcome derivation (matches plan touch-list exactly):
	//   parserFailures == 0           → success (regardless of upserted)
	//   parserFailures > 0 && upserted>0 → partial
	//   parserFailures > 0 && upserted==0 → failed
	switch {
	case parserFailures.Load() == 0:
		j.recordOutcome("success", &report)
	case upserted > 0:
		j.recordOutcome("partial", &report)
	default:
		j.recordOutcome("failed", &report)
	}
	j.logCompleted(report)
	return report, nil
}

// recordOutcome stamps the outcome on the report AND bumps the runs counter.
func (j *NewEpisodeDetectorJob) recordOutcome(outcome string, r *RunReport) {
	r.Outcome = outcome
	NotificationsDetectorRunsTotal.WithLabelValues(outcome).Inc()
}

// logCompleted writes the NOTIF-NF-02 "detector run completed" line with
// the five required fields and zero PII.
func (j *NewEpisodeDetectorJob) logCompleted(r RunReport) {
	if j.log == nil {
		return
	}
	j.log.Infow("detector run completed",
		"combos_scanned", r.CombosScanned,
		"affected_combos", r.AffectedCombos,
		"notifications_upserted", r.NotificationsUpserted,
		"duration_ms", r.DurationMs,
		"parser_failures", r.ParserFailures,
		"outcome", r.Outcome,
	)
}
