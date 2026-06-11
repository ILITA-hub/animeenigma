// Package handler — admin_recs.go: GET /api/admin/recs/{user_id} and
// POST /api/admin/recs/{user_id}/recompute.
//
// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02). The admin debug endpoint surfaces
// the full per-signal breakdown for the target user — every Raw / Normalized
// / Weighted contribution per anime, the top-contributor signal_id, the S5
// TF-IDF term breakdown on s5-dominated rows, the S6 cascade source +
// pin_seed_anime_id on the pinned row, and a sibling filtered_out array of
// {anime_id, reason} entries from S11.FilterAudit. The force-recompute
// endpoint invalidates the per-user topN cache, runs the synchronous user
// precompute orchestrator, and returns the latency.
//
// Both endpoints are mounted under the admin-role-gated /api/admin/recs/*
// group — see services/player/internal/transport/router.go and
// services/player/internal/transport/admin.go.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs/signals"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// uuidRe matches the canonical UUID v4 form (8-4-4-4-12 hex with hyphens).
// We use it to decide whether the {user_id} URL param is already a UUID, or
// whether it needs to be resolved from username / public_id.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// resolveUserID accepts a string from the URL param and returns the canonical
// UUID. The param may be any of: (a) a UUID v4 — returned as-is, (b) a
// username, (c) a public_id. The lookup is a single SELECT against the users
// table the player service shares with auth.
//
// Behavior:
//   - Returns the raw input + nil if it already matches the UUID v4 format.
//   - Returns the resolved UUID + nil on a successful username/public_id lookup.
//   - Returns "" + nil if the lookup completes cleanly but the user does
//     not exist (caller treats as 404).
//   - Returns the raw input + nil if the users table is unreachable
//     (e.g. test fixtures that only seed recs tables) — graceful fall-through
//     so downstream queries error naturally rather than masking the real
//     issue with a confusing 404.
//   - Returns "" + err on any other DB error (caller surfaces as 500).
func (h *AdminRecsHandler) resolveUserID(ctx context.Context, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if uuidRe.MatchString(raw) {
		return raw, nil
	}
	var id string
	err := h.db.WithContext(ctx).
		Table("users").
		Select("id").
		Where("username = ? OR public_id = ?", raw, raw).
		Limit(1).
		Scan(&id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		// Test fixtures often only create the recs tables; "no such table"
		// (sqlite) and "relation ... does not exist" (postgres) both indicate
		// the users table is absent — fall through to the raw input rather
		// than 500. Production has the users table guaranteed.
		msg := err.Error()
		if regexpUsersTableMissing.MatchString(msg) {
			h.log.Debugw("admin recs: users table missing, falling through with raw param",
				"raw", raw, "error", msg)
			return raw, nil
		}
		return "", err
	}
	return id, nil
}

// regexpUsersTableMissing matches "no such table: users" (sqlite) and
// "relation \"users\" does not exist" (postgres). Used by resolveUserID to
// gracefully fall through when the users table is absent in tests.
var regexpUsersTableMissing = regexp.MustCompile(`no such table:?\s*users|relation\s+"?users"?\s+does\s+not\s+exist`)

// adminTopNSliceSize — the admin debug page shows the top 50 ranked candidates,
// matching the user-facing serveLoggedIn slice ceiling so admins see exactly
// what the public API would return (plus the pin if S6 fires).
const adminTopNSliceSize = 50

// AdminRecsResponse is the JSON payload for GET /api/admin/recs/{user_id}.
// Schema locked per CONTEXT.md §C1.
type AdminRecsResponse struct {
	Recs           []AdminRecRow            `json:"recs"`
	FilteredOut    []signals.FilteredOutEntry `json:"filtered_out"`
	ComputedAt     string                   `json:"computed_at"`
	SignalVersions map[string]string        `json:"signal_versions"`
	UserID         string                   `json:"user_id"`
}

// AdminRecRow is one row in the admin debug table. Surfaces every per-signal
// raw / normalized / weighted contribution + the top contributor signal_id.
// For pinned rows: PinSource / PinSeedAnimeID carry the S6 cascade metadata
// and TopContributor is the literal "s6_pin" (locked Phase 13 hand-off).
type AdminRecRow struct {
	Rank              int                    `json:"rank"`
	Anime             RecAnimePayload        `json:"anime"`
	Final             float64                `json:"final"`
	Breakdown         map[string]float64     `json:"breakdown"`
	Weights           map[string]float64     `json:"weights"`
	TopContributor    string                 `json:"top_contributor"`
	ContributorDetail map[string]interface{} `json:"contributor_detail,omitempty"`
	Pinned            bool                   `json:"pinned,omitempty"`
	PinReason         string                 `json:"pin_reason,omitempty"`           // legacy English fallback
	PinReasonKey      string                 `json:"pin_reason_key,omitempty"`       // UX-09 (Phase 3) — i18n key
	PinReasonData     map[string]interface{} `json:"pin_reason_data,omitempty"`      // UX-09 (Phase 3) — interpolation values for PinReasonKey
	PinSource         string                 `json:"pin_source,omitempty"`
	PinSeedAnimeID    string                 `json:"pin_seed_anime_id,omitempty"`
}

// ForceRecomputeResponse is the JSON payload for POST .../recompute.
type ForceRecomputeResponse struct {
	ComputedAt string `json:"computed_at"`
	TopNCount  int    `json:"top_n_count"`
	LatencyMs  int64  `json:"latency_ms"`
}

// adminRecsCache is the narrow cache surface AdminRecsHandler needs:
// the public /api/users/recs handler uses recsCache (Get + Set);
// the admin handler additionally needs Delete to bust the per-user topN
// cache before a force-recompute.
type adminRecsCache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// AdminRecsHandler serves the admin debug + force-recompute endpoints.
type AdminRecsHandler struct {
	db         *gorm.DB
	recsRepo   *repo.RecsRepository
	cache      adminRecsCache
	log        *logger.Logger
	precompute *recs.Orchestrator

	// Composed signals — built once at construction; same modules the public
	// handler uses. Built inline so the admin handler doesn't depend on the
	// public RecsHandler's signal registry (decoupled).
	s11 *signals.S11Filter
	s1  *signals.S1ScoreCluster
	s2  *signals.S2Metadata
	s3  *signals.S3Trending
	s4  *signals.S4Recency
	s5  *signals.S5Attribute
	s6  *signals.S6ComboPin        // optional; nil-guarded
	s7  *signals.S7DroppedPenalty  // spec 2026-06-11 Phase 3 — demotes dropped-similar
}

// NewAdminRecsHandler wires the admin handler. The s6 module may be nil
// (mirrors NewRecsHandler for tests that don't exercise the pin surface).
// The precompute orchestrator is the caller's responsibility — main.go hoists
// the user-orchestrator's underlying recs.Orchestrator out so admin
// force-recompute calls the SAME synchronous RunForUser path the cron uses.
func NewAdminRecsHandler(
	db *gorm.DB,
	recsRepo *repo.RecsRepository,
	cache adminRecsCache,
	s6 *signals.S6ComboPin,
	precompute *recs.Orchestrator,
	log *logger.Logger,
) *AdminRecsHandler {
	return &AdminRecsHandler{
		db:         db,
		recsRepo:   recsRepo,
		cache:      cache,
		log:        log,
		precompute: precompute,
		s11:        signals.NewS11Filter(db),
		s1:         signals.NewS1ScoreCluster(db, recsRepo),
		s2:         signals.NewS2Metadata(db),
		s3:         signals.NewS3Trending(db, recsRepo),
		s4:         signals.NewS4Recency(db),
		s5:         signals.NewS5Attribute(db, recsRepo),
		s6:         s6,
		s7:         signals.NewS7DroppedPenalty(db), // spec 2026-06-11 Phase 3
	}
}

// adminEnsembleWeights is the weight registry mirrored from computeFreshForUser
// at services/recs/internal/handler/recs.go. Matched here so admin breakdown
// columns reflect the exact weights production uses.
// Phase-12 + S7 dropped-penalty (spec 2026-06-11 Phase 3).
var adminEnsembleWeights = map[recs.SignalID]float64{
	recs.SignalID("s1"): 0.30,
	recs.SignalID("s2"): 0.20,
	recs.SignalID("s3"): 0.20,
	recs.SignalID("s4"): 0.10,
	recs.SignalID("s5"): 0.20,
	recs.SignalID("s7"): -0.15,
}

// adminSignalVersions is the hardcoded signal-version map surfaced in the
// response. Phase 14 uses a flat v1.0 across all signals; future signal
// versioning is a v3.0 concern.
var adminSignalVersions = map[string]string{
	"s1": "v1.0",
	"s2": "v1.0",
	"s3": "v1.0",
	"s4": "v1.0",
	"s5": "v1.0",
	"s6": "v1.0",
	"s7": "v1.0",
}

// GetAdminRecs handles GET /api/admin/recs/{user_id}. Builds the full Phase-12
// ensemble against S11.CandidatePoolForUser, runs RankWithBreakdown, slices
// to top-50, hydrates anime metadata, surfaces the S5 TF-IDF top-3 terms on
// s5-dominated rows, prepends the S6 pin row when the cascade fires, and
// returns the filtered_out audit alongside.
func (h *AdminRecsHandler) GetAdminRecs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rawID := chi.URLParam(r, "user_id")
	if rawID == "" {
		httputil.BadRequest(w, "user_id is required")
		return
	}

	// Resolve username / public_id to UUID so admins can hit
	// /admin/recs/{username} without memorizing UUIDs.
	userID, resolveErr := h.resolveUserID(ctx, rawID)
	if resolveErr != nil {
		h.log.Errorw("admin recs: resolve user_id failed", "raw", rawID, "error", resolveErr)
		httputil.Error(w, resolveErr)
		return
	}
	if userID == "" {
		httputil.NotFound(w, "user not found: "+rawID)
		return
	}

	// 1. Candidate pool (excludes any anime in user's list / hidden / soft-deleted).
	pool, err := h.s11.CandidatePoolForUser(ctx, recs.UserID(userID))
	if err != nil {
		h.log.Errorw("admin recs: candidate pool failed", "user_id", userID, "error", err)
		httputil.Error(w, err)
		return
	}

	// 2. Filter audit (always run regardless of pool size — admins use this
	// panel to investigate "why is the row empty for this user?").
	auditEntries, auditErr := h.s11.FilterAudit(ctx, recs.UserID(userID))
	if auditErr != nil {
		h.log.Warnw("admin recs: filter audit failed (non-fatal)", "user_id", userID, "error", auditErr)
		auditEntries = nil
	}

	// 3. computed_at — pulled from rec_user_signals.last_computed when the row
	// exists; falls back to current time otherwise.
	computedAt := time.Now().UTC().Format(time.RFC3339)
	if sigs, sigErr := h.recsRepo.GetUserSignals(ctx, userID); sigErr == nil && sigs != nil && !sigs.LastComputed.IsZero() {
		computedAt = sigs.LastComputed.UTC().Format(time.RFC3339)
	}

	// Empty pool — return graceful empty response (NOT 404). Admins use this
	// surface to investigate "why is the row empty for this user?".
	if len(pool) == 0 {
		httputil.OK(w, AdminRecsResponse{
			Recs:           []AdminRecRow{},
			FilteredOut:    auditEntries,
			ComputedAt:     computedAt,
			SignalVersions: adminSignalVersions,
			UserID:         userID,
		})
		return
	}

	// 4. Build the full ensemble + RankWithBreakdown.
	// Phase-12 + S7 dropped-penalty (spec 2026-06-11 Phase 3). S7 appended LAST
	// so it cannot steal top_contributor via tie-breaking in the all-zero case.
	ensemble := recs.NewEnsemble([]recs.WeightedSignal{
		{Module: h.s1, Weight: adminEnsembleWeights[recs.SignalID("s1")]},
		{Module: h.s2, Weight: adminEnsembleWeights[recs.SignalID("s2")]},
		{Module: h.s3, Weight: adminEnsembleWeights[recs.SignalID("s3")]},
		{Module: h.s4, Weight: adminEnsembleWeights[recs.SignalID("s4")]},
		{Module: h.s5, Weight: adminEnsembleWeights[recs.SignalID("s5")]},
		{Module: h.s7, Weight: adminEnsembleWeights[recs.SignalID("s7")]},
	})
	ranked, err := ensemble.RankWithBreakdown(ctx, recs.UserID(userID), pool)
	if err != nil {
		h.log.Errorw("admin recs: ensemble RankWithBreakdown failed", "user_id", userID, "error", err)
		httputil.Error(w, err)
		return
	}

	// 5. Stable secondary sort (matches public handler tiebreak chain) so
	// admin debug ordering equals what the user sees.
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Final != ranked[j].Final {
			return ranked[i].Final > ranked[j].Final
		}
		s4i := float64(ranked[i].Breakdown[recs.SignalID("s4")])
		s4j := float64(ranked[j].Breakdown[recs.SignalID("s4")])
		if s4i != s4j {
			return s4i > s4j
		}
		return ranked[i].AnimeID < ranked[j].AnimeID
	})

	// 6. Slice to top-50.
	end := adminTopNSliceSize
	if len(ranked) < end {
		end = len(ranked)
	}
	top := ranked[:end]

	// 7. Hydrate anime metadata (reuse the public handler's helper via a
	// fresh inline implementation — keeps admin handler decoupled).
	ids := make([]string, len(top))
	for i, r := range top {
		ids[i] = string(r.AnimeID)
	}
	hydrated, err := h.hydrateAnime(ctx, ids)
	if err != nil {
		h.log.Errorw("admin recs: hydrate failed", "user_id", userID, "error", err)
		httputil.Error(w, err)
		return
	}

	// 8. Build the response rows. For rows where TopContributor==s5, parse
	// rec_user_signals.s5_affinity JSONB and slice the top-3 attribute terms
	// into contributor_detail.tf_idf_terms for the expand-on-click panel.
	weightsAsFloat64 := make(map[string]float64, len(adminEnsembleWeights))
	for k, v := range adminEnsembleWeights {
		weightsAsFloat64[string(k)] = v
	}

	var s5Terms []map[string]interface{}
	if sigs, sigErr := h.recsRepo.GetUserSignals(ctx, userID); sigErr == nil && sigs != nil && sigs.S5Affinity != "" {
		s5Terms = topS5TFIDFTerms(sigs.S5Affinity, 3)
	}

	rows := make([]AdminRecRow, 0, len(top))
	for i, r := range top {
		anime, ok := hydrated[string(r.AnimeID)]
		if !ok {
			continue
		}
		row := AdminRecRow{
			Rank:           i + 1,
			Anime:          anime,
			Final:          r.Final,
			Breakdown:      breakdownAsFloat64(r.Breakdown),
			Weights:        weightsAsFloat64,
			TopContributor: string(r.TopContributor),
		}
		if r.TopContributor == recs.SignalID("s5") && len(s5Terms) > 0 {
			row.ContributorDetail = map[string]interface{}{
				"tf_idf_terms": s5Terms,
			}
		}
		rows = append(rows, row)
	}

	// 9. S6 pin: if the resolver fires, prepend a Pinned row at rank 1, dedup
	// the underlying anime from the ensemble tail, and re-rank rank 2..N.
	if h.s6 != nil {
		topIDs := make([]string, 0, len(top))
		for _, r := range top {
			topIDs = append(topIDs, string(r.AnimeID))
		}
		pin, pinErr := h.s6.Resolve(ctx, userID, topIDs)
		if pinErr != nil {
			h.log.Warnw("admin recs: s6 resolve failed (non-fatal)", "user_id", userID, "error", pinErr)
		} else if pin != nil {
			pinHydrated, hydrateErr := h.hydrateAnime(ctx, []string{pin.AnimeID})
			if hydrateErr == nil {
				if anime, ok := pinHydrated[pin.AnimeID]; ok {
					// Dedup: drop the pin's anime from the ensemble tail.
					deduped := make([]AdminRecRow, 0, len(rows))
					for _, it := range rows {
						if it.Anime.ID != pin.AnimeID {
							deduped = append(deduped, it)
						}
					}
					for i := range deduped {
						deduped[i].Rank = i + 2
					}
					pinRow := AdminRecRow{
						Rank:           1,
						Anime:          anime,
						Final:          0, // spec §B7: float64-zero approximates null; frontend gates on Pinned
						Breakdown:      map[string]float64{}, // not from ensemble
						Weights:        weightsAsFloat64,
						TopContributor: "s6_pin", // locked Phase-13 hand-off
						Pinned:         true,
						PinReason:      "Because you finished " + pin.SeedName,                  // legacy English fallback
						PinReasonKey:   "recs.pinReason.becauseYouFinished",                    // UX-09 (Phase 3) — i18n key
						PinReasonData:  map[string]interface{}{"name": pin.SeedName},           // UX-09 (Phase 3) — interpolation values
						PinSeedAnimeID: pin.SeedAnimeID,
						PinSource:      pin.Source,
						ContributorDetail: map[string]interface{}{
							"pin_source":         pin.Source,
							"pin_seed_anime_id":  pin.SeedAnimeID,
							"pin_seed_name":      pin.SeedName,
						},
					}
					rows = append([]AdminRecRow{pinRow}, deduped...)
				}
			} else {
				h.log.Warnw("admin recs: s6 pin hydrate failed (non-fatal)",
					"user_id", userID, "pin_anime_id", pin.AnimeID, "error", hydrateErr)
			}
		}
	}

	httputil.OK(w, AdminRecsResponse{
		Recs:           rows,
		FilteredOut:    auditEntries,
		ComputedAt:     computedAt,
		SignalVersions: adminSignalVersions,
		UserID:         userID,
	})
}

// ForceRecompute handles POST /api/admin/recs/{user_id}/recompute.
// 1) Bust recs:user:{user_id}:topN (best-effort, logs on err).
// 2) Run precompute.RunForUser synchronously (NOT TriggerForUser — bypasses
//    the 5-min debounce that would NO-OP repeated admin clicks).
// 3) Return {computed_at, top_n_count, latency_ms}.
//
// top_n_count is the size of the post-recompute candidate pool (via S11),
// NOT a re-ranked top-N — recomputing the full ensemble at the admin path
// would double the latency for marginal info; the size of the candidate pool
// is the metric admins actually care about ("did this user get more
// recommendations?").
func (h *AdminRecsHandler) ForceRecompute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rawID := chi.URLParam(r, "user_id")
	if rawID == "" {
		httputil.BadRequest(w, "user_id is required")
		return
	}

	userID, resolveErr := h.resolveUserID(ctx, rawID)
	if resolveErr != nil {
		h.log.Errorw("admin recs recompute: resolve user_id failed", "raw", rawID, "error", resolveErr)
		httputil.Error(w, resolveErr)
		return
	}
	if userID == "" {
		httputil.NotFound(w, "user not found: "+rawID)
		return
	}

	// 1. Bust the per-user topN cache (fire-and-forget). Uses the shared
	//    UserTopNKey helper so the :v2 suffix stays in lockstep with the
	//    handler / orchestrator read/write paths.
	if err := h.cache.Delete(ctx, recs.UserTopNKey(recs.UserID(userID))); err != nil {
		h.log.Warnw("admin recs: cache delete failed (non-fatal)", "user_id", userID, "error", err)
	}

	// 2. Synchronous precompute.
	start := time.Now()
	if h.precompute != nil {
		if err := h.precompute.RunForUser(ctx, recs.UserID(userID)); err != nil {
			h.log.Errorw("admin recs: precompute failed", "user_id", userID, "error", err)
			httputil.Error(w, err)
			return
		}
	}
	latency := time.Since(start)

	// 3. Re-fetch pool size for top_n_count metric.
	count := 0
	if pool, poolErr := h.s11.CandidatePoolForUser(ctx, recs.UserID(userID)); poolErr == nil {
		count = len(pool)
		if count > adminTopNSliceSize {
			count = adminTopNSliceSize
		}
	}

	httputil.OK(w, ForceRecomputeResponse{
		ComputedAt: time.Now().UTC().Format(time.RFC3339),
		TopNCount:  count,
		LatencyMs:  latency.Milliseconds(),
	})
}

// hydrateAnime fetches anime fields for the admin debug payload. Mirrors
// RecsHandler.hydrateAnime — small enough to inline rather than coupling.
func (h *AdminRecsHandler) hydrateAnime(ctx context.Context, ids []string) (map[string]RecAnimePayload, error) {
	if len(ids) == 0 {
		return map[string]RecAnimePayload{}, nil
	}
	type row struct {
		ID            string
		Name          string
		NameRU        string
		NameJP        string
		PosterURL     string
		Score         float64
		EpisodesCount int
		Status        string
		Year          int
	}
	var rows []row
	err := h.db.WithContext(ctx).
		Table("animes").
		Select("id, name, name_ru, name_jp, poster_url, score, episodes_count, status, year").
		Where("id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]RecAnimePayload, len(rows))
	for _, r := range rows {
		out[r.ID] = RecAnimePayload{
			ID:            r.ID,
			Name:          r.Name,
			NameRU:        r.NameRU,
			NameJP:        r.NameJP,
			PosterURL:     r.PosterURL,
			Score:         r.Score,
			EpisodesCount: r.EpisodesCount,
			Status:        r.Status,
			Year:          r.Year,
		}
	}
	return out, nil
}

// breakdownAsFloat64 converts the strongly-typed Ensemble breakdown map to
// the JSON-friendly map[string]float64 the AdminRecRow exposes. Pure helper.
func breakdownAsFloat64(in map[recs.SignalID]recs.NormalizedScore) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[string(k)] = float64(v)
	}
	return out
}

// topS5TFIDFTerms parses the rec_user_signals.s5_affinity JSONB blob and
// returns the top-N terms by absolute affinity value. Each term is rendered
// as {attribute: "studio", value: "Madhouse", affinity: 0.42} for the
// expand-on-click panel. The TF/IDF math is collapsed into the single
// affinity value at precompute time per S5's design — we don't surface the
// underlying TF and IDF separately here (deferred to v2.1 if useful).
//
// Returns nil when the blob is empty or unparseable.
func topS5TFIDFTerms(jsonStr string, n int) []map[string]interface{} {
	if jsonStr == "" || jsonStr == "{}" {
		return nil
	}
	affinity := make(map[string]float64)
	if err := json.Unmarshal([]byte(jsonStr), &affinity); err != nil {
		return nil
	}
	if len(affinity) == 0 {
		return nil
	}
	type kv struct {
		Key string
		Val float64
	}
	all := make([]kv, 0, len(affinity))
	for k, v := range affinity {
		all = append(all, kv{Key: k, Val: v})
	}
	sort.SliceStable(all, func(i, j int) bool {
		ai, aj := all[i].Val, all[j].Val
		if ai < 0 {
			ai = -ai
		}
		if aj < 0 {
			aj = -aj
		}
		if ai != aj {
			return ai > aj
		}
		return all[i].Key < all[j].Key
	})
	if n > len(all) {
		n = len(all)
	}
	out := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		// affinity key is "<dim>:<value>" — split for readability.
		dim, val := splitAffinityKey(all[i].Key)
		out = append(out, map[string]interface{}{
			"attribute": dim,
			"value":     val,
			"affinity":  all[i].Val,
		})
	}
	return out
}

// splitAffinityKey splits "studio:Madhouse" into ("studio", "Madhouse").
// Returns ("", key) when no separator is found.
func splitAffinityKey(key string) (dim, val string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}
