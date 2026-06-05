package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Group 5 — Shikimori import maps the `rewatches` count (design 2026-06-05).
//
// Today the importer parses shikimoriAnimeRate.Rewatches but throws it away
// (only a status=='rewatching' bool is derived). buildShikimoriListReq is the
// extracted pure mapper so the count → rewatch_count mapping is unit-testable.

func TestBuildShikimoriListReq_MapsRewatchesToCount(t *testing.T) {
	req := buildShikimoriListReq(shikimoriAnimeRate{Status: "completed", Rewatches: 3}, "anime-1")
	require.NotNil(t, req.RewatchCount)
	assert.Equal(t, 3, *req.RewatchCount)
}

func TestBuildShikimoriListReq_ZeroRewatches_OverwritesToZero(t *testing.T) {
	// Re-import is authoritative: a source count of 0 must overwrite, not be
	// dropped (nil), so a stale local count can't survive a fresh import.
	req := buildShikimoriListReq(shikimoriAnimeRate{Status: "completed", Rewatches: 0}, "anime-1")
	require.NotNil(t, req.RewatchCount)
	assert.Equal(t, 0, *req.RewatchCount)
}

func TestBuildShikimoriListReq_RewatchingStatus_SetsFlagAndCount(t *testing.T) {
	req := buildShikimoriListReq(shikimoriAnimeRate{Status: "rewatching", Rewatches: 2}, "anime-1")
	require.NotNil(t, req.IsRewatching)
	assert.True(t, *req.IsRewatching, "status=rewatching sets the in-progress flag")
	require.NotNil(t, req.RewatchCount)
	assert.Equal(t, 2, *req.RewatchCount)
}

func TestBuildShikimoriListReq_PreservesScoreEpisodesAndAnimeID(t *testing.T) {
	req := buildShikimoriListReq(shikimoriAnimeRate{Status: "completed", Score: 8, Episodes: 12, Rewatches: 1}, "anime-1")
	assert.Equal(t, "anime-1", req.AnimeID)
	require.NotNil(t, req.Score)
	assert.Equal(t, 8, *req.Score)
	require.NotNil(t, req.Episodes)
	assert.Equal(t, 12, *req.Episodes)
}
