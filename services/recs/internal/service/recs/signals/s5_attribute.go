package signals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/recs/internal/service/recs"
	"gorm.io/gorm"
)

// S5Attribute is the TF-IDF time-weighted attribute affinity signal per
// spec §3.1 / §13. Six attribute dimensions with the locked weights from
// Decision §A2 (the spec's separate "producers" weight is collapsed into
// "studios" because Shikimori does not separate them):
//
//	tag         0.30
//	studio      0.25  (= 0.20 studios + 0.05 producers per Decision §A2)
//	genre       0.15
//	demographic 0.10  (proxied by animes.rating per Decision §A3)
//	source      0.10  (sourced from animes.material_source)
//	kind        0.10  (sourced from animes.kind)
//	  sum       1.00
//
// Storage: rec_user_signals.s5_affinity JSONB keyed by "{dim}:{attr_id}",
// value is the per-attribute affinity = tf*idf.
//
// Kodik fallback (spec §3.1): watch_history rows with player='kodik' use
// unit=1 (integer episode count) instead of max(duration_watched/60, 1)
// because Kodik writes 0 for duration_watched (the iframe player does
// not expose video.currentTime).
//
// Cold-start contract (spec §3.3): users with zero watch_history get an
// empty {} JSONB. Score returns an empty map — the ensemble normalizer
// treats absent entries as zero, same contract as S1/S2.
type S5Attribute struct {
	db   *gorm.DB
	repo *repo.RecsRepository

	// idfCalls counts how many times the population IDF was actually computed
	// from the DB (computePopulationIDF). The IDF-hoist (audit L648) computes
	// it ONCE per cron tick via PrecomputeShared and reuses it across every
	// per-user Precompute, so this stays at 1 per tick instead of N (one per
	// user). Read by tests via idfComputeCalls().
	idfCalls atomic.Int64
}

// s5PopulationIDF holds the six population-scope IDF tables that are identical
// for every user within a single cron tick. Computed once by PrecomputeShared
// and threaded through ctx so per-user Precompute calls reuse it (audit L648).
type s5PopulationIDF struct {
	tag    map[string]float64
	studio map[string]float64
	genre  map[string]float64
	kind   map[string]float64
	rating map[string]float64
	source map[string]float64
}

// s5IDFContextKey is the unexported context key under which PrecomputeShared
// stows the per-tick IDF bundle. Unexported type prevents collisions.
type s5IDFContextKey struct{}

// idfFromContext returns the per-tick IDF bundle if PrecomputeShared seeded it.
func idfFromContext(ctx context.Context) (*s5PopulationIDF, bool) {
	idf, ok := ctx.Value(s5IDFContextKey{}).(*s5PopulationIDF)
	return idf, ok
}

const (
	// Attribute dimension key prefixes for the s5_affinity JSONB. The
	// "{dim}:{attr_id}" format is locked for v1 (Decision §A2 / plan
	// 12-03 reference_data). Admin debug page in Phase 14 will use these
	// prefixes to group the per-dimension breakdown.
	s5DimTag         = "tag"
	s5DimStudio      = "studio"
	s5DimGenre       = "genre"
	s5DimDemographic = "rating" // Decision §A3 — proxied by animes.rating
	s5DimSource      = "source" // animes.material_source
	s5DimKind        = "kind"   // animes.kind

	// Per-attribute weights — locked from Decision §A2. Sum MUST equal 1.0
	// (verified at runtime by TestS5Attribute_ScorePerAttributeWeights).
	// If anyone reverts s5WeightStudio to 0.20 + adds a separate producer
	// dimension, the test breaks — see <threat_model> T-12-22.
	s5WeightTag = 0.30
	// s5WeightStudio absorbs both the spec's separate studios (0.20) and
	// the spec's separate producers (0.05) weight per Decision §A2 —
	// Shikimori does not separate them.
	s5WeightStudio      = 0.25
	s5WeightGenre       = 0.15
	s5WeightDemographic = 0.10
	s5WeightSource      = 0.10
	s5WeightKind        = 0.10
)

// NewS5Attribute wires S5 with the player DB handle and the recs repository.
func NewS5Attribute(db *gorm.DB, recsRepo *repo.RecsRepository) *S5Attribute {
	return &S5Attribute{db: db, repo: recsRepo}
}

// ID returns the stable signal identifier "s5".
func (s *S5Attribute) ID() recs.SignalID { return recs.SignalID("s5") }

// idfComputeCalls returns how many times the population IDF was computed from
// the DB. Test-only observability for the IDF-hoist (audit L648).
func (s *S5Attribute) idfComputeCalls() int64 { return s.idfCalls.Load() }

// computePopulationIDF runs the six population-scope IDF queries once. It is
// the single place the IDF is materialized; PrecomputeShared calls it once per
// tick, and the per-user Precompute inline-fallback calls it when no shared
// bundle was seeded into ctx. The idfCalls counter increments on every actual
// computation so the hoist can be verified (computed once per tick, not per
// user).
func (s *S5Attribute) computePopulationIDF(ctx context.Context) (*s5PopulationIDF, error) {
	s.idfCalls.Add(1)

	totalUsers, err := s.totalUsersWithHistory(ctx)
	if err != nil {
		return nil, err
	}
	if totalUsers == 0 {
		// Defensive — callers only reach IDF when at least one user has
		// history, so totalUsers >= 1. Belt-and-braces.
		totalUsers = 1
	}

	idfTag, err := s.idfMultiValue(ctx, "anime_tags", "tag_id", totalUsers)
	if err != nil {
		return nil, err
	}
	idfStudio, err := s.idfMultiValue(ctx, "anime_studios", "studio_id", totalUsers)
	if err != nil {
		return nil, err
	}
	idfGenre, err := s.idfMultiValue(ctx, "anime_genres", "genre_id", totalUsers)
	if err != nil {
		return nil, err
	}
	idfKind, idfRating, idfSource, err := s.idfSingleValueAttrs(ctx, totalUsers)
	if err != nil {
		return nil, err
	}

	return &s5PopulationIDF{
		tag:    idfTag,
		studio: idfStudio,
		genre:  idfGenre,
		kind:   idfKind,
		rating: idfRating,
		source: idfSource,
	}, nil
}

// PrecomputeShared computes the population-scope IDF once and returns a child
// context carrying it (audit L648). The UserOrchestrator calls this once per
// RunOnce sweep before iterating users; each per-user Precompute then reuses
// the seeded IDF instead of recomputing it. Implements the optional
// recs.SharedPrecomputer interface. Returns the parent ctx unchanged on error
// so the caller can still fall back to per-user inline IDF.
func (s *S5Attribute) PrecomputeShared(ctx context.Context) (context.Context, error) {
	idf, err := s.computePopulationIDF(ctx)
	if err != nil {
		return ctx, fmt.Errorf("s5: precompute shared IDF: %w", err)
	}
	return context.WithValue(ctx, s5IDFContextKey{}, idf), nil
}

// unitForRow returns the time-weighted unit for one watch_history row per
// the spec §3.1 Kodik fallback rule.
func unitForRow(player string, durationWatched int) float64 {
	if player == "kodik" {
		// Integer episode count fallback — one row = one watched episode.
		// Kodik iframes do not expose video.currentTime so duration_watched
		// is always 0 here.
		return 1.0
	}
	return math.Max(float64(durationWatched)/60.0, 1.0)
}

// Precompute aggregates the user's per-attribute affinity vector via SQL
// joins over watch_history × animes × the m2m attribute join tables, then
// applies the spec §3.1 TF-IDF math and persists the result to
// rec_user_signals.s5_affinity.
//
// Idempotent — repeated calls upsert the same vector with a fresh
// LastComputed stamp (the underlying data hasn't changed).
//
// Cold-start: a user with zero watch_history rows persists "{}" so the
// row exists for downstream Score reads.
func (s *S5Attribute) Precompute(ctx context.Context, userID recs.UserID) error {
	// 1. Load user history rows. We do the unit math in Go (rather than
	//    pushing GREATEST + CASE into SQL) so the same code path works on
	//    sqlite (tests) and postgres (production).
	var historyRows []struct {
		AnimeID         string
		Player          string
		DurationWatched int
	}
	if err := s.db.WithContext(ctx).
		Table("watch_history").
		Select("anime_id, player, duration_watched").
		Where("user_id = ?", userID).
		Scan(&historyRows).Error; err != nil {
		return fmt.Errorf("s5: load watch_history: %w", err)
	}

	if len(historyRows) == 0 {
		// Cold-start gate: persist "{}" so the row exists.
		return s.persistVector(ctx, userID, "{}")
	}

	// 2. Per-anime units. One anime can appear in multiple history rows
	//    (Kodik writes one row per episode, some providers can write multiple
	//    if the user re-opened the same episode). We sum the per-row
	//    units before computing TF.
	unitsByAnime := make(map[string]float64)
	var totalUnits float64
	for _, r := range historyRows {
		u := unitForRow(r.Player, r.DurationWatched)
		unitsByAnime[r.AnimeID] += u
		totalUnits += u
	}
	if totalUnits == 0 {
		// Defensive — unitForRow's floor ensures every reliable-player row
		// contributes >= 1 and every Kodik row contributes 1, so this is
		// effectively unreachable. Persist empty to be safe.
		return s.persistVector(ctx, userID, "{}")
	}

	animeIDs := make([]string, 0, len(unitsByAnime))
	for id := range unitsByAnime {
		animeIDs = append(animeIDs, id)
	}

	// 3. Compute per-dimension TF (term frequency).
	tfTag, err := s.tfMultiValue(ctx, "anime_tags", "tag_id", animeIDs, unitsByAnime, totalUnits)
	if err != nil {
		return err
	}
	tfStudio, err := s.tfMultiValue(ctx, "anime_studios", "studio_id", animeIDs, unitsByAnime, totalUnits)
	if err != nil {
		return err
	}
	tfGenre, err := s.tfMultiValue(ctx, "anime_genres", "genre_id", animeIDs, unitsByAnime, totalUnits)
	if err != nil {
		return err
	}
	tfKind, tfRating, tfSource, err := s.tfSingleValueAttrs(ctx, animeIDs, unitsByAnime, totalUnits)
	if err != nil {
		return err
	}

	// 4. Resolve the population-scope IDF. IDF is identical for every user in
	//    a cron tick, so the UserOrchestrator computes it ONCE per tick via
	//    PrecomputeShared and seeds it into ctx (audit L648). When present we
	//    reuse it; otherwise (the single-user TriggerForUser path, or any
	//    standalone Precompute) we compute it inline so S5 still works on its
	//    own.
	idf, ok := idfFromContext(ctx)
	if !ok {
		idf, err = s.computePopulationIDF(ctx)
		if err != nil {
			return err
		}
	}

	// 5. Combine TF * IDF into the final affinity vector keyed by
	//    "{dim}:{attr_id}".
	affinity := make(map[string]float64)
	for k, tf := range tfTag {
		affinity[s5DimTag+":"+k] = tf * idf.tag[k]
	}
	for k, tf := range tfStudio {
		affinity[s5DimStudio+":"+k] = tf * idf.studio[k]
	}
	for k, tf := range tfGenre {
		affinity[s5DimGenre+":"+k] = tf * idf.genre[k]
	}
	for k, tf := range tfKind {
		affinity[s5DimKind+":"+k] = tf * idf.kind[k]
	}
	for k, tf := range tfRating {
		affinity[s5DimDemographic+":"+k] = tf * idf.rating[k]
	}
	for k, tf := range tfSource {
		affinity[s5DimSource+":"+k] = tf * idf.source[k]
	}

	encoded, err := json.Marshal(affinity)
	if err != nil {
		return fmt.Errorf("s5: marshal affinity vector: %w", err)
	}
	return s.persistVector(ctx, userID, string(encoded))
}

// tfMultiValue computes TF for a single multi-value (m2m) dimension by
// joining anime_<X> on the user's history anime IDs and summing the
// per-anime units that touched each attribute, then dividing by totalUnits.
func (s *S5Attribute) tfMultiValue(
	ctx context.Context,
	joinTable, attrColumn string,
	animeIDs []string,
	unitsByAnime map[string]float64,
	totalUnits float64,
) (map[string]float64, error) {
	if len(animeIDs) == 0 {
		return map[string]float64{}, nil
	}
	var rows []struct {
		AnimeID string
		AttrID  string
	}
	err := s.db.WithContext(ctx).
		Table(joinTable).
		Select("anime_id, "+attrColumn+" AS attr_id").
		Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("s5: tfMultiValue %s: %w", joinTable, err)
	}
	tf := make(map[string]float64)
	for _, r := range rows {
		tf[r.AttrID] += unitsByAnime[r.AnimeID]
	}
	for k := range tf {
		tf[k] /= totalUnits
	}
	return tf, nil
}

// tfSingleValueAttrs reads the user's watched-animes' kind, rating, and
// material_source columns and aggregates per-attribute units → TF.
func (s *S5Attribute) tfSingleValueAttrs(
	ctx context.Context,
	animeIDs []string,
	unitsByAnime map[string]float64,
	totalUnits float64,
) (kind, rating, source map[string]float64, err error) {
	if len(animeIDs) == 0 {
		return map[string]float64{}, map[string]float64{}, map[string]float64{}, nil
	}
	var rows []struct {
		AnimeID        string
		Kind           string
		Rating         string
		MaterialSource string
	}
	err = s.db.WithContext(ctx).
		Table("animes").
		Select("id AS anime_id, kind, rating, material_source").
		Where("id IN ?", animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, nil, nil, fmt.Errorf("s5: tfSingleValueAttrs: %w", err)
	}
	kind = make(map[string]float64)
	rating = make(map[string]float64)
	source = make(map[string]float64)
	for _, r := range rows {
		u := unitsByAnime[r.AnimeID]
		if r.Kind != "" {
			kind[r.Kind] += u
		}
		if r.Rating != "" {
			rating[r.Rating] += u
		}
		if r.MaterialSource != "" {
			source[r.MaterialSource] += u
		}
	}
	for k := range kind {
		kind[k] /= totalUnits
	}
	for k := range rating {
		rating[k] /= totalUnits
	}
	for k := range source {
		source[k] /= totalUnits
	}
	return kind, rating, source, nil
}

// totalUsersWithHistory returns the total number of distinct user IDs in
// watch_history. Used as the numerator in the IDF formula
// log(total_users / (1 + users_with_attr)).
func (s *S5Attribute) totalUsersWithHistory(ctx context.Context) (int, error) {
	var n int64
	err := s.db.WithContext(ctx).
		Table("watch_history").
		Select("COUNT(DISTINCT user_id)").
		Scan(&n).Error
	if err != nil {
		return 0, fmt.Errorf("s5: total users: %w", err)
	}
	return int(n), nil
}

// idfMultiValue computes IDF for a single multi-value dimension. Returns a
// map keyed by attr_id with idf = log(total_users / (1 + users_with_attr)).
func (s *S5Attribute) idfMultiValue(
	ctx context.Context,
	joinTable, attrColumn string,
	totalUsers int,
) (map[string]float64, error) {
	type row struct {
		AttrID    string
		UserCount int64
	}
	var rows []row
	q := fmt.Sprintf(`
		SELECT j.%s AS attr_id, COUNT(DISTINCT wh.user_id) AS user_count
		FROM watch_history wh
		JOIN %s j ON j.anime_id = wh.anime_id
		GROUP BY j.%s
	`, attrColumn, joinTable, attrColumn)
	if err := s.db.WithContext(ctx).Raw(q).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("s5: idfMultiValue %s: %w", joinTable, err)
	}
	out := make(map[string]float64, len(rows))
	tu := float64(totalUsers)
	for _, r := range rows {
		out[r.AttrID] = math.Log(tu / (1.0 + float64(r.UserCount)))
	}
	return out, nil
}

// idfSingleValueAttrs computes IDF for the kind / rating / material_source
// columns by counting distinct users-per-attribute across watch_history
// joined to animes.
func (s *S5Attribute) idfSingleValueAttrs(
	ctx context.Context,
	totalUsers int,
) (kind, rating, source map[string]float64, err error) {
	type row struct {
		AttrID    string
		UserCount int64
	}
	tu := float64(totalUsers)

	q := func(col string) string {
		return fmt.Sprintf(`
			SELECT a.%s AS attr_id, COUNT(DISTINCT wh.user_id) AS user_count
			FROM watch_history wh
			JOIN animes a ON a.id = wh.anime_id
			WHERE a.%s != ''
			GROUP BY a.%s
		`, col, col, col)
	}

	var kindRows, ratingRows, sourceRows []row
	if err = s.db.WithContext(ctx).Raw(q("kind")).Scan(&kindRows).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("s5: idf kind: %w", err)
	}
	if err = s.db.WithContext(ctx).Raw(q("rating")).Scan(&ratingRows).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("s5: idf rating: %w", err)
	}
	if err = s.db.WithContext(ctx).Raw(q("material_source")).Scan(&sourceRows).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("s5: idf material_source: %w", err)
	}

	kind = make(map[string]float64, len(kindRows))
	for _, r := range kindRows {
		kind[r.AttrID] = math.Log(tu / (1.0 + float64(r.UserCount)))
	}
	rating = make(map[string]float64, len(ratingRows))
	for _, r := range ratingRows {
		rating[r.AttrID] = math.Log(tu / (1.0 + float64(r.UserCount)))
	}
	source = make(map[string]float64, len(sourceRows))
	for _, r := range sourceRows {
		source[r.AttrID] = math.Log(tu / (1.0 + float64(r.UserCount)))
	}
	return kind, rating, source, nil
}

// persistVector upserts the s5_affinity for a user, preserving any existing
// S1Vector + S6Seed* fields the orchestrator (or a previous S1 run) may
// have already written. Mirrors the s1_score_cluster.go::persistVector
// pattern from Phase 11.
func (s *S5Attribute) persistVector(ctx context.Context, userID recs.UserID, jsonStr string) error {
	existing, err := s.repo.GetUserSignals(ctx, string(userID))
	if err != nil {
		return fmt.Errorf("s5: load existing user signals: %w", err)
	}
	now := time.Now().UTC()
	row := &domain.RecUserSignals{
		UserID:       string(userID),
		S1Vector:     "{}",
		S5Affinity:   jsonStr,
		LastComputed: now,
	}
	if existing != nil {
		// Don't clobber S1 / S6 fields the orchestrator may have populated.
		if existing.S1Vector != "" {
			row.S1Vector = existing.S1Vector
		}
		row.S6SeedAnimeID = existing.S6SeedAnimeID
		row.S6SeedCompletedAt = existing.S6SeedCompletedAt
		row.S6SeedScore = existing.S6SeedScore
	}
	return s.repo.UpsertUserSignals(ctx, row)
}

// Score reads the persisted s5_affinity vector for the user, then computes
// per-candidate raw scores by summing per-attribute affinity * per-dimension
// weight across the candidate's six attribute dimensions.
//
// Candidates with raw == 0 are omitted; the ensemble normalizer treats
// missing entries as zero. Candidates with raw < 0 are also omitted — they
// can occur when a user's history is dominated by universally-watched
// attributes (negative IDF) and the per-pool normalizer would scale them
// into [0,1] arbitrarily; safer to bias toward zero contribution.
func (s *S5Attribute) Score(ctx context.Context, userID recs.UserID, candidates []recs.AnimeID) (map[recs.AnimeID]recs.RawScore, error) {
	out := make(map[recs.AnimeID]recs.RawScore, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}
	row, err := s.repo.GetUserSignals(ctx, string(userID))
	if err != nil {
		return nil, fmt.Errorf("s5: load user signals: %w", err)
	}
	if row == nil || row.S5Affinity == "" {
		return out, nil
	}
	affinity := make(map[string]float64)
	if err := json.Unmarshal([]byte(row.S5Affinity), &affinity); err != nil {
		return nil, fmt.Errorf("s5: parse affinity vector: %w", err)
	}
	if len(affinity) == 0 {
		return out, nil
	}

	// Load each candidate's attributes via batched SELECTs.
	candStrs := make([]string, len(candidates))
	for i, c := range candidates {
		candStrs[i] = string(c)
	}

	tagsByAnime, err := s.loadM2M(ctx, "anime_tags", "tag_id", candStrs)
	if err != nil {
		return nil, err
	}
	studiosByAnime, err := s.loadM2M(ctx, "anime_studios", "studio_id", candStrs)
	if err != nil {
		return nil, err
	}
	genresByAnime, err := s.loadM2M(ctx, "anime_genres", "genre_id", candStrs)
	if err != nil {
		return nil, err
	}
	type attrRow struct {
		AnimeID        string
		Kind           string
		Rating         string
		MaterialSource string
	}
	var attrRows []attrRow
	if err := s.db.WithContext(ctx).
		Table("animes").
		Select("id AS anime_id, kind, rating, material_source").
		Where("id IN ?", candStrs).
		Scan(&attrRows).Error; err != nil {
		return nil, fmt.Errorf("s5: load candidate single-value attrs: %w", err)
	}
	attrsByAnime := make(map[string]attrRow, len(attrRows))
	for _, r := range attrRows {
		attrsByAnime[r.AnimeID] = r
	}

	for _, c := range candidates {
		cs := string(c)
		raw := 0.0
		for _, t := range tagsByAnime[cs] {
			raw += s5WeightTag * affinity[s5DimTag+":"+t]
		}
		for _, st := range studiosByAnime[cs] {
			raw += s5WeightStudio * affinity[s5DimStudio+":"+st]
		}
		for _, g := range genresByAnime[cs] {
			raw += s5WeightGenre * affinity[s5DimGenre+":"+g]
		}
		a, ok := attrsByAnime[cs]
		if ok {
			if a.Rating != "" {
				raw += s5WeightDemographic * affinity[s5DimDemographic+":"+a.Rating]
			}
			if a.MaterialSource != "" {
				raw += s5WeightSource * affinity[s5DimSource+":"+a.MaterialSource]
			}
			if a.Kind != "" {
				raw += s5WeightKind * affinity[s5DimKind+":"+a.Kind]
			}
		}
		// Filter NaN / Inf / non-positive — see method doc for rationale.
		if math.IsNaN(raw) || math.IsInf(raw, 0) || raw <= 0 {
			continue
		}
		out[c] = recs.RawScore(raw)
	}
	return out, nil
}

// loadM2M reads (anime_id, attr_id) pairs from a join table for the given
// anime IDs and returns a map of anime_id -> []attr_id.
func (s *S5Attribute) loadM2M(
	ctx context.Context,
	joinTable, attrColumn string,
	animeIDs []string,
) (map[string][]string, error) {
	out := make(map[string][]string, len(animeIDs))
	if len(animeIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		AnimeID string
		AttrID  string
	}
	err := s.db.WithContext(ctx).
		Table(joinTable).
		Select("anime_id, "+attrColumn+" AS attr_id").
		Where("anime_id IN ?", animeIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("s5: loadM2M %s: %w", joinTable, err)
	}
	for _, r := range rows {
		out[r.AnimeID] = append(out[r.AnimeID], r.AttrID)
	}
	return out, nil
}
