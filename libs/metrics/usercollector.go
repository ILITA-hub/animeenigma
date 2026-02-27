package metrics

import (
	"database/sql"
	"time"
)

// StartUserMetricsCollector starts a background goroutine that periodically
// queries the users table and updates user-related Prometheus gauges.
// Intended to run in the auth service.
func StartUserMetricsCollector(db *sql.DB, interval time.Duration) {
	// Run immediately on start, then on interval
	go func() {
		collectUserMetrics(db)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			collectUserMetrics(db)
		}
	}()
}

func collectUserMetrics(db *sql.DB) {
	var count float64

	// Total registered users
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL").Scan(&count); err == nil {
		UsersRegisteredTotal.Set(count)
	}

	// New users in last 24 hours
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND created_at > NOW() - INTERVAL '1 day'").Scan(&count); err == nil {
		UsersNew.WithLabelValues("24h").Set(count)
	}

	// New users in last 7 days
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND created_at > NOW() - INTERVAL '7 days'").Scan(&count); err == nil {
		UsersNew.WithLabelValues("7d").Set(count)
	}

	// New users in last 30 days
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND created_at > NOW() - INTERVAL '30 days'").Scan(&count); err == nil {
		UsersNew.WithLabelValues("30d").Set(count)
	}
}
