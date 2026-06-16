// Package service — player_ranking.go is the Stage 2b daily provider-reliability
// recompute. It clones the read_threshold pipeline: the scheduler triggers an
// analytics /internal recompute; analytics aggregates the Stage 2a player
// telemetry from ClickHouse and publishes a JSON ranking to Redis (48h TTL) that
// the catalog serves per anime. READS the events table only (records no effect).
package service

import (
	"context"
	"sort"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// Tuning constants for the daily reliability compute.
const (
	rankLookbackDays = 7
	rankMinGlobal    = 20 // global per-provider min resolves
	rankMinPerAnime  = 10 // per-(anime,provider) min resolves (sparser)

	// Composite score weights/penalties (see plan §"Composite score").
	wReached = 0.65
	wOK      = 0.35
	stallCap = 0.20
	stallW   = 0.50
	latCap   = 0.10
	latRefMS = 30000.0
	latW     = 0.10
)

// ProviderRank is one ranked provider entry. JSON tags are the Redis wire
// contract — catalog's sourceranking.Record must match exactly.
type ProviderRank struct {
	Provider    string  `json:"provider"`
	Score       float64 `json:"score"`
	ReachedRate float64 `json:"reached_rate"`
	OKRate      float64 `json:"ok_rate"`
	P95MS       float64 `json:"p95_ms"`
	StallRate   float64 `json:"stall_rate"`
	Samples     uint64  `json:"samples"`
}

// rankingWriter publishes the computed ranking to Redis. Narrow so the service
// does not import go-redis (the adapter lives in cmd/.../adapters.go).
type rankingWriter interface {
	PublishRanking(ctx context.Context, global []ProviderRank, perAnime map[string][]ProviderRank) error
}

// PlayerRankingService computes the daily provider-reliability ranking from
// ClickHouse and publishes it to Redis.
type PlayerRankingService struct {
	conn  driver.Conn
	redis rankingWriter
}

// NewPlayerRankingService builds the service. conn/redis may be nil in degraded
// boots; Recompute guards against nil.
func NewPlayerRankingService(conn driver.Conn, rw rankingWriter) *PlayerRankingService {
	return &PlayerRankingService{conn: conn, redis: rw}
}

// scoreProviders turns raw aggregates into sorted ProviderRank records. Pure +
// unit-tested. Rows with zero resolves are skipped defensively.
func scoreProviders(rows []repo.ProviderReliabilityRow) []ProviderRank {
	out := make([]ProviderRank, 0, len(rows))
	for _, r := range rows {
		if r.Resolves == 0 {
			continue
		}
		resolves := float64(r.Resolves)
		reached := float64(r.Reached) / resolves
		ok := float64(r.OK) / resolves
		stallRate := float64(r.Stalls) / resolves
		stallPen := stallRate * stallW
		if stallPen > stallCap {
			stallPen = stallCap
		}
		latPen := (r.P95MS / latRefMS) * latW
		if latPen > latCap {
			latPen = latCap
		}
		score := wReached*reached + wOK*ok - stallPen - latPen
		if score < 0 {
			score = 0
		}
		out = append(out, ProviderRank{
			Provider:    r.Provider,
			Score:       score,
			ReachedRate: reached,
			OKRate:      ok,
			P95MS:       r.P95MS,
			StallRate:   stallRate,
			Samples:     r.Resolves,
		})
	}
	// scores derive from integer ratios; exact compare is fine, tie-break falls through to Samples.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].ReachedRate != out[j].ReachedRate {
			return out[i].ReachedRate > out[j].ReachedRate
		}
		return out[i].Samples > out[j].Samples
	})
	return out
}

// publish scores global + per-anime rows and hands them to the writer. Exposed
// (lowercase, same package) for the unit test.
func (s *PlayerRankingService) publish(ctx context.Context, globalRows, perAnimeRows []repo.ProviderReliabilityRow) error {
	global := scoreProviders(globalRows)
	byAnime := map[string][]repo.ProviderReliabilityRow{}
	for _, r := range perAnimeRows {
		if r.AnimeID == "" {
			continue
		}
		byAnime[r.AnimeID] = append(byAnime[r.AnimeID], r)
	}
	perAnime := make(map[string][]ProviderRank, len(byAnime))
	for id, rs := range byAnime {
		perAnime[id] = scoreProviders(rs)
	}
	return s.redis.PublishRanking(ctx, global, perAnime)
}

// Recompute is the daily entry point: query ClickHouse (global + per-anime),
// score, publish. nil conn/redis → no-op success (the 48h Redis TTL carries the
// last good ranking until CH/Redis return).
func (s *PlayerRankingService) Recompute(ctx context.Context) error {
	if s.conn == nil || s.redis == nil {
		return nil
	}
	globalRows, err := repo.QueryProviderReliability(ctx, s.conn, rankLookbackDays, rankMinGlobal, false)
	if err != nil {
		return err
	}
	perAnimeRows, err := repo.QueryProviderReliability(ctx, s.conn, rankLookbackDays, rankMinPerAnime, true)
	if err != nil {
		return err
	}
	return s.publish(ctx, globalRows, perAnimeRows)
}
