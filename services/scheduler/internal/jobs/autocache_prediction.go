// Package jobs — autocache_prediction.go is the Phase-11 (OBS-05) daily
// storage-need prediction producer. It is a LEANER sibling of the Phase-9 Logic A
// job (autocache_logic_a.go): it runs the SAME shared-DB DISTINCT join over
// (watch_history × anime_list × animes) — all in the `animeenigma` DB the
// scheduler reads — but instead of firing a per-row HTTP demand it only COUNTS
// rows and publishes a heuristic gauge.
//
// The prediction needs the same shared-DB watcher/combo counts Logic A enumerates,
// and the library DB is SEPARATE (LIBRARY_DB_NAME) — so the heuristic CANNOT live
// in the library and must run from a shared-DB producer like this one. The gauge
// (metrics.AutocachePredictedBytes) keeps the `library_autocache_` name prefix
// DELIBERATELY so OBS-05's Grafana table can union it with the library-exposed
// library_autocache_budget_bytes (see libs/metrics/scheduler.go).
//
// Two components, both = a distinct-anime COUNT × avg_raw_ep_size. The count keys
// on DISTINCT a.id (the non-null PK) and excludes empty shikimori_id, so it matches
// the set of anime Logic A actually fans out demand for (Logic A skips empty
// shikimori_id — autocache_logic_a.go:121); see the predictionOngoingQuery comment:
//   - ongoing: distinct ONGOING anime (with a non-empty shikimori_id) with ≥1 active
//     JP-audio watcher — the Logic A join, counted per a.id.
//   - nextep:  distinct anime with an active JP-audio watcher in the last
//     active_watcher_days, regardless of a.status (the same join MINUS the
//     a.status='ongoing' clause — spec §7 "distinct JP-combo watching-anime
//     active in last 30d").
//
// The heuristic is intentionally COARSE for v1 (spec §7); an AI prediction model
// supersedes it in v2.
package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"gorm.io/gorm"
)

// AutocachePredictionJob counts the two heuristic components off the shared Logic A
// watcher join and publishes them (× avgRawEpBytes) as a {component} gauge. Unlike
// Logic A it has NO http.Client / libraryURL — it only reads the shared DB and sets
// a Prometheus gauge, so there is no per-row fan-out.
type AutocachePredictionJob struct {
	db                *gorm.DB
	activeWatcherDays int
	avgRawEpBytes     int64
	log               *logger.Logger
}

// NewAutocachePredictionJob constructs the job. activeWatcherDays is the recency
// window (a scheduler env mirror of the library autocache_config value);
// avgRawEpBytes is the assumed average bytes of one raw episode (spec §7
// avg_raw_ep_size mirror, default ~1.2 GiB).
func NewAutocachePredictionJob(db *gorm.DB, activeWatcherDays int, avgRawEpBytes int64, log *logger.Logger) *AutocachePredictionJob {
	return &AutocachePredictionJob{
		db:                db,
		activeWatcherDays: activeWatcherDays,
		avgRawEpBytes:     avgRawEpBytes,
		log:               log,
	}
}

// The shared DISTINCT-join body (verbatim from Logic A, autocache_logic_a.go:104-110)
// counted two ways. predictionOngoingQuery keeps the a.status='ongoing' clause;
// predictionNextepQuery drops it. Both bind the Go-computed recency cutoff as `?`
// so the same SQL runs on Postgres (prod) and SQLite (tests) without DB-specific
// interval syntax. Wrapping the DISTINCT projection in a COUNT(*) subquery yields
// the distinct-anime count for each component.
//
// IN-01 alignment: count DISTINCT a.id (the guaranteed-non-null PK) rather than
// a.shikimori_id (size:50;index — nullable, non-unique), and EXCLUDE rows with an
// empty shikimori_id (AND a.shikimori_id <> ''). This mirrors Logic A's actual
// fan-out: it projects per-row a.id/a.shikimori_id and SKIPS empty shikimori_id
// downstream (autocache_logic_a.go:121), so the heuristic count now equals the
// number of distinct anime Logic A would actually fire demand for — no longer
// collapsing all empty-shikimori_id anime into one distinct bucket.
// Both queries splice in the SHARED rawAudioWatcherPredicate const (defined in
// autocache_logic_a.go) so the prediction join stays byte-for-byte identical to
// Logic A's raw-audio filter and the two cannot drift again — the package header
// promises they are the same. predictionOngoingQuery keeps the a.status='ongoing'
// clause; predictionNextepQuery drops it.
const predictionOngoingQuery = `
	SELECT count(*) FROM (
		SELECT DISTINCT a.id
		FROM watch_history wh
		JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
		JOIN animes a ON a.id = wh.anime_id
		WHERE al.status = 'watching'
		  AND a.status = 'ongoing'
		  AND a.shikimori_id <> ''
		  AND ` + rawAudioWatcherPredicate + `
		  AND al.updated_at > ?
	) t
`

const predictionNextepQuery = `
	SELECT count(*) FROM (
		SELECT DISTINCT a.id
		FROM watch_history wh
		JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
		JOIN animes a ON a.id = wh.anime_id
		WHERE al.status = 'watching'
		  AND a.shikimori_id <> ''
		  AND ` + rawAudioWatcherPredicate + `
		  AND al.updated_at > ?
	) t
`

// Run computes the two distinct-anime counts and sets the {component} gauge to
// count × avgRawEpBytes. It returns an error ONLY when a count query fails (so the
// JobService metrics wrap records a real failure); a clean run with zero rows sets
// both gauges to 0 and returns nil.
func (j *AutocachePredictionJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting autocache prediction sweep (storage-need heuristic)")
	}

	cutoff := time.Now().AddDate(0, 0, -j.activeWatcherDays)

	var ongoing int64
	if err := j.db.WithContext(ctx).Raw(predictionOngoingQuery, cutoff).Scan(&ongoing).Error; err != nil {
		return fmt.Errorf("autocache prediction ongoing count: %w", err)
	}

	var nextep int64
	if err := j.db.WithContext(ctx).Raw(predictionNextepQuery, cutoff).Scan(&nextep).Error; err != nil {
		return fmt.Errorf("autocache prediction nextep count: %w", err)
	}

	metrics.AutocachePredictedBytes.WithLabelValues("ongoing").Set(float64(ongoing) * float64(j.avgRawEpBytes))
	metrics.AutocachePredictedBytes.WithLabelValues("nextep").Set(float64(nextep) * float64(j.avgRawEpBytes))

	if j.log != nil {
		j.log.Infow("autocache prediction sweep completed",
			"ongoing_count", ongoing,
			"nextep_count", nextep,
			"avg_raw_ep_bytes", j.avgRawEpBytes,
		)
	}
	return nil
}
