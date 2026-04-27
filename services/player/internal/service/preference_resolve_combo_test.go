// Wave 0 RED test — references metrics created by Wave 1 plan 01-02.
// This file SHOULD fail to compile until Wave 1 lands. Going green is the Wave 1
// acceptance gate (per phase 01 VALIDATION.md).
//
// Symbols referenced that DO NOT yet exist:
//   - metrics.ComboResolveTotal
//     (libs/metrics/watch.go — added in plan 01-02)
//
// Behavioral contract — the assertion below FREEZES the Wave 1 contract:
//   - PreferenceService.Resolve must increment metrics.ComboResolveTotal
//     by exactly 1 per call (the denominator for the override-rate metric)
//   - The labels emitted are (tier, language, anon, player). With userID
//     non-empty, anon="false". The other three derive from result.

package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupComboResolveTestDB stands up a minimal SQLite in-memory schema sufficient
// for PreferenceRepository to return predictable empty results so the resolver
// falls through to the default tier (which is what we want — we're testing the
// metric emit, not the resolver branches; resolver_test.go covers branch logic).
func setupComboResolveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// User-anime preference (Tier 1) — empty for this test.
	require.NoError(t, db.Exec(`CREATE TABLE user_anime_preferences (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		player TEXT NOT NULL,
		language TEXT NOT NULL,
		watch_type TEXT NOT NULL,
		translation_id TEXT,
		translation_title TEXT,
		updated_at DATETIME
	)`).Error)

	// Watch history (Tier 2 source) — empty for this test.
	require.NoError(t, db.Exec(`CREATE TABLE watch_histories (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		anime_id TEXT,
		player TEXT,
		language TEXT,
		watch_type TEXT,
		translation_title TEXT,
		duration_watched INTEGER DEFAULT 0,
		started_at DATETIME,
		created_at DATETIME
	)`).Error)

	// Pinned translations (Tier 4 source) — empty for this test.
	require.NoError(t, db.Exec(`CREATE TABLE pinned_translations (
		anime_id TEXT,
		translation_id INTEGER,
		translation_title TEXT,
		translation_type TEXT
	)`).Error)

	return db
}

func TestResolve_IncrementsComboCounter(t *testing.T) {
	db := setupComboResolveTestDB(t)
	prefRepo := repo.NewPreferenceRepository(db)
	log := logger.Default()
	svc := NewPreferenceService(prefRepo, log)

	// One available combo — Tier 5 (default kodik+ru+sub) will pick this.
	available := []domain.WatchCombo{{
		Player:           "kodik",
		Language:         "ru",
		WatchType:        "sub",
		TranslationID:    "963",
		TranslationTitle: "Crunchyroll",
	}}

	// Capture counter at the labels Wave 1 will emit. With userID="user-1"
	// (non-empty) the anon label is "false". Tier comes from the resolver result.
	// Default tier name is "default" per resolver.go.
	getCounter := func() float64 {
		return testutil.ToFloat64(metrics.ComboResolveTotal.WithLabelValues("default", "ru", "false", "kodik"))
	}

	before := getCounter()

	_, err := svc.Resolve(context.Background(), "user-1", &domain.ResolveRequest{
		AnimeID:   "anime-x",
		Available: available,
	})
	require.NoError(t, err, "Resolve should not error on a default-tier path")

	after := getCounter()
	assert.InDelta(t, 1.0, after-before, 0.001,
		"PreferenceService.Resolve must increment metrics.ComboResolveTotal by exactly 1 — "+
			"this is the denominator for combo_override / combo_resolve in Grafana")
}
