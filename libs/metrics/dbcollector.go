package metrics

import (
	"database/sql"
	"time"
)

// StartDBPoolCollector starts a background goroutine that periodically reads
// sql.DBStats and updates the DB pool Prometheus gauges.
func StartDBPoolCollector(db *sql.DB, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// sql.DBStats.WaitCount / WaitDuration are CUMULATIVE process-lifetime
		// totals. The old code Add()'d the running total every tick, so the
		// Prometheus counter ballooned quadratically and rate() was meaningless
		// (audit #22). Track the previous reading and Add only the per-tick
		// delta, so the counter reflects real increments.
		var prevWaitCount int64
		var prevWaitDuration time.Duration
		for range ticker.C {
			stats := db.Stats()
			DBPoolOpenConnections.Set(float64(stats.OpenConnections))
			DBPoolIdleConnections.Set(float64(stats.Idle))
			if d := stats.WaitCount - prevWaitCount; d > 0 {
				DBPoolWaitTotal.Add(float64(d))
			}
			if d := stats.WaitDuration - prevWaitDuration; d > 0 {
				DBPoolWaitDurationTotal.Add(d.Seconds())
			}
			prevWaitCount = stats.WaitCount
			prevWaitDuration = stats.WaitDuration
		}
	}()
}
