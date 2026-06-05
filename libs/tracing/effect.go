package tracing

// Effect is one backend effect row in the activity register — currently the
// egress shape (one row per outbound HTTP request). It mirrors the effect
// dimensions/measures the analytics service persists (services/analytics
// domain.Event + the ClickHouse effect columns). The recording RoundTripper
// produces these; the Producer ships them to /internal/effects.
type Effect struct {
	// Dimensions.
	Origin     string // who caused it (seeded "api" by SeedMiddleware)
	Operation  string // coarse op label, e.g. "catalog GET /api/anime/{id}"
	UserID     string // private-ctx user_id (never on wire baggage)
	EffectKind string // "egress" | "db_write" | "db_read" | "cache" (effect kind)
	Host       string // target host
	Provider   string // optional provider tag (streaming/scraper path, D-02)
	Target     string // concrete target (host for egress; table/key-class for db/cache)
	TargetKind string // "host" | "table" | "key_class" — how Target is interpreted
	AnimeID    string // optional anime UUID this effect concerns (db/cache rows)
	Status     int    // HTTP status code (egress); 0 for non-egress

	// Measures.
	BytesIn    int // response body bytes (counted on read/Close)
	BytesOut   int // request body bytes
	DurationMS int // wall-clock duration of the request
	Requests   int // requests this row represents (1 for a single call)
	Rows       int // GORM RowsAffected for db_write/db_read rows (0 otherwise)

	// op carries the program counters captured SYNCHRONOUSLY at the record
	// point (D-11). When set, the Producer resolves it to the fine `operation`
	// label on its async goroutine via op.Resolve() — keeping CallersFrames
	// symbol resolution off the request hot path. When op is the zero value the
	// pre-set Operation field (if any) is used as-is.
	op Operation
}

// WithOperationPCs returns a copy of e carrying the captured operation PCs so
// the Producer can resolve the fine `operation` label asynchronously (D-11).
// Hooks (egress RoundTrip, GORM callbacks, cache observers) call this at record
// time instead of resolving inline.
func (e Effect) WithOperationPCs(op Operation) Effect {
	e.op = op
	return e
}

// resolvedOperation returns the fine operation label for this effect: the async
// resolve of the captured PCs when present, else the already-set Operation
// dimension. Called on the Producer side only.
func (e Effect) resolvedOperation() string {
	if len(e.op.pcs) > 0 || e.op.ctx != nil {
		if op := e.op.Resolve(); op != "" {
			return op
		}
	}
	return e.Operation
}

// EffectSink receives recorded effects. Implementations MUST be non-blocking
// (the recording RoundTripper calls Record on the request hot path — D-10). The
// Producer is the production sink; tests use a capturing fake.
type EffectSink interface {
	Record(Effect)
}
