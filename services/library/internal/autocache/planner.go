package autocache

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/library/internal/metrics"
)

// planner.go is the Phase-09 drain loop (TRIG-03/04/05): a config-gated ticker
// that drains autocache_demand and turns wanted (mal_id, episode) rows into RAW
// download jobs, with single-flight dedup (present-check + in-flight-check),
// RAW/quality/seeder gating (selectRAW, raw_filter.go), and backfill-drain
// semantics. The library DB cannot see watch data (cross-DB boundary, RESEARCH
// Pitfall 1) — demand only ever arrives via autocache_demand, which this loop
// consumes.

// --- interface seams (satisfied by the existing repos/searcher; main.go owns
// the concrete wiring so this package imports no parser/repo package) ---

// demandDrainer is the slice of *repo.DemandRepository the Planner needs.
type demandDrainer interface {
	Drain(ctx context.Context, limit int) ([]domain.AutocacheDemand, error)
	DrainWeighted(ctx context.Context, hotN, coldN int) ([]domain.AutocacheDemand, error)
	Delete(ctx context.Context, malID string, episode int) error
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}

// presenceChecker is the slice of *repo.EpisodeRepository the Planner needs.
// GetByShikimoriEpisode returns a non-nil *Episode + nil error when present, or
// a not-found error when absent (the present-check). The Planner passes storage=""
// (present-in-ANY-backend, minio-first): autocache's job is making an episode
// AVAILABLE, and a copy on EITHER local minio OR external s3 means it is already
// watchable — so a present row in either store correctly suppresses a new download.
type presenceChecker interface {
	GetByShikimoriEpisode(ctx context.Context, shikimoriID string, episodeNumber int, storage string) (*domain.Episode, error)
}

// jobEnqueuer is the slice of *repo.JobRepository the Planner needs.
type jobEnqueuer interface {
	HasActiveForEpisode(ctx context.Context, shikimoriID string, episode int) (bool, error)
	Create(ctx context.Context, job *domain.Job) error
}

// budgetEvictor is the Phase-10 pre-admit seam (EnsureRoom only — the *autocache.Evictor
// method from Plan 02). main.go injects the concrete Evictor; nil is allowed (a Planner
// built with no evictor skips the pre-admit gate, mirroring the nil-guarded metrics/log).
// On the SAME EnsureRoom both the Planner and the admin upload handler gate the budget
// (EVICT-05), so an unfittable download is rejected rather than blowing the pool budget.
type budgetEvictor interface {
	EnsureRoom(ctx context.Context, estBytes int64) (admitted bool, err error)
}

// SearchQuery is the Planner's local search-input shape. main.go adapts
// *service.TieredSearcher (whose FetchAll takes service.SearchParams) to this so
// the autocache package stays free of any service-layer import.
type SearchQuery struct {
	Query string
	MALID int
	Limit int
}

// SearchResult is the Planner's local search-output shape. Releases is the
// seeder-ranked DESC slice selectRAW consumes.
type SearchResult struct {
	Releases []domain.Release
}

// searcher is the torrent-search seam. *tieredSearcherAdapter (main.go)
// satisfies it.
type searcher interface {
	FetchAll(ctx context.Context, q SearchQuery) (SearchResult, error)
}

// configGetter is the slice of *repo.AutocacheConfigRepository the Planner reads
// live each tick (master enabled + sweep_interval_min + quality_cap + min_seeders).
type configGetter interface {
	Get(ctx context.Context) (*domain.AutocacheConfig, error)
}

// plannerLogger is the structured-logging seam (a subset of *logger.Logger).
// nil is allowed (the Planner is nil-guarded).
type plannerLogger interface {
	Infow(msg string, keysAndValues ...any)
	Warnw(msg string, keysAndValues ...any)
	Errorw(msg string, keysAndValues ...any)
}

const (
	// drainBatchLimit caps how many demand rows one sweep loads (T-09-02 DoS
	// guard — the Drain repo method enforces the bound, this is the value).
	drainBatchLimit = 50
	// searchFanoutLimit caps the number of Jackett searches issued per sweep so
	// a large demand table cannot trigger a thundering herd (RESEARCH Pitfall 4).
	searchFanoutLimit = 5
	// searchBackoff is the per-(mal,ep) re-search cooldown: a demand that yielded
	// no qualifying release is not re-searched within this window (RESEARCH
	// Pitfall 4 — a not-yet-released episode otherwise re-searches every tick).
	searchBackoff = time.Hour
	// maxDemandAge is the expiry safety-valve (audit #20): a demand row first
	// requested longer ago than this is aged out so an unsatisfiable head can't
	// permanently starve newer rows behind the FIFO drain window. Producers
	// re-assert live demand on watch activity, so a still-wanted episode is
	// simply re-recorded with a fresh requested_at.
	maxDemandAge = 14 * 24 * time.Hour
	// minSweepInterval floors a misconfigured/zero sweep_interval_min so the loop
	// can never busy-spin.
	minSweepInterval = time.Minute
	// AvgRawEpSize is the pre-admit budget estimate fallback (~1.2 GiB) used when an
	// incoming download reports SizeBytes <= 0 (a selected release with no declared
	// size, or an admin upload that omitted size_bytes). It is a Phase-10 const, NOT a
	// config column (CONTEXT: avg_raw_ep_size is a const this phase, no schema change).
	// Exported (WR-03) so the admin upload handler can apply the SAME fallback the
	// Planner uses, keeping both pre-admit paths symmetric on the unknown-size case.
	AvgRawEpSize int64 = 1288490188 // ~1.2 GiB
)

// hotShare is the fraction of each drain batch reserved for hot-reason demand
// (next_ep, ongoing) over backfill (spec §5). Env-overridable; clamped to [0,1].
var hotShare = envHotShare()

func envHotShare() float64 {
	if v := os.Getenv("AUTOCACHE_HOT_SHARE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			return f
		}
	}
	return 0.70
}

// Planner drains autocache_demand → RAW download jobs on a config-gated ticker.
type Planner struct {
	demand   demandDrainer
	presence presenceChecker
	jobs     jobEnqueuer
	search   searcher
	config   configGetter
	evictor  budgetEvictor // Phase-10 pre-admit gate; nil → gate skipped.
	metrics  *metrics.LibraryMetrics
	log      plannerLogger

	// lastSearched is the per-(mal:ep) no-release backoff map (Pitfall 4). Guarded
	// by mu since runOnce could in principle be called concurrently in tests.
	mu           sync.Mutex
	lastSearched map[string]time.Time

	stop chan struct{}
	done chan struct{}
}

// NewPlanner wires the Planner from its seams. metrics + log may be nil (the
// Planner is nil-guarded throughout).
func NewPlanner(
	demand demandDrainer,
	presence presenceChecker,
	jobs jobEnqueuer,
	search searcher,
	config configGetter,
	evictor budgetEvictor,
	libMetrics *metrics.LibraryMetrics,
	log plannerLogger,
) *Planner {
	return &Planner{
		demand:       demand,
		presence:     presence,
		jobs:         jobs,
		search:       search,
		config:       config,
		evictor:      evictor,
		metrics:      libMetrics,
		log:          log,
		lastSearched: make(map[string]time.Time),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start launches the drain-loop goroutine. It returns immediately; the loop runs
// until Stop() is called or ctx is cancelled. Mirrors the WorkerPool lifecycle.
func (p *Planner) Start(ctx context.Context) {
	go p.loop(ctx)
}

// Stop signals the loop to exit and waits for it (bounded by the in-flight
// sleep). Safe to call once.
func (p *Planner) Stop() {
	select {
	case <-p.stop:
		// already stopped
	default:
		close(p.stop)
	}
	<-p.done
}

// loop is the ctx-aware sweep loop. Each iteration re-reads config live, then —
// if enabled — drains and processes a batch. The cadence (sweep_interval_min) is
// re-read every tick so an admin PATCH takes effect without a redeploy.
func (p *Planner) loop(ctx context.Context) {
	defer close(p.done)
	for {
		cadence := p.runOnce(ctx)
		if !p.sleep(ctx, cadence) {
			return
		}
	}
}

// runOnce performs one sweep and returns the cadence to sleep before the next.
// Exported-for-test via the package: the unit drives runOnce directly so it
// never has to wait on a ticker. When the master switch is off it drains/enqueues
// nothing (POOL-05 / T-09-06).
func (p *Planner) runOnce(ctx context.Context) time.Duration {
	cfg, err := p.config.Get(ctx)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("autocache planner: config read failed, skipping sweep", "error", err)
		}
		return minSweepInterval
	}
	cadence := time.Duration(cfg.SweepIntervalMin) * time.Minute
	if cadence < minSweepInterval {
		cadence = minSweepInterval
	}
	if !cfg.Enabled {
		return cadence
	}

	// WR-03: evict stale backoff entries before processing so the lastSearched
	// map can't grow without bound over a long-running process with a churning
	// catalog (new episodes weekly across thousands of ongoing titles). An entry
	// older than searchBackoff no longer suppresses a re-search, so dropping it is
	// behavior-preserving.
	p.gcBackoff()

	// Expiry safety-valve (audit #20): age out demand rows older than
	// maxDemandAge so the FIFO drain can't be permanently starved by a head of
	// unsatisfiable rows that never become present (and so are never deleted).
	// Safe because producers re-assert live demand on watch activity.
	if n, err := p.demand.DeleteExpired(ctx, time.Now().Add(-maxDemandAge)); err != nil {
		if p.log != nil {
			p.log.Warnw("autocache planner: expire stale demand failed", "error", err)
		}
	} else if n > 0 && p.log != nil {
		p.log.Infow("autocache planner: expired stale demand rows", "count", n, "max_age", maxDemandAge.String())
	}

	hotN := int(float64(drainBatchLimit) * hotShare)
	coldN := drainBatchLimit - hotN
	rows, err := p.demand.DrainWeighted(ctx, hotN, coldN)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("autocache planner: weighted drain failed; falling back to FIFO", "error", err)
		}
		rows, err = p.demand.Drain(ctx, drainBatchLimit)
		if err != nil {
			if p.log != nil {
				p.log.Warnw("autocache planner: drain failed", "error", err)
			}
			return cadence
		}
	}

	searches := 0
	for i := range rows {
		if ctx.Err() != nil {
			return cadence
		}
		searched := p.plan(ctx, rows[i], cfg, searches < searchFanoutLimit)
		if searched {
			searches++
		}
	}
	return cadence
}

// plan processes one drained demand row in order: present-check → in-flight-check
// → search → filter → enqueue. It returns true when it actually issued a search
// (so the caller can cap per-sweep fan-out). The trigger label is derived from
// the demand reason (ongoing→A, next_ep→B, backfill→backfill).
func (p *Planner) plan(ctx context.Context, d domain.AutocacheDemand, cfg *domain.AutocacheConfig, maySearch bool) (searched bool) {
	trigger := triggerForReason(d.Reason)

	// 1. Present-check (TRIG-04): already in the pool → enqueue nothing, drop the
	// demand row (RESEARCH Pitfall 6 — delete only on confirmed presence).
	// storage="" = present in EITHER backend (minio OR s3) counts as available,
	// which is the correct gate here: autocache exists to make an episode
	// watchable, and an s3 copy is just as watchable as a minio one.
	ep, err := p.presence.GetByShikimoriEpisode(ctx, d.MALID, d.Episode, "")
	if err == nil && ep != nil {
		if delErr := p.demand.Delete(ctx, d.MALID, d.Episode); delErr != nil && p.log != nil {
			p.log.Warnw("autocache planner: delete present demand failed", "mal", d.MALID, "ep", d.Episode, "error", delErr)
		}
		// WR-03: the (mal,ep) is satisfied and no longer wanted — drop its backoff
		// entry immediately so the lastSearched map doesn't retain a dead key until
		// the next gcBackoff sweep.
		p.forgetSearched(demandKey(d.MALID, d.Episode))
		p.metrics.IncDownloadsTotal(trigger, "present")
		return false
	}

	// 2. In-flight single-flight (TRIG-04): a non-terminal job already targets
	// (mal,ep) → enqueue no second job; LEAVE the demand row so the next tick
	// re-derives once the job goes terminal (Pitfall 6 option b).
	active, err := p.jobs.HasActiveForEpisode(ctx, d.MALID, d.Episode)
	if err != nil {
		if p.log != nil {
			p.log.Warnw("autocache planner: in-flight check failed", "mal", d.MALID, "ep", d.Episode, "error", err)
		}
		p.metrics.IncDownloadsTotal(trigger, "error")
		return false
	}
	if active {
		p.metrics.IncDownloadsTotal(trigger, "dedup")
		return false
	}

	// 3. Backoff: skip re-searching a recently-searched no-release row (Pitfall 4)
	// and respect the per-sweep fan-out cap.
	k := demandKey(d.MALID, d.Episode)
	if !maySearch || p.inBackoff(k) {
		return false
	}

	// 4. Search + RAW/quality/seeder filter (TRIG-05). The library has no anime
	// titles of its own, so it tries each producer-supplied title in fallback
	// order (name_jp → romaji → name_en); the first title that yields a qualifying
	// RAW wins. MALID is passed on every query so AnimeTosho's MAL-keyed search
	// contributes regardless of the keyword. A legacy/title-less row falls back to
	// the bare mal_id keyword (the old, near-useless behavior — better than not
	// searching).
	matchTitles := d.SearchTitles()
	malID := malIDInt(d.MALID)
	queries := matchTitles
	if len(queries) == 0 {
		queries = []string{d.MALID}
	}
	var (
		rel      domain.Release
		ok       bool
		sawError bool
	)
	for _, title := range queries {
		res, err := p.search.FetchAll(ctx, SearchQuery{
			Query: searchQueryFor(title, d.Episode),
			MALID: malID,
			Limit: 50,
		})
		if err != nil {
			sawError = true
			if p.log != nil {
				p.log.Warnw("autocache planner: search failed", "mal", d.MALID, "ep", d.Episode, "title", title, "error", err)
			}
			continue
		}
		// Episode-exact + anime-identity (MAL-ID, else title-match) guard the pick
		// against false matches (e.g. a popular unrelated keyword hit). matchTitles
		// is the FULL ordered title set, independent of which title drove this query.
		if rel, ok = selectRAW(res.Releases, cfg.QualityCap, cfg.MinSeeders, d.Episode, malID, matchTitles); ok {
			break
		}
	}
	if !ok {
		// No qualifying release across all titles — LEAVE the demand row (retry next
		// tick, "as soon as on torrents") and arm the backoff so it isn't re-searched
		// every tick. Distinguish a transport error from a genuine no-result for the
		// OBS-04 metric.
		result := "no_release"
		if sawError {
			result = "error"
		}
		p.metrics.IncDownloadsTotal(trigger, result)
		p.markSearched(k)
		return true
	}

	// 5. Pre-admit budget gate (EVICT-04/05): before enqueueing, the SAME
	// Evictor.EnsureRoom the admin upload path gates must admit the incoming
	// download under the logical pool budget (layered ON TOP of the physical
	// DiskGuard). estBytes = the selected release size, falling back to the
	// avgRawEpSize const when the release reports no size. nil-guarded so a
	// Planner with no evictor skips the gate.
	if p.evictor != nil {
		estBytes := rel.SizeBytes
		if estBytes <= 0 {
			estBytes = AvgRawEpSize
		}
		admitted, err := p.evictor.EnsureRoom(ctx, estBytes)
		if err != nil {
			// Fail-open: a budget-read blip must not silently lose a download
			// (mirror the handler/disk-guard fail-open). Proceed to enqueue.
			if p.log != nil {
				p.log.Warnw("autocache planner: budget check failed, admitting (fail-open)",
					"mal", d.MALID, "ep", d.Episode, "error", err)
			}
		} else if !admitted {
			// Pool can't fit even after draining the Stale queue (EVICT-04).
			// LEAVE the demand row (do NOT Delete) so it retries once room frees,
			// arm the existing per-(mal,ep) searchBackoff so it isn't re-searched/
			// re-rejected every tick (T-10-08 hot-loop guard), and record the
			// OBS-04 "rejected" result.
			p.metrics.IncRejectedTotal("budget_full")
			p.metrics.IncDownloadsTotal(trigger, "rejected")
			p.markSearched(k)
			if p.log != nil {
				p.log.Warnw("autocache planner: enqueue rejected — budget full",
					"mal", d.MALID, "ep", d.Episode, "est_bytes", estBytes, "trigger", trigger)
			}
			return true
		}
	}

	// 6. Enqueue a source=autocache job carrying the INTENDED episode (the
	// single-flight key for the next tick's HasActiveForEpisode dedup). LEAVE the
	// demand row — it clears once the episode is confirmed present.
	episode := d.Episode
	job := &domain.Job{
		Source:      domain.JobSourceAutocache,
		Magnet:      rel.Magnet,
		Title:       rel.Title,
		Uploader:    rel.Uploader,
		Quality:     rel.Quality,
		SizeBytes:   rel.SizeBytes,
		ShikimoriID: d.MALID,
		Episode:     &episode,
		Status:      domain.JobStatusQueued,
	}
	if err := p.jobs.Create(ctx, job); err != nil {
		if p.log != nil {
			p.log.Errorw("autocache planner: enqueue failed", "mal", d.MALID, "ep", d.Episode, "error", err)
		}
		p.metrics.IncDownloadsTotal(trigger, "error")
		p.markSearched(k)
		return true
	}
	if p.log != nil {
		p.log.Infow("autocache planner: enqueued RAW job",
			"mal", d.MALID, "ep", d.Episode, "title", rel.Title, "quality", rel.Quality, "seeders", rel.Seeders, "trigger", trigger)
	}
	p.metrics.IncDownloadsTotal(trigger, "enqueued")
	p.markSearched(k)
	return true
}

// triggerForReason maps a demand reason to the OBS-04 trigger label. ongoing
// (Logic A) → "A", next_ep (Logic B) → "B", backfill → "backfill". Any unknown
// reason falls back to "backfill" (the safe default, matching the Phase-08
// validate-and-honor handler).
func triggerForReason(r domain.DemandReason) string {
	switch r {
	case domain.DemandReasonOngoing:
		return "A"
	case domain.DemandReasonNextEp:
		return "B"
	default:
		return "backfill"
	}
}

// searchQueryFor builds the best-effort search query from the demand. The library
// DB has no title for a bare mal_id, so the Planner passes the mal_id + episode
// as both the keyword query and (via SearchQuery.MALID) the AnimeTosho MAL-feed
// key. Targeting the specific episode keeps the top hit a single episode rather
// than a season pack (RESEARCH Pitfall 2).
// searchQueryFor builds the tracker keyword query for one candidate title and
// episode, e.g. ("Youkoso Jitsuryoku...", 12) → "Youkoso Jitsuryoku... 12". The
// title-less fallback passes the mal_id as the "title" (the old "59708 12" form).
func searchQueryFor(title string, episode int) string {
	return title + " " + strconv.Itoa(episode)
}

// malIDInt converts the string mal_id to the int SearchQuery.MALID expects.
// Returns 0 on a non-numeric id (the searcher then relies on the keyword query).
func malIDInt(malID string) int {
	n, err := strconv.Atoi(malID)
	if err != nil {
		return 0
	}
	return n
}

func demandKey(malID string, episode int) string {
	return malID + ":" + strconv.Itoa(episode)
}

// inBackoff reports whether (mal:ep) was searched within searchBackoff.
func (p *Planner) inBackoff(k string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	last, ok := p.lastSearched[k]
	return ok && time.Since(last) < searchBackoff
}

// markSearched records the last-searched time for the (mal:ep) backoff.
func (p *Planner) markSearched(k string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSearched[k] = time.Now()
}

// forgetSearched drops a single (mal:ep) backoff entry. Called when the demand
// is satisfied (present-delete) so the lastSearched map sheds dead keys eagerly
// rather than waiting for the periodic gcBackoff sweep (WR-03).
func (p *Planner) forgetSearched(k string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.lastSearched, k)
}

// gcBackoff evicts every lastSearched entry older than searchBackoff. Such an
// entry can no longer suppress a re-search (inBackoff already treats it as
// expired), so removing it is behavior-preserving — it only bounds map growth
// (WR-03). Called once per sweep before processing rows.
func (p *Planner) gcBackoff() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, t := range p.lastSearched {
		if time.Since(t) >= searchBackoff {
			delete(p.lastSearched, k)
		}
	}
}

// sleep is a ctx-aware + stop-aware sleep. Returns false when the loop must
// exit (ctx cancelled or Stop signalled).
func (p *Planner) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-p.stop:
		return false
	case <-t.C:
		return true
	}
}
