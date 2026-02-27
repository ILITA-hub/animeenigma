package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// UsersRegisteredTotal is the total number of registered users (soft-deleted excluded).
	UsersRegisteredTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "users_registered_total",
			Help: "Total number of registered users",
		},
	)

	// UsersNew tracks new user registrations per time period.
	UsersNew = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "users_new",
			Help: "Number of new users registered in the given period",
		},
		[]string{"period"},
	)

	// UsersActive tracks distinct users with watch activity per time period.
	UsersActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "users_active",
			Help: "Number of active users (with watch activity) in the given period",
		},
		[]string{"period"},
	)

	// UsersWatchingNow tracks users currently watching (last_watched_at within 5 minutes).
	UsersWatchingNow = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "users_watching_now",
			Help: "Number of users currently watching anime",
		},
	)
)
