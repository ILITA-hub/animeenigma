package job

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTierDue(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	w := TierWindows{Hot: 36 * time.Hour, Warm: 3 * time.Hour, Floor: 6 * time.Hour}
	soon := now.Add(10 * time.Hour) // within hot window
	far := now.Add(10 * 24 * time.Hour)

	// Never checked → due.
	require.True(t, tierDue(&far, time.Time{}, false, now, w))
	// Hot (imminent) + checked 1h ago → due (every run).
	require.True(t, tierDue(&soon, now.Add(-1*time.Hour), true, now, w))
	// Warm + checked 1h ago → NOT due (warm cadence 3h not elapsed).
	require.False(t, tierDue(&far, now.Add(-1*time.Hour), true, now, w))
	// Warm + checked 4h ago → due (warm cadence elapsed).
	require.True(t, tierDue(&far, now.Add(-4*time.Hour), true, now, w))
	// Warm + checked 2h ago but floor 6h... not yet; still warm-not-due.
	require.False(t, tierDue(&far, now.Add(-2*time.Hour), true, now, w))
	// Any tier + checked 7h ago → floor forces due.
	require.True(t, tierDue(&far, now.Add(-7*time.Hour), true, now, w))
	// Unknown airing (nil) → treated hot → due.
	require.True(t, tierDue(nil, now.Add(-1*time.Hour), true, now, w))
}

func TestTierFilterIncludesAllCombosOfDueAnime(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	w := TierWindows{Hot: 36 * time.Hour, Warm: 3 * time.Hour, Floor: 6 * time.Hour}
	far := now.Add(10 * 24 * time.Hour)
	combos := []domain.Combo{
		{AnimeID: "hot", Player: "kodik"},
		{AnimeID: "hot", Player: "english"}, // second combo of the same anime
		{AnimeID: "cold", Player: "kodik"},
	}
	airing := map[string]*time.Time{"hot": timePtr(now.Add(5 * time.Hour)), "cold": &far}
	lastChecked := map[string]time.Time{
		"hot":  now.Add(-1 * time.Hour), // hot → due
		"cold": now.Add(-1 * time.Hour), // warm, checked 1h ago → not due
	}
	got := tierFilter(combos, airing, lastChecked, now, w)
	require.Len(t, got, 2) // both "hot" combos, no "cold"
	for _, c := range got {
		require.Equal(t, "hot", c.AnimeID)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
