package service

import (
	"math"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedNow is the reference "now" used by all decay-related tests in this file
// so the math is reproducible regardless of when the test runs.
var fixedNow = time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)

// histRow is a tiny constructor to keep the test cases readable.
func histRow(player, lang, wtype, tid, title string, durationSec int, ageDays float64) domain.WatchHistory {
	return domain.WatchHistory{
		Player:           player,
		Language:         lang,
		WatchType:        wtype,
		TranslationID:    tid,
		TranslationTitle: title,
		DurationWatched:  durationSec,
		WatchedAt:        fixedNow.Add(-time.Duration(ageDays * 24 * float64(time.Hour))),
	}
}

func TestAggregateTier2_EmptyHistory(t *testing.T) {
	coarse, fine, total := AggregateTier2(nil, 30, fixedNow, 60)
	assert.Empty(t, coarse)
	assert.Empty(t, fine)
	assert.Zero(t, total)
}

func TestAggregateTier2_SingleRowAtAgeZero_FullDurationWeight(t *testing.T) {
	rows := []domain.WatchHistory{
		histRow("kodik", "ru", "dub", "610", "AniLibria", 1440, 0),
	}
	coarse, fine, total := AggregateTier2(rows, 30, fixedNow, 60)

	require.Len(t, coarse, 1)
	require.Len(t, fine, 1)
	assert.InDelta(t, 1440.0, total, 0.01, "age=0 → no decay → full duration weight")
	assert.Equal(t, "ru", coarse[0].Language)
	assert.Equal(t, "dub", coarse[0].WatchType)
	assert.Equal(t, "AniLibria", fine[0].TranslationTitle)
}

func TestAggregateTier2_OneHalfLifeReducesWeightByHalf(t *testing.T) {
	rows := []domain.WatchHistory{
		histRow("kodik", "ru", "dub", "610", "AniLibria", 1000, 30), // exactly one half-life old
	}
	_, _, total := AggregateTier2(rows, 30, fixedNow, 60)
	// Expected = 1000 * 0.5
	assert.InDelta(t, 500.0, total, 0.5, "30d old at half_life=30 → weight halved")
}

func TestAggregateTier2_DurationFloor_RescuesLegacyZeroRows(t *testing.T) {
	rows := []domain.WatchHistory{
		histRow("kodik", "ru", "dub", "610", "AniLibria", 0, 0), // legacy row with no duration
	}
	_, _, total := AggregateTier2(rows, 30, fixedNow, 60)
	assert.InDelta(t, 60.0, total, 0.01, "duration=0 row clamps to durationFloor")
}

func TestAggregateTier2_RowsWithUnknownDimensionsAreSkipped(t *testing.T) {
	rows := []domain.WatchHistory{
		histRow("kodik", "", "dub", "610", "AniLibria", 1000, 0),  // empty language → skip
		histRow("kodik", "ru", "", "610", "AniLibria", 1000, 0),   // empty watch_type → skip
		histRow("kodik", "ru", "dub", "610", "AniLibria", 500, 0), // valid
	}
	coarse, fine, total := AggregateTier2(rows, 30, fixedNow, 60)
	require.Len(t, coarse, 1)
	require.Len(t, fine, 1)
	assert.InDelta(t, 500.0, total, 0.01, "only the valid row should contribute")
}

func TestAggregateTier2_TwoSignalsAggregateIndependently(t *testing.T) {
	rows := []domain.WatchHistory{
		// Two episodes with the SAME (language, watch_type) but DIFFERENT teams
		histRow("kodik", "ru", "dub", "610", "AniLibria", 1000, 0),
		histRow("kodik", "ru", "dub", "609", "AniDUB", 800, 0),
		// Plus an EN dub from a different combo
		histRow("hianime", "en", "dub", "hd-1", "HD-1", 600, 0),
	}
	coarse, fine, total := AggregateTier2(rows, 30, fixedNow, 60)

	// Coarse should have 2 buckets: ru/dub (1800), en/dub (600)
	require.Len(t, coarse, 2)
	assert.Equal(t, "ru", coarse[0].Language)
	assert.Equal(t, "dub", coarse[0].WatchType)
	assert.InDelta(t, 1800.0, coarse[0].Weight, 0.01)
	assert.Equal(t, "en", coarse[1].Language)
	assert.InDelta(t, 600.0, coarse[1].Weight, 0.01)

	// Fine should have 3 separate teams: AniLibria, AniDUB, HD-1
	require.Len(t, fine, 3)
	assert.Equal(t, "AniLibria", fine[0].TranslationTitle, "AniLibria heaviest")
	assert.InDelta(t, 1000.0, fine[0].Weight, 0.01)

	assert.InDelta(t, 2400.0, total, 0.01)
}

func TestAggregateTier2_BinaryDecayDecaysCorrectly(t *testing.T) {
	// Two equal-duration rows but one is 60 days old (2 half-lives).
	rows := []domain.WatchHistory{
		histRow("kodik", "ru", "dub", "610", "AniLibria", 1000, 0),  // weight 1000
		histRow("kodik", "ru", "dub", "609", "AniDUB", 1000, 60),    // weight 250 (1000 * 0.25)
	}
	coarse, _, total := AggregateTier2(rows, 30, fixedNow, 60)
	require.Len(t, coarse, 1, "both rows in same coarse bucket")
	assert.InDelta(t, 1250.0, total, 1.0, "1000 + 250 (decayed)")
	assert.InDelta(t, 1250.0, coarse[0].Weight, 1.0)
}

func TestChooseTier2Lock_BelowConfidenceFloor_ReturnsNil(t *testing.T) {
	coarse := []domain.WeightedCoarse{
		{Language: "ru", WatchType: "dub", Weight: 500},
	}
	fine := []domain.WeightedFine{
		{Language: "ru", WatchType: "dub", TranslationTitle: "AniLibria", Weight: 500},
	}
	lock := ChooseTier2Lock(coarse, fine, 500, 1800)
	assert.Nil(t, lock, "total weight 500 < floor 1800 → no lock")
}

func TestChooseTier2Lock_AboveFloor_PicksTopCoarse_AndTopFineInLock(t *testing.T) {
	coarse := []domain.WeightedCoarse{
		{Language: "ru", WatchType: "dub", Weight: 5000}, // top
		{Language: "en", WatchType: "dub", Weight: 1000},
	}
	fine := []domain.WeightedFine{
		// Top fine is in en+dub but the lock is ru+dub — chooser must filter
		{Language: "en", WatchType: "dub", TranslationTitle: "HD-1", Weight: 1000},
		{Language: "ru", WatchType: "dub", TranslationTitle: "AniLibria", Weight: 3000},
		{Language: "ru", WatchType: "dub", TranslationTitle: "AniDUB", Weight: 2000},
	}
	lock := ChooseTier2Lock(coarse, fine, 6000, 1800)
	require.NotNil(t, lock)
	assert.Equal(t, "ru", lock.Language)
	assert.Equal(t, "dub", lock.WatchType)
	assert.Equal(t, "AniLibria", lock.TopTranslationTitle, "top fine must be inside locked language+watch_type")
	assert.InDelta(t, 6000.0, lock.Confidence, 0.01)
}

func TestChooseTier2Lock_EmptyCoarse_ReturnsNil(t *testing.T) {
	lock := ChooseTier2Lock(nil, nil, 9999, 1800)
	assert.Nil(t, lock)
}

// TestAggregateTier2_ManyRowsLatencyCheck loosely verifies AggregateTier2 stays
// well under the 50ms p95 budget for the resolver even at the MaxHistoryRows
// safety cap. Not a formal benchmark — runs in unit-test time and passes
// trivially on any reasonable hardware. Provides a regression flag if someone
// accidentally introduces an O(n²) loop.
func TestAggregateTier2_ManyRowsLatencyCheck(t *testing.T) {
	const n = 5000 // matches default config.Tier2Config.MaxHistoryRows
	rows := make([]domain.WatchHistory, n)
	for i := range rows {
		ageDays := math.Mod(float64(i), 365)
		rows[i] = histRow("kodik", "ru", "dub", "610", "AniLibria", 1200, ageDays)
	}
	start := time.Now()
	_, _, total := AggregateTier2(rows, 30, fixedNow, 60)
	dur := time.Since(start)

	assert.Greater(t, total, 0.0)
	assert.Less(t, dur, 50*time.Millisecond, "5k-row aggregation should stay well under resolver latency budget")
}
