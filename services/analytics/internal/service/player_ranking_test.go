package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// fakeRankingWriter captures published payloads without a live Redis.
type fakeRankingWriter struct {
	global   []ProviderRank
	perAnime map[string][]ProviderRank
}

func (f *fakeRankingWriter) PublishRanking(_ context.Context, global []ProviderRank, perAnime map[string][]ProviderRank) error {
	f.global = global
	f.perAnime = perAnime
	return nil
}

func TestScoreAndSort(t *testing.T) {
	// provider A: perfect; provider B: half reach, some stalls. A must rank first.
	rows := []repo.ProviderReliabilityRow{
		{Provider: "a", Resolves: 100, Reached: 100, OK: 100, Stalls: 0, P95MS: 1000},
		{Provider: "b", Resolves: 100, Reached: 50, OK: 80, Stalls: 40, P95MS: 9000},
	}
	ranks := scoreProviders(rows)
	if len(ranks) != 2 {
		t.Fatalf("want 2 ranks, got %d", len(ranks))
	}
	if ranks[0].Provider != "a" {
		t.Errorf("want a first, got %q", ranks[0].Provider)
	}
	if ranks[0].Score <= ranks[1].Score {
		t.Errorf("a score %.3f must beat b %.3f", ranks[0].Score, ranks[1].Score)
	}
	if ranks[0].ReachedRate != 1.0 {
		t.Errorf("a reached_rate = %.3f, want 1.0", ranks[0].ReachedRate)
	}
}

func TestPublishSplitsGlobalAndPerAnime(t *testing.T) {
	w := &fakeRankingWriter{}
	s := NewPlayerRankingService(nil, w)
	global := []repo.ProviderReliabilityRow{{Provider: "a", Resolves: 50, Reached: 50, OK: 50}}
	perAnime := []repo.ProviderReliabilityRow{
		{AnimeID: "uuid-1", Provider: "a", Resolves: 30, Reached: 30, OK: 30},
		{AnimeID: "uuid-1", Provider: "b", Resolves: 30, Reached: 10, OK: 20},
	}
	if err := s.publish(context.Background(), global, perAnime); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(w.global) != 1 || w.global[0].Provider != "a" {
		t.Errorf("global = %+v", w.global)
	}
	if got := w.perAnime["uuid-1"]; len(got) != 2 || got[0].Provider != "a" {
		t.Errorf("perAnime[uuid-1] = %+v", got)
	}
}
