// Package jobs — scraper_playability_canary.go is the v3.1 Phase 23 daily
// canary that exercises real production scraper code paths
// (HTTP /scraper/servers + /scraper/stream + libs/streamprobe.Probe)
// against 5 anime — 2 stable anchors (Frieren, One Piece) + 3 dynamic
// from the last 24h of watch_history — and emits the
// playability_canary_runs_total{provider, server, result, reason,
// anime_slot} counter so Prometheus + Grafana can detect upstream-site
// regressions before users do.
//
// Per-run JSON logs persist to the player_reports Docker volume so the
// downstream maintenance bot has artifacts to `cat` instead of having to
// re-fetch upstream content (which may have rotated by the time it runs).
//
// SCRAPER-HEAL-12 + SCRAPER-HEAL-13.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
	"gorm.io/gorm"
)

const (
	// AnchorFrierenMAL — MAL ID for Frieren: Beyond Journey's End.
	// Verify against the live `animes` table on the first production run;
	// adjust if the seed data differs.
	AnchorFrierenMAL = 52991
	// AnchorOnePieceMAL — MAL ID for One Piece.
	AnchorOnePieceMAL = 21

	// scraperReqTimeout is the per-HTTP-request budget for /scraper/servers
	// and /scraper/stream calls. Generous enough to absorb streamprobe's
	// 10s budget when the scraper service runs the gate internally.
	scraperReqTimeout = 12 * time.Second
	// canaryRunBudget is the hard cap on a single end-to-end canary run.
	// 5 anime × 3 servers × ~15s/probe ≈ 225s worst case; 5min is comfortable.
	canaryRunBudget = 5 * time.Minute
	// canaryProviderLabel — currently only gogoanime is wired upstream.
	// Future providers add new label values, not new label dimensions.
	canaryProviderLabel = "gogoanime"
	// sentinelServerUnreachable — used as the `server` label when
	// /scraper/servers itself fails so the alert rule layer has a
	// deterministic non-empty value to match on. T-23-04 mitigation:
	// bounded sentinel, not a raw URL or error string.
	sentinelServerUnreachable = "_unreachable"
)

// canaryAnime is the per-slot input the canary iterates over. The Slot
// field carries the literal anime_slot label value, MALID the upstream ID
// to query, and Title is a human-readable name persisted in the per-run
// log (empty when the anime isn't in our local `animes` table — the
// anchors fall back to the MAL ID then).
type canaryAnime struct {
	Slot  string
	MALID int
	Title string
}

// TupleResult is a single (provider, server, anime_slot) probe outcome.
// Persisted to the per-run JSON log on disk + emitted as a metric tuple.
type TupleResult struct {
	Provider  string   `json:"provider"`
	Server    string   `json:"server"`
	Result    string   `json:"result"`     // "pass" | "fail"
	Reason    string   `json:"reason"`     // libs/streamprobe.Reason string value
	AnimeSlot string   `json:"anime_slot"` // metrics.AnimeSlots() value
	AnimeID   int      `json:"anime_id"`   // MAL id
	Title     string   `json:"title,omitempty"`
	MasterURL string   `json:"master_url"`
	Sampled   []string `json:"sampled,omitempty"`
}

// RunSummary is the per-run JSON log shape persisted to CanaryReportDir.
// Includes start/end timestamps + applied jitter for diagnostic
// reproducibility — the maintenance bot reads these to triage Pattern 6/7
// alerts (see .claude/maintenance-prompt.md).
type RunSummary struct {
	RunStartedAt time.Time     `json:"run_started_at"`
	RunEndedAt   time.Time     `json:"run_ended_at"`
	Jitter       time.Duration `json:"jitter"`
	Results      []TupleResult `json:"results"`
}

// scraperServer is the subset of the /scraper/servers response shape the
// canary needs. The upstream handler returns far more (URLs, kinds, etc.)
// — the canary only forwards `id` as the `server` query param to
// /scraper/stream.
type scraperServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// serversEnvelope is the /scraper/servers wire shape: {success, data:{servers,meta:{tried,gated?}}}.
type serversEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Servers []scraperServer `json:"servers"`
	} `json:"data"`
}

// streamEnvelope is the /scraper/stream wire shape: {success, data:{stream:{url,headers,...},meta:{tried,gated?}}}.
type streamEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Stream struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers,omitempty"`
		} `json:"stream"`
	} `json:"data"`
}

// ScraperPlayabilityCanaryJob runs daily at 03:00 (±5min jitter) and emits
// playability_canary_runs_total per (slot, server) tuple per run.
//
// All side-effect collaborators (HTTP client, probe function, clock, RNG,
// file writer) are injectable so the unit tests can swap them out without
// touching production code paths.
type ScraperPlayabilityCanaryJob struct {
	db     *gorm.DB
	config *config.JobsConfig
	client *http.Client
	log    *logger.Logger

	// probe is the playability probe used per (anime, server) pair. Default
	// is libs/streamprobe.Probe; tests inject a stub so they don't need to
	// host real m3u8 fixtures.
	probe func(ctx context.Context, masterURL string, hdrs http.Header) streamprobe.Result

	// rng seeds computeJitter. Production uses a time-seeded source so each
	// scheduler boot draws a different jitter pattern; tests inject a fixed
	// seed for reproducibility.
	rng *rand.Rand
	// now is the clock. Production = time.Now; tests freeze it.
	now func() time.Time
	// writeFile is the on-disk writer for the per-run log. Production =
	// os.WriteFile; tests can fake it (but the current suite uses t.TempDir
	// so the real os.WriteFile is exercised).
	writeFile func(name string, data []byte, perm os.FileMode) error
	// skipJitter is a test-only flag that short-circuits the sleep at the
	// head of Run so tests don't have to wait for the jitter duration.
	skipJitter bool
}

// NewScraperPlayabilityCanaryJob constructs the canary with production
// defaults: real streamprobe.Probe, time-seeded RNG, time.Now clock,
// os.WriteFile log writer, and an HTTP client matching the scraperReqTimeout
// budget.
func NewScraperPlayabilityCanaryJob(db *gorm.DB, cfg *config.JobsConfig, log *logger.Logger) *ScraperPlayabilityCanaryJob {
	return &ScraperPlayabilityCanaryJob{
		db:        db,
		config:    cfg,
		client:    &http.Client{Timeout: scraperReqTimeout},
		log:       log,
		probe:     streamprobe.Probe,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		now:       time.Now,
		writeFile: os.WriteFile,
	}
}

// Run is the cron entry point. Applies jitter, composes the anime list,
// probes every (slot, server) pair, emits metrics, persists the per-run
// log. Returns nil on success; only returns an error when the run cannot
// even start (e.g. ctx already canceled). Individual probe failures are
// recorded as metric tuples, not propagated.
//
// Always applies the configured jitter on scheduled runs. Use RunNoJitter
// for the manual-trigger HTTP path (synthetic test + ops smoke) so admins
// don't wait up to 5 minutes for a result.
func (j *ScraperPlayabilityCanaryJob) Run(ctx context.Context) error {
	return j.run(ctx, !j.skipJitter)
}

// RunNoJitter is the manual-trigger entry point: same body as Run but
// skips the ±5min sleep. Cron schedule still uses Run.
func (j *ScraperPlayabilityCanaryJob) RunNoJitter(ctx context.Context) error {
	return j.run(ctx, false)
}

func (j *ScraperPlayabilityCanaryJob) run(ctx context.Context, applyJitter bool) error {
	jitter := time.Duration(0)
	if applyJitter {
		jitter = j.computeJitter()
		if jitter > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jitter):
			}
		}
	}

	runStart := j.now()

	ctx, cancel := context.WithTimeout(ctx, canaryRunBudget)
	defer cancel()

	list, err := j.composeAnimeList(ctx)
	if err != nil {
		j.log.Errorw("canary anime list composition failed", "error", err)
		return err
	}

	results := make([]TupleResult, 0, len(list)*3)
	for _, a := range list {
		tuples := j.probeOne(ctx, a)
		for _, t := range tuples {
			// Emit metric per tuple.
			metrics.PlayabilityCanaryRunsTotal.WithLabelValues(
				t.Provider, t.Server, t.Result, t.Reason, t.AnimeSlot,
			).Inc()
		}
		results = append(results, tuples...)
	}

	runEnd := j.now()
	summary := RunSummary{
		RunStartedAt: runStart,
		RunEndedAt:   runEnd,
		Jitter:       jitter,
		Results:      results,
	}
	if err := j.writeRunLog(summary); err != nil {
		// Log-write failure does not fail the run — metrics already emitted.
		j.log.Errorw("canary per-run log write failed", "error", err)
	}

	j.log.Infow("scraper playability canary run complete",
		"anime_count", len(list),
		"tuple_count", len(results),
		"duration", runEnd.Sub(runStart),
	)
	return nil
}

// composeAnimeList builds the 2..5 element anime list per CONTEXT.md D2:
//
//  1. anchors (always 2): Frieren + One Piece. Title resolved from the
//     local `animes` table when present; falls back to empty.
//  2. recent 3 (dynamic): top 3 distinct anime_ids from watch_history
//     where watched_at > NOW() - 24h, ordered by watched_at DESC.
//  3. fallback when watch_history empty: top 3 from `animes` ORDER BY
//     updated_at DESC (with deleted_at NULL).
//  4. when both queries return zero rows: log a warning and return only
//     the 2 anchors. Don't fail the run.
//
// The sqlite-flavored queries here use a portable subquery pattern that
// works on both sqlite (tests) and Postgres (production). DISTINCT ON
// would be more elegant on Postgres but would force a dialect split.
func (j *ScraperPlayabilityCanaryJob) composeAnimeList(ctx context.Context) ([]canaryAnime, error) {
	list := make([]canaryAnime, 0, 5)

	// Anchors first. Title comes from a single-row lookup against `animes`.
	frieren := canaryAnime{Slot: "anchor_frieren", MALID: AnchorFrierenMAL, Title: j.lookupTitle(ctx, AnchorFrierenMAL)}
	onePiece := canaryAnime{Slot: "anchor_one_piece", MALID: AnchorOnePieceMAL, Title: j.lookupTitle(ctx, AnchorOnePieceMAL)}
	list = append(list, frieren, onePiece)

	recents := j.recentFromWatchHistory(ctx)
	if len(recents) == 0 {
		// Fallback: top 3 from animes ORDER BY updated_at DESC.
		recents = j.recentFromAnimeList(ctx)
	}
	if len(recents) == 0 {
		j.log.Warnw("canary anime list incomplete: no recent watch history, no animes — running anchors only")
		return list, nil
	}
	slots := []string{"recent_1", "recent_2", "recent_3"}
	for i, r := range recents {
		if i >= 3 {
			break
		}
		r.Slot = slots[i]
		list = append(list, r)
	}
	return list, nil
}

// lookupTitle returns the russian title for the given MAL ID, or empty
// string when not present. Best-effort — errors are swallowed because the
// title is purely for the JSON log's human readability.
//
// The production `animes` schema stores mal_id as varchar (string), so we
// pass the int formatted as a decimal string. The `name_ru` column is the
// Russian title (NOT `russian` — schema drift caught during smoke).
func (j *ScraperPlayabilityCanaryJob) lookupTitle(ctx context.Context, malID int) string {
	if j.db == nil {
		return ""
	}
	var title string
	row := j.db.WithContext(ctx).Raw(
		`SELECT name_ru FROM animes WHERE mal_id = ? LIMIT 1`, fmt.Sprintf("%d", malID),
	).Row()
	if err := row.Scan(&title); err != nil {
		return ""
	}
	return title
}

// recentFromWatchHistory returns the 3 most-recently-watched distinct
// anime from the last 24h. Joins watch_history with animes for MAL id +
// title. Returns nil when the DB is empty or unavailable.
//
// Schema note: production `animes` stores mal_id as varchar(50), not int —
// scanned into a string and parsed via strconv.Atoi. Rows with a missing or
// unparseable mal_id are skipped (the canary needs a numeric ID for the
// upstream /scraper/* calls). Column `name_ru` (NOT `russian`) is the
// Russian title.
func (j *ScraperPlayabilityCanaryJob) recentFromWatchHistory(ctx context.Context) []canaryAnime {
	if j.db == nil {
		return nil
	}
	// Portable query: select most-recent watched_at per anime_id from the
	// last 24h, then join + order. Works on both sqlite (tests) and
	// Postgres (prod).
	rows, err := j.db.WithContext(ctx).Raw(`
		SELECT a.mal_id, a.name_ru, last_watched
		FROM (
			SELECT anime_id, MAX(watched_at) AS last_watched
			FROM watch_history
			WHERE watched_at > ?
			GROUP BY anime_id
		) recent
		JOIN animes a ON a.id = recent.anime_id
		ORDER BY last_watched DESC
		LIMIT 3
	`, j.now().Add(-24*time.Hour)).Rows()
	if err != nil {
		j.log.Warnw("canary recentFromWatchHistory query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var out []canaryAnime
	for rows.Next() {
		var (
			malIDStr sql.NullString
			nameRu   sql.NullString
			// last_watched is scanned as interface{} because sqlite returns
			// MAX(DATETIME) as string (driver.Value type) while Postgres returns
			// it as time.Time. The canary only uses this column for ORDER BY
			// in SQL — never for arithmetic in Go — so the loose type is safe.
			lastWatched interface{}
		)
		if err := rows.Scan(&malIDStr, &nameRu, &lastWatched); err != nil {
			j.log.Warnw("canary recentFromWatchHistory row scan failed", "error", err)
			continue
		}
		malID := parseMALID(malIDStr.String)
		if malID == 0 {
			continue
		}
		out = append(out, canaryAnime{MALID: malID, Title: nameRu.String})
	}
	return out
}

// recentFromAnimeList is the fallback when watch_history is empty: top 3
// animes by updated_at DESC. The spec uses the "anime_list" identifier
// loosely — in practice the data lives in the `animes` table (one row per
// anime, not per user-watchlist-entry).
//
// Schema note (production): mal_id is varchar(50), name_ru is the Russian
// title. Rows with NULL / unparseable mal_id are skipped — we need a
// numeric ID for upstream /scraper/* calls. Filter to anime that actually
// have a numeric mal_id at the SQL layer so we don't read a giant catalog
// row just to throw it away.
func (j *ScraperPlayabilityCanaryJob) recentFromAnimeList(ctx context.Context) []canaryAnime {
	if j.db == nil {
		return nil
	}
	rows, err := j.db.WithContext(ctx).Raw(`
		SELECT mal_id, name_ru
		FROM animes
		WHERE deleted_at IS NULL AND mal_id IS NOT NULL AND mal_id != ''
		ORDER BY updated_at DESC
		LIMIT 3
	`).Rows()
	if err != nil {
		j.log.Warnw("canary recentFromAnimeList query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var out []canaryAnime
	for rows.Next() {
		var (
			malIDStr sql.NullString
			nameRu   sql.NullString
		)
		if err := rows.Scan(&malIDStr, &nameRu); err != nil {
			continue
		}
		malID := parseMALID(malIDStr.String)
		if malID == 0 {
			continue
		}
		out = append(out, canaryAnime{MALID: malID, Title: nameRu.String})
	}
	return out
}

// parseMALID converts the production schema's varchar(50) mal_id into the
// int the canary uses for upstream /scraper/* calls. Returns 0 for any
// unparseable or empty input — the caller skips zero values.
func parseMALID(s string) int {
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

// probeOne iterates every server returned by /scraper/servers for the
// anime, calls /scraper/stream for each, runs the probe against the
// returned URL, and returns one TupleResult per server. When
// /scraper/servers itself fails, emits a single sentinel
// (server="_unreachable", reason="cdn_unreachable") tuple so the alert
// rule layer has something to match.
func (j *ScraperPlayabilityCanaryJob) probeOne(ctx context.Context, a canaryAnime) []TupleResult {
	servers, err := j.fetchServers(ctx, a.MALID)
	if err != nil {
		j.log.Warnw("canary /scraper/servers failed", "anime_slot", a.Slot, "mal_id", a.MALID, "error", err)
		return []TupleResult{{
			Provider:  canaryProviderLabel,
			Server:    sentinelServerUnreachable,
			Result:    "fail",
			Reason:    string(streamprobe.ReasonCDNUnreachable),
			AnimeSlot: a.Slot,
			AnimeID:   a.MALID,
			Title:     a.Title,
		}}
	}
	if len(servers) == 0 {
		// Empty server list is itself a regression signal.
		return []TupleResult{{
			Provider:  canaryProviderLabel,
			Server:    sentinelServerUnreachable,
			Result:    "fail",
			Reason:    string(streamprobe.ReasonZeroMatch),
			AnimeSlot: a.Slot,
			AnimeID:   a.MALID,
			Title:     a.Title,
		}}
	}

	out := make([]TupleResult, 0, len(servers))
	for _, s := range servers {
		t := j.probeServer(ctx, a, s)
		out = append(out, t)
	}
	return out
}

// probeServer runs the full /scraper/stream → streamprobe.Probe chain for a
// single (anime, server) pair and returns a populated TupleResult.
func (j *ScraperPlayabilityCanaryJob) probeServer(ctx context.Context, a canaryAnime, s scraperServer) TupleResult {
	t := TupleResult{
		Provider:  canaryProviderLabel,
		Server:    s.ID,
		AnimeSlot: a.Slot,
		AnimeID:   a.MALID,
		Title:     a.Title,
	}
	streamURL, hdrs, err := j.fetchStream(ctx, a.MALID, s.ID)
	if err != nil {
		t.Result = "fail"
		t.Reason = string(streamprobe.ReasonCDNUnreachable)
		return t
	}
	t.MasterURL = streamURL
	if streamURL == "" {
		t.Result = "fail"
		t.Reason = string(streamprobe.ReasonEmptyResponse)
		return t
	}

	res := j.probe(ctx, streamURL, hdrsToHeader(hdrs))
	t.Reason = string(res.Reason)
	t.Sampled = res.Sampled
	if res.Playable {
		t.Result = "pass"
	} else {
		t.Result = "fail"
	}
	return t
}

// fetchServers calls /scraper/servers?animeId=<mal>&episode=1 and returns
// the parsed server list. Non-200 + decode errors surface as an error so
// the caller can record the sentinel.
func (j *ScraperPlayabilityCanaryJob) fetchServers(ctx context.Context, malID int) ([]scraperServer, error) {
	u := fmt.Sprintf("%s/scraper/servers?mal_id=%d&episode=1", strings.TrimRight(j.config.ScraperBaseURL, "/"), malID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build /scraper/servers request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("/scraper/servers: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/scraper/servers returned %d", resp.StatusCode)
	}
	var env serversEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode /scraper/servers: %w", err)
	}
	return env.Data.Servers, nil
}

// fetchStream calls /scraper/stream?mal_id=<mal>&episode=1&server=<id> and
// returns (url, headers, nil) on success.
func (j *ScraperPlayabilityCanaryJob) fetchStream(ctx context.Context, malID int, serverID string) (string, map[string]string, error) {
	u := fmt.Sprintf("%s/scraper/stream?mal_id=%d&episode=1&server=%s",
		strings.TrimRight(j.config.ScraperBaseURL, "/"), malID, serverID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, fmt.Errorf("build /scraper/stream request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("/scraper/stream: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("/scraper/stream returned %d", resp.StatusCode)
	}
	var env streamEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return "", nil, fmt.Errorf("decode /scraper/stream: %w", err)
	}
	return env.Data.Stream.URL, env.Data.Stream.Headers, nil
}

// hdrsToHeader converts the map shape the scraper returns into an
// http.Header for streamprobe.Probe. Drops the redacted keys so we never
// re-forward Authorization / Cookie material upstream.
func hdrsToHeader(m map[string]string) http.Header {
	if len(m) == 0 {
		return nil
	}
	h := make(http.Header, len(m))
	for k, v := range m {
		if isSensitiveHeader(k) {
			continue
		}
		h.Set(k, v)
	}
	return h
}

// writeRunLog persists the per-run summary to CanaryReportDir as a single
// JSON file named YYYY-MM-DD-HHMMSS.json (UTC-flavored timestamp). Mkdir
// is idempotent. Sensitive headers are already absent from TupleResult so
// we don't need to scrub the JSON again here.
//
// T-23-01 mitigation: header redaction happens before the response is
// converted into our internal TupleResult shape (no upstream response
// headers are ever stored in RunSummary).
func (j *ScraperPlayabilityCanaryJob) writeRunLog(s RunSummary) error {
	if err := os.MkdirAll(j.config.CanaryReportDir, 0o755); err != nil {
		return fmt.Errorf("mkdir CanaryReportDir: %w", err)
	}
	name := s.RunStartedAt.UTC().Format("2006-01-02-150405") + ".json"
	path := j.config.CanaryReportDir + string(os.PathSeparator) + name
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal RunSummary: %w", err)
	}
	if j.writeFile == nil {
		j.writeFile = os.WriteFile
	}
	return j.writeFile(path, data, 0o644)
}

// computeJitter samples a duration uniformly from [-5min, +5min].
// Bounded by `j.rng.Int63n(601)` (0..600 inclusive) shifted by -300 so
// the sleep at the head of Run never exceeds the documented bound and a
// trivial change to the cron tick can't produce a runaway jitter.
//
// T-23-02 mitigation: ±5min jitter prevents 03:00:00 fingerprinting that
// could trip upstream-site rate-limit retaliation.
func (j *ScraperPlayabilityCanaryJob) computeJitter() time.Duration {
	if j.rng == nil {
		// Defensive: never seen in production because NewScraperPlayabilityCanaryJob
		// always seeds rng; but tests sometimes build the struct by hand.
		j.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	secs := j.rng.Int63n(601) - 300 // 0..600 → -300..+300
	return time.Duration(secs) * time.Second
}

// isSensitiveHeader returns true for header names that may carry credentials
// or session tokens — they MUST be stripped before any persistence or
// forwarding step. Case-insensitive match.
//
// T-23-01 + ASVS V8 (authentication / session) hygiene.
func isSensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "cookie", "set-cookie", "proxy-authorization":
		return true
	}
	return false
}
