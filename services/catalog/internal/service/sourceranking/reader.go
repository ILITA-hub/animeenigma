// Package sourceranking reads the Stage 2b provider-reliability ranking that the
// analytics service publishes to Redis (keys player_ranking:global and
// player_ranking:anime:{id}) and exposes it as typed records. This is the
// catalog (read) half of the contract; analytics service.ProviderRank is the
// writer half — the JSON tags MUST stay in sync.
package sourceranking

import (
	"context"
	"encoding/json"
)

// Record mirrors analytics service.ProviderRank exactly (Redis wire contract).
type Record struct {
	Provider    string  `json:"provider"`
	Score       float64 `json:"score"`
	ReachedRate float64 `json:"reached_rate"`
	OKRate      float64 `json:"ok_rate"`
	P95MS       float64 `json:"p95_ms"`
	StallRate   float64 `json:"stall_rate"`
	Samples     uint64  `json:"samples"`
}

// Ranking is the assembled response for one anime.
type Ranking struct {
	Global   []Record `json:"global"`
	PerAnime []Record `json:"perAnime"`
	// Fix is a same-day per-anime override provider (srcfix:{id}, 24h TTL),
	// written when a client-side fallback rescued a failed resolve. Empty = none.
	Fix string `json:"fix"`
}

// stringGetter is the narrow cache surface the reader needs (a string GET with a
// found flag). *cache.RedisCache is adapted to this in main.go.
type stringGetter interface {
	GetString(ctx context.Context, key string) (string, bool)
}

// Reader reads the published ranking keys.
type Reader struct{ cache stringGetter }

func NewReader(c stringGetter) *Reader { return &Reader{cache: c} }

// Read fetches the global ranking and the per-anime ranking (if any). Missing or
// malformed keys yield empty slices — never an error (the ranking is advisory).
func (r *Reader) Read(ctx context.Context, animeID string) Ranking {
	var out Ranking
	if s, ok := r.cache.GetString(ctx, "player_ranking:global"); ok {
		_ = json.Unmarshal([]byte(s), &out.Global)
	}
	if animeID != "" {
		if s, ok := r.cache.GetString(ctx, "player_ranking:anime:"+animeID); ok {
			_ = json.Unmarshal([]byte(s), &out.PerAnime)
		}
		if s, ok := r.cache.GetString(ctx, "srcfix:"+animeID); ok {
			out.Fix = s
		}
	}
	if out.Global == nil {
		out.Global = []Record{}
	}
	if out.PerAnime == nil {
		out.PerAnime = []Record{}
	}
	return out
}
