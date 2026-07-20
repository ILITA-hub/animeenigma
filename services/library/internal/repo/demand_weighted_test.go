package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDemandTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE autocache_demand (
		mal_id TEXT, episode INTEGER, reason TEXT, requested_at DATETIME, titles TEXT,
		PRIMARY KEY (mal_id, episode)
	)`).Error)
	return db
}

func seedDemand(t *testing.T, db *gorm.DB, mal string, ep int, reason domain.DemandReason, at time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO autocache_demand (mal_id,episode,reason,requested_at,titles) VALUES (?,?,?,?,?)`,
		mal, ep, string(reason), at, "T").Error)
}

func TestDrainWeightedSplitAndFIFO(t *testing.T) {
	db := setupDemandTestDB(t)
	r := NewDemandRepository(db)
	base := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	// 4 hot (2 ongoing + 2 next_ep) and 4 backfill, ascending requested_at.
	seedDemand(t, db, "h", 1, domain.DemandReasonOngoing, base.Add(1*time.Minute))
	seedDemand(t, db, "h", 2, domain.DemandReasonNextEp, base.Add(2*time.Minute))
	seedDemand(t, db, "h", 3, domain.DemandReasonOngoing, base.Add(3*time.Minute))
	seedDemand(t, db, "h", 4, domain.DemandReasonNextEp, base.Add(4*time.Minute))
	seedDemand(t, db, "c", 1, domain.DemandReasonBackfill, base.Add(1*time.Minute))
	seedDemand(t, db, "c", 2, domain.DemandReasonBackfill, base.Add(2*time.Minute))
	seedDemand(t, db, "c", 3, domain.DemandReasonBackfill, base.Add(3*time.Minute))
	seedDemand(t, db, "c", 4, domain.DemandReasonBackfill, base.Add(4*time.Minute))

	rows, err := r.DrainWeighted(context.Background(), 3, 2)
	require.NoError(t, err)
	require.Len(t, rows, 5)
	// First 3 hot (FIFO), next 2 backfill (FIFO).
	require.Equal(t, "h", rows[0].MALID)
	require.Equal(t, 1, rows[0].Episode)
	require.Equal(t, 3, rows[2].Episode) // hot FIFO up to hotN
	require.Equal(t, domain.DemandReasonBackfill, rows[3].Reason)
	require.Equal(t, 1, rows[3].Episode) // backfill FIFO
}

func TestDrainWeightedShortClassFillsRemainder(t *testing.T) {
	db := setupDemandTestDB(t)
	r := NewDemandRepository(db)
	base := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	// Only 1 hot, plenty of backfill; hotN=3 coldN=2 → 1 hot + 4 backfill = 5.
	seedDemand(t, db, "h", 1, domain.DemandReasonOngoing, base)
	for i := 1; i <= 5; i++ {
		seedDemand(t, db, "c", i, domain.DemandReasonBackfill, base.Add(time.Duration(i)*time.Minute))
	}
	rows, err := r.DrainWeighted(context.Background(), 3, 2)
	require.NoError(t, err)
	require.Len(t, rows, 5) // 1 hot + 4 backfill (remainder filled)
	hot := 0
	for _, x := range rows {
		if x.Reason != domain.DemandReasonBackfill {
			hot++
		}
	}
	require.Equal(t, 1, hot)
}

func TestDrainWeightedNonPositive(t *testing.T) {
	db := setupDemandTestDB(t)
	r := NewDemandRepository(db)
	rows, err := r.DrainWeighted(context.Background(), 0, 0)
	require.NoError(t, err)
	require.Nil(t, rows)
}
