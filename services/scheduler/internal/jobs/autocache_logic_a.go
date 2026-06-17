// Package jobs — autocache_logic_a.go is the Phase-9 (TRIG-01) "Logic A"
// ongoing-push producer. Every sweep it runs the proven hotcombos-style DISTINCT
// join over (watch_history × anime_list × animes) — all in the shared
// `animeenigma` DB the scheduler reads — adding a JP-audio filter, the D8
// active-watcher recency predicate (active_watcher_days, default 30), and the
// episodes_aired join, to find every ongoing anime with ≥1 active JP-audio-combo
// watcher, then fires an `ongoing`-reason demand for that anime's latest-aired
// episode to the library internal endpoint
// (POST /internal/library/autocache/demand, Docker-network-only).
//
// "As soon as on torrents" is satisfied by the library Planner (Plan 09-02)
// retrying the search each loop; this producer simply re-asserts demand every
// sweep. The library autocache_demand composite PK collapses repeats and the
// Planner backoff bounds the churn (RESEARCH Pitfall 4 / T-09-11), so re-asserting
// is idempotent and cheap.
//
// The library DB is SEPARATE (LIBRARY_DB_NAME), so the join CANNOT live there —
// all "who wants what" enumeration must run from a shared-DB producer like this
// one (CONTEXT load-bearing constraint).
package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

// logicADemandTimeout caps each per-demand POST. The demands are tiny
// (a 3-field JSON body to a Docker-network sink), so a short timeout keeps a slow
// or down library from stalling the whole sweep; a single blip self-heals on the
// next sweep (the demand is idempotent via the library PK).
const logicADemandTimeout = 5 * time.Second

// AutocacheLogicAJob enumerates ongoing anime with an active JP-audio watcher and
// re-asserts an `ongoing` demand for each one's latest-aired episode.
type AutocacheLogicAJob struct {
	db                *gorm.DB
	client            *http.Client
	libraryURL        string
	activeWatcherDays int
	log               *logger.Logger
}

// NewAutocacheLogicAJob constructs the job. libraryURL is the library service base
// URL (e.g. http://library:8089); the job appends the demand path itself.
// activeWatcherDays is the D8 recency window (default 30, a scheduler env mirror of
// the authoritative library autocache_config.active_watcher_days — see SUMMARY).
func NewAutocacheLogicAJob(db *gorm.DB, libraryURL string, activeWatcherDays int, log *logger.Logger) *AutocacheLogicAJob {
	return &AutocacheLogicAJob{
		db:                db,
		client:            &http.Client{Timeout: logicADemandTimeout},
		libraryURL:        libraryURL,
		activeWatcherDays: activeWatcherDays,
		log:               log,
	}
}

// logicARow is the per-ongoing target the join projects: the latest-aired episode
// (episodes_aired) of an ongoing anime that has ≥1 active JP-audio watcher.
type logicARow struct {
	ShikimoriID   string `gorm:"column:shikimori_id"`
	EpisodesAired int    `gorm:"column:episodes_aired"`
	NameJP        string `gorm:"column:name_jp"`
	Name          string `gorm:"column:name"`
	NameEN        string `gorm:"column:name_en"`
}

// titles returns the ordered, fallback-ranked title list for the demand
// (name_jp → romaji → name_en). The library JoinTitles drops empties/dups, but
// building the slice in producer order here keeps the fallback intent explicit.
func (r logicARow) titles() []string {
	return []string{r.NameJP, r.Name, r.NameEN}
}

// demandBody is the wire shape of POST /internal/library/autocache/demand. Titles
// is the ordered title list the library Planner searches trackers with (the
// library has no anime titles of its own).
type demandBody struct {
	MalID   string   `json:"mal_id"`
	Episode int      `json:"episode"`
	Reason  string   `json:"reason"`
	Titles  []string `json:"titles,omitempty"`
}

// Run executes the adapted hotcombos DISTINCT join and fires one ongoing demand
// per qualifying ongoing anime. It returns an error ONLY when the join query
// itself fails (so the JobService metrics wrap records a real failure). A single
// demand POST failure is logged + counted but does NOT abort the rest of the
// sweep — re-asserting demand is idempotent (library PK dedup), so a transient
// blip self-heals next sweep.
func (j *AutocacheLogicAJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting autocache Logic A sweep (ongoing push)")
	}

	// D8 recency: anime_list.updated_at must be within active_watcher_days. We
	// compute the cutoff in Go and bind it as a parameter so the same SQL runs on
	// both Postgres (prod) and SQLite (tests) without DB-specific interval syntax.
	cutoff := time.Now().AddDate(0, 0, -j.activeWatcherDays)

	// Adapted from notifications/internal/job/hotcombos.go:46 — the DISTINCT join
	// over (watch_history × anime_list × animes) WHERE watching + ongoing, plus the
	// Logic-A additions: raw-audio filter (any SUB combo, plus the ae/raw players —
	// skip DUB), the D8 recency predicate (al.updated_at > cutoff), and the
	// episodes_aired projection (the latest-aired episode target, A3). ANY sub combo
	// carries original Japanese audio regardless of subtitle language/provider, so
	// kodik/ru/sub, english/en/sub, hianime/en/sub etc. all qualify. (Corrected
	// 2026-06-17: was player∈{ae,raw} OR lang='ja', which wrongly dropped sub combos.)
	// All three tables live in animeenigma.
	const q = `
		SELECT DISTINCT
		    a.shikimori_id   AS shikimori_id,
		    a.episodes_aired AS episodes_aired,
		    a.name_jp        AS name_jp,
		    a.name           AS name,
		    a.name_en        AS name_en
		FROM watch_history wh
		JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
		JOIN animes a ON a.id = wh.anime_id
		WHERE al.status = 'watching'
		  AND a.status = 'ongoing'
		  AND (wh.watch_type = 'sub' OR wh.player IN ('ae', 'raw'))
		  AND al.updated_at > ?
	`

	var rows []logicARow
	if err := j.db.WithContext(ctx).Raw(q, cutoff).Scan(&rows).Error; err != nil {
		return fmt.Errorf("logic A enumeration join: %w", err)
	}

	demandsFired, demandsFailed, skipped := 0, 0, 0
	for _, r := range rows {
		// Skip rows with no valid demand target.
		if r.EpisodesAired <= 0 || r.ShikimoriID == "" {
			skipped++
			continue
		}
		if err := j.fireDemand(ctx, r.ShikimoriID, r.EpisodesAired, r.titles()); err != nil {
			demandsFailed++
			if j.log != nil {
				j.log.Warnw("autocache Logic A demand POST failed; continuing sweep",
					"shikimori_id", r.ShikimoriID,
					"episode", r.EpisodesAired,
					"error", err,
				)
			}
			continue
		}
		demandsFired++
	}

	if j.log != nil {
		j.log.Infow("autocache Logic A sweep completed",
			"rows", len(rows),
			"demands_fired", demandsFired,
			"demands_failed", demandsFailed,
			"skipped", skipped,
		)
	}
	return nil
}

// fireDemand POSTs a single {mal_id, episode, reason:"ongoing"} demand to the
// library endpoint. Plan 09-01 Task 4 fixed the demand handler to validate-and-
// honor the wire reason on this Docker-network-only path, so an `ongoing` demand
// is recorded as ongoing (no server-side override). Returns an error on a
// transport failure or a non-2xx response.
func (j *AutocacheLogicAJob) fireDemand(ctx context.Context, shikimoriID string, episode int, titles []string) error {
	body, err := json.Marshal(demandBody{
		MalID:   shikimoriID,
		Episode: episode,
		Reason:  "ongoing",
		Titles:  titles,
	})
	if err != nil {
		return fmt.Errorf("marshal demand: %w", err)
	}

	url := j.libraryURL + "/internal/library/autocache/demand"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build demand request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post demand: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("demand returned status %d", resp.StatusCode)
	}
	return nil
}
