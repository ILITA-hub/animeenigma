// Package gormtrace adds OpenTelemetry spans to GORM queries. It lives in its
// own sub-package so services that don't use GORM (Redis-only: gateway,
// watch-together) never pull the GORM dependency into their binaries.
package gormtrace

import (
	"gorm.io/gorm"
	otelgorm "gorm.io/plugin/opentelemetry/tracing"
)

// InstrumentGORM registers the OTel tracing plugin on db. Metrics are disabled
// (Prometheus already covers DB pool stats via libs/metrics). Call once after
// database.New(), before serving traffic:
//
//	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
//	    log.Warnw("gorm tracing disabled", "error", err)
//	}
func InstrumentGORM(db *gorm.DB) error {
	return db.Use(otelgorm.NewPlugin(otelgorm.WithoutMetrics()))
}
