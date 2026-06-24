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
		return j.fail(&report, "detector hot-combos collect failed", err)
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
		return j.fail(&report, "detector snapshot bulk-load failed", err)
	}

	// Step 3 — fan-out per-combo parser lookups.
	resultPerCombo, parserFailures := j.checkCombos(ctx, combos)
	report.ParserFailures = parserFailures

	// Step 4 — diff + bootstrap protection (NOTIF-DET-06) + never-lower
	// snapshot invariant (NOTIF-DET-10).
	affected, snapUpdates := diffSnapshots(resultPerCombo, snapshotMap)
	report.AffectedCombos = len(affected)

	// Step 5 — BulkUpsert snapshots BEFORE notifications. A mid-run crash
	// re-runs parser calls (idempotent) but does not re-fire notifications
	// against a now-newer snapshot.
	if err := j.snapshots.BulkUpsert(ctx, snapUpdates); err != nil {
		return j.fail(&report, "detector snapshot bulk-upsert failed", err)
	}

	if len(affected) == 0 {
		// No diffs after bootstrap + never-lower filter — success even if
		// parser had some failures (the failures just mean we couldn't
		// confirm; next hour retries).
		if parserFailures > 0 {
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
		return j.fail(&report, "detector max-watched lookup failed", err)
	}

	upserted := j.upsertNotifications(ctx, affected, resultPerCombo, maxByCombo)
	report.NotificationsUpserted = upserted

	// Outcome derivation (matches plan touch-list exactly):
	//   parserFailures == 0           → success (regardless of upserted)
	//   parserFailures > 0 && upserted>0 → partial
	//   parserFailures > 0 && upserted==0 → failed
	switch {
	case parserFailures == 0:
		j.recordOutcome("success", &report)
	case upserted > 0:
		j.recordOutcome("partial", &report)
	default:
		j.recordOutcome("failed", &report)
	}
	j.logCompleted(report)
	return report, nil
}

// fail stamps the "failed" outcome, logs the infrastructure error, and returns
// the report + err — the canonical exit for the four abort-the-run paths
// (collect, bulk-load, bulk-upsert, max-watched lookup).
func (j *NewEpisodeDetectorJob) fail(report *RunReport, msg string, err error) (RunReport, error) {
	j.recordOutcome("failed", report)
	if j.log != nil {
		j.log.Errorw(msg, "error", err)
	}
	return *report, err
}

// snapKey drops shikimori_id from a combo. Snapshot rows and watch_history
// (both keyed by parser identity) carry no shikimori_id, while combos from
// HotCombosCollector do — so every cross-lookup against those maps must
// normalise first, else lookups miss and we silently re-bootstrap forever
// (NOTIF-DET-06 violation).
func snapKey(c domain.Combo) domain.Combo {
	c.ShikimoriID = ""
	return c
}

// checkCombos fans out per-combo parser lookups (errgroup capped by
// WorkerLimit, per-call ParserTimeout). The result carries both the latest
// episode number (drives the diff) and the per-player translation title
// (rides into the new_episode payload). Per-combo failures are LOGGED +
// COUNTED but never abort the run; episode-not-found is a normal "no episode
// yet" signal and is NOT counted as a failure.
func (j *NewEpisodeDetectorJob) checkCombos(ctx context.Context, combos []domain.Combo) (map[domain.Combo]service.EpisodeCheckResult, int) {
	resultPerCombo := make(map[domain.Combo]service.EpisodeCheckResult, len(combos))
	var (
		mu             sync.Mutex
		parserFailures atomic.Int64
	)

	workerLimit := j.cfg.WorkerLimit
	if workerLimit <= 0 {
		workerLimit = 5
	}
	timeout := j.cfg.ParserTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workerLimit)
	for _, combo := range combos {
		combo := combo
		g.Go(func() error {
			callCtx, cancel := context.WithTimeout(gctx, timeout)
			defer cancel()
			result, err := j.checker.LatestEpisode(callCtx, combo)
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
			resultPerCombo[combo] = result
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return resultPerCombo, int(parserFailures.Load())
}

// diffSnapshots applies bootstrap protection (NOTIF-DET-06) and the
// never-lower invariant (NOTIF-DET-10) to each parser result, returning the
// combos with a genuine new episode plus the full set of snapshot writes
// (snapKey-normalised) for the BulkUpsert. snapshotMap is keyed by
// snapKey-normalised combos (parser_episode_snapshots rows carry no
// shikimori_id).
func diffSnapshots(
	resultPerCombo map[domain.Combo]service.EpisodeCheckResult,
	snapshotMap map[domain.Combo]int,
) (affected []domain.Combo, snapUpdates map[domain.Combo]int) {
	affected = make([]domain.Combo, 0, len(resultPerCombo))
	snapUpdates = make(map[domain.Combo]int, len(resultPerCombo))
	for combo, result := range resultPerCombo {
		key := snapKey(combo)
		prev, hadSnapshot := snapshotMap[key]
		switch {
		case !hadSnapshot:
			// BOOTSTRAP PROTECTION (NOTIF-DET-06): first time we see a
			// combo, record the snapshot but DO NOT fire a notification —
			// otherwise every existing user gets spammed about every
			// in-progress anime on first deploy.
			snapUpdates[key] = result.Latest
		case result.Latest <= prev:
			// Never lower the snapshot. Refresh checked_at by re-writing
			// the same value.
			snapUpdates[key] = prev
		default:
			// Genuine new episode discovered.
			snapUpdates[key] = result.Latest
			affected = append(affected, combo)
		}
	}
	return affected, snapUpdates
}

// upsertNotifications UPSERTs a new_episode notification per (user, combo)
// for every affected combo (D-DET-01 — in-process NotificationService, not
// HTTP self-loopback). maxByCombo is keyed by snapKey-normalised combos
// (watch_history has no shikimori_id column). Anime metadata is memoized
// across combos sharing an animeID. Returns the count successfully upserted.
func (j *NewEpisodeDetectorJob) upsertNotifications(
	ctx context.Context,
	affected []domain.Combo,
	resultPerCombo map[domain.Combo]service.EpisodeCheckResult,
	maxByCombo map[domain.Combo]map[string]int,
) int {
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
		result := resultPerCombo[combo]
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
			if firstUnwatched > result.Latest {
				// Defensive race guard: the user already watched the new
				// episode between the parser call and now. NOTIF-DET-07.
				continue
			}
			payload, err := service.BuildNewEpisodePayload(combo, anime, maxWatched, result.Latest, result.TranslationTitle)
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
	return upserted
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
