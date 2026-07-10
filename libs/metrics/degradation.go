package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Degradation-shedding metrics shared by the CONSUMER services
// (graceful-degradation Phase 3,
// docs/superpowers/specs/2026-07-10-graceful-degradation-design.md).
// The governor's own singleton metrics (ae_degradation_level, reason gauges,
// transition/failure counters) live in services/governor/internal/govmetrics —
// plain (non-Vec) metrics in this shared package would auto-register and
// export as a permanent 0 in EVERY importing binary, polluting the dashboard
// with impostor series. Vecs are safe here: they emit no series until used.

// DegradationShed marks the shed intensity each heavy subsystem is currently
// applying (0 = admitting normally, 1 = new-work admission paused at Elevated,
// 2 = refusing at Critical). Emitted by the consumers themselves (library,
// stealth-scraper), not the governor — it reflects what is ACTUALLY shed.
var DegradationShed = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "ae_degradation_shed",
		Help: "Shed intensity currently applied by a heavy subsystem (0 none, 1 paused admission, 2 refusing).",
	},
	[]string{"subsystem"},
)
