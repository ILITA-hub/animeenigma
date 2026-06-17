package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// farFutureCron never fires during the test run, so registering a job with it
// proves the wiring (AddFunc accepts the expr + the job appears in GetStatus)
// without actually executing any Run().
const farFutureCron = "0 0 1 1 *" // 00:00 on Jan 1 (annual)

// TestJobService_RegistersAutocacheLogicA verifies the Phase-09 Logic A job is
// wired into the cron harness via the new NewJobService/Start arity and surfaces
// in GetStatus. The other jobs are passed as typed-nil — they are only invoked
// when their (annual) crons fire, which never happens in this test.
func TestJobService_RegistersAutocacheLogicA(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	logicA := jobs.NewAutocacheLogicAJob(db, "http://library:8089", 30, logger.Default())
	prediction := jobs.NewAutocachePredictionJob(db, 30, 1288490188, logger.Default())

	svc := NewJobService(nil, nil, nil, nil, nil, nil, nil, logicA, prediction, logger.Default())

	err = svc.Start(
		farFutureCron, // shikimori
		farFutureCron, // cleanup
		farFutureCron, // topAnime
		farFutureCron, // calendar
		farFutureCron, // scraperPlayabilityCanary
		farFutureCron, // readThreshold (nil job → skipped)
		farFutureCron, // providerRanking (nil job → skipped)
		farFutureCron, // autocacheLogicA
		farFutureCron, // autocachePrediction
	)
	require.NoError(t, err)
	defer svc.Stop()

	status := svc.GetStatus()
	_, ok := status["autocache_logic_a"]
	assert.True(t, ok, "GetStatus must expose autocache_logic_a")
}

// TestJobService_RegistersAutocachePrediction verifies the Phase-11 prediction job
// is wired into the cron harness via the new NewJobService/Start arity and surfaces
// in GetStatus. Unlike Logic A it is always constructed (no optional URL).
func TestJobService_RegistersAutocachePrediction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	prediction := jobs.NewAutocachePredictionJob(db, 30, 1288490188, logger.Default())

	svc := NewJobService(nil, nil, nil, nil, nil, nil, nil, nil, prediction, logger.Default())

	err = svc.Start(
		farFutureCron, farFutureCron, farFutureCron, farFutureCron,
		farFutureCron, farFutureCron, farFutureCron, farFutureCron,
		farFutureCron, // autocachePrediction
	)
	require.NoError(t, err)
	defer svc.Stop()

	status := svc.GetStatus()
	_, ok := status["autocache_prediction"]
	assert.True(t, ok, "GetStatus must expose autocache_prediction")
}

// TestJobService_NilAutocacheLogicASkipped verifies a nil Logic A job (no library
// URL configured) is skipped cleanly — Start succeeds and GetStatus still exposes
// the key (zero last_run) without panicking.
func TestJobService_NilAutocacheLogicASkipped(t *testing.T) {
	svc := NewJobService(nil, nil, nil, nil, nil, nil, nil, nil, nil, logger.Default())

	err := svc.Start(
		farFutureCron, farFutureCron, farFutureCron, farFutureCron,
		farFutureCron, farFutureCron, farFutureCron, farFutureCron,
		farFutureCron, // autocachePrediction (nil job → skipped)
	)
	require.NoError(t, err)
	defer svc.Stop()

	status := svc.GetStatus()
	_, ok := status["autocache_logic_a"]
	assert.True(t, ok, "GetStatus exposes autocache_logic_a even when the job is nil")
}
