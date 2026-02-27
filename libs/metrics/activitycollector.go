package metrics

import (
	"database/sql"
	"time"
)

// StartActivityMetricsCollector starts a background goroutine that periodically
// queries the watch_progress table and updates activity-related Prometheus gauges.
// Intended to run in the player service.
func StartActivityMetricsCollector(db *sql.DB, interval time.Duration) {
	// Run immediately on start, then on interval
	go func() {
		collectActivityMetrics(db)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			collectActivityMetrics(db)
		}
	}()
}

func collectActivityMetrics(db *sql.DB) {
	var count float64

	// Active users in last 24 hours
	if err := db.QueryRow("SELECT COUNT(DISTINCT user_id) FROM watch_progress WHERE last_watched_at > NOW() - INTERVAL '1 day'").Scan(&count); err == nil {
		UsersActive.WithLabelValues("24h").Set(count)
	}

	// Active users in last 7 days
	if err := db.QueryRow("SELECT COUNT(DISTINCT user_id) FROM watch_progress WHERE last_watched_at > NOW() - INTERVAL '7 days'").Scan(&count); err == nil {
		UsersActive.WithLabelValues("7d").Set(count)
	}

	// Active users in last 30 days
	if err := db.QueryRow("SELECT COUNT(DISTINCT user_id) FROM watch_progress WHERE last_watched_at > NOW() - INTERVAL '30 days'").Scan(&count); err == nil {
		UsersActive.WithLabelValues("30d").Set(count)
	}

	// Users watching right now (last_watched_at within 5 minutes)
	if err := db.QueryRow("SELECT COUNT(DISTINCT user_id) FROM watch_progress WHERE last_watched_at > NOW() - INTERVAL '5 minutes'").Scan(&count); err == nil {
		UsersWatchingNow.Set(count)
	}
}
