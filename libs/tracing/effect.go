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
	EffectKind string // "egress" for outbound HTTP
	Host       string // target host
	Provider   string // optional provider tag (streaming/scraper path, D-02)
	Target     string // concrete target (host for egress)
	Status     int    // HTTP status code

	// Measures.
	BytesIn    int // response body bytes (counted on read/Close)
	BytesOut   int // request body bytes
	DurationMS int // wall-clock duration of the request
	Requests   int // requests this row represents (1 for a single call)
}

// EffectSink receives recorded effects. Implementations MUST be non-blocking
// (the recording RoundTripper calls Record on the request hot path — D-10). The
// Producer is the production sink; tests use a capturing fake.
type EffectSink interface {
	Record(Effect)
}
