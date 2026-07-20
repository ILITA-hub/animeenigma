package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// These live in the catalog service, NOT in libs/metrics. A plain (non-Vec)
// promauto metric placed in libs/metrics registers at import time in every
// service that imports the package and exports a permanent 0 from each,
// creating impostor series. catalog is the only emitter here, so the metrics
// belong to catalog.
var (
	// englishDubBackfillTotal counts probe outcomes:
	//	dub      — the title has an EN dub
	//	nodub    — probed, no dub
	//	stamped  — inconclusive (unreachable / non-200 / empty list); only the
	//	           timestamp moved, so the loop rotates
	//	error    — the candidate query itself failed
	//	shed     — skipped, governor reported Elevated+
	englishDubBackfillTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "catalog_english_dub_backfill_total",
		Help: "EN-dub backfill probe outcomes by result.",
	}, []string{"result"})

	// englishDubPromotedTotal counts titles promoted from an audio-verified
	// content-verify verdict, which outranks provider metadata.
	englishDubPromotedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "catalog_english_dub_promoted_total",
		Help: "Titles promoted to has_english_dub from a verified content-verify audio verdict.",
	})

	// englishDubUnchecked is the catch-up gauge: EN-sourced titles that have
	// never had an EN-dub verdict established.
	englishDubUnchecked = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "catalog_english_dub_unchecked",
		Help: "Titles with has_english=true whose EN-dub verdict has never been established.",
	})
)
