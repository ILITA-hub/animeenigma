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
		for range ticker.C {
			stats := db.Stats()
			DBPoolOpenConnections.Set(float64(stats.OpenConnections))
			DBPoolIdleConnections.Set(float64(stats.Idle))
			DBPoolWaitTotal.Add(float64(stats.WaitCount))
			DBPoolWaitDurationTotal.Add(stats.WaitDuration.Seconds())
		}
	}()
}
