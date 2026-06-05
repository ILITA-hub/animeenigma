package tracing

import (
	"context"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/baggage"
)

// baggageKeyUserID is the baggage member key that must NEVER ride the outbound
// wire (T-02-PII). user_id rides the private non-propagated ctx value
// (WithUserID) only; this constant exists solely so the defense-in-depth strip
// below can detect and remove a member a careless future caller might have
// added.
const baggageKeyUserID = "user_id"

// stripWireBaggagePII returns ctx with any PII baggage member removed. The
// recording transport calls this on every outbound request BEFORE the base
// RoundTrip runs, so otelhttp's outbound baggage propagation can never inject a
// user_id member onto a 3rd-party-bound request — even if one was accidentally
// seeded upstream. origin/operation are preserved. This is defense-in-depth: by
// design user_id never enters baggage (it rides WithUserID's private ctx value),
// but this guarantees it regardless of caller behavior (T-02-PII, the sharpest
// RESEARCH §Security finding).
func stripWireBaggagePII(ctx context.Context) context.Context {
	bg := baggage.FromContext(ctx)
	if bg.Member(baggageKeyUserID).Key() == "" {
		return ctx // no user_id member present — nothing to strip
	}
	return baggage.ContextWithBaggage(ctx, bg.DeleteMember(baggageKeyUserID))
}

// globalSink is an optional process-wide EffectSink. When set (via SetGlobalSink)
// WrapTransport composes the recording transport so EVERY shared HTTP client
// records egress effects without per-call wiring. nil → bare otelhttp (today's
// behavior, zero overhead).
var globalSink atomic.Pointer[EffectSink]

// SetGlobalSink installs (or clears, with nil) the process-wide effect sink that
// WrapTransport composes. Call once at BE service boot after the Producer is
// started.
func SetGlobalSink(sink EffectSink) {
	if sink == nil {
		globalSink.Store(nil)
		return
	}
	globalSink.Store(&sink)
}

// WrapTransport wraps an existing RoundTripper so outbound requests get a
// client span and the active trace context is injected (W3C traceparent) using
// the global propagator. Pass nil to wrap http.DefaultTransport. When a
// process-global effect sink is set (SetGlobalSink) the result also records one
// egress effect per outbound request. Use this to keep a custom transport's
// dialer/pool settings while adding propagation + recording:
//
//	t.Transport = tracing.WrapTransport(t.Transport)
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	traced := otelhttp.NewTransport(base)
	if sp := globalSink.Load(); sp != nil {
		return WrapRecording(traced, *sp)
	}
	return traced
}

// WrapRecording wraps base so each outbound request emits exactly one Effect to
// rec: host, status, request/response bytes, and duration. It NEVER buffers the
// whole response body — it wraps resp.Body in a byte-counting ReadCloser and
// emits the effect on the FIRST of (full body read to EOF) OR (Close),
// idempotently (CR-01). Emitting on the terminal read means a caller that reads
// a 2xx body to completion but forgets to Close still records its egress row;
// the Close path still fires for callers that close early without a full read.
// The emit is guarded so read-to-EOF-then-Close records exactly once, never
// twice. On a transport error the effect is emitted immediately (no body to
// wrap). rec must be non-blocking. If rec is nil, base is returned unchanged.
func WrapRecording(base http.RoundTripper, rec EffectSink) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if rec == nil {
		return base
	}
	return &recordingTransport{base: base, rec: rec}
}

// NewHTTPClient returns an *http.Client whose transport propagates trace
// context (and records egress effects when a global sink is set). base may be
// nil (http.DefaultTransport). When tracing is disabled the global provider is
// a no-op and no header is injected.
func NewHTTPClient(base http.RoundTripper, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: WrapTransport(base)}
}

// recordingTransport measures status/bytes/duration for one outbound request
// and emits a single Effect on body Close (or immediately on error).
type recordingTransport struct {
	base http.RoundTripper
	rec  EffectSink
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Attribution: origin rides wire baggage; user_id + provider ride private
	// ctx values (never baggage — T-02-PII). Read attribution BEFORE stripping
	// so the recorder still captures user_id from the private ctx value.
	ctx := req.Context()
	origin, _ := ReadBaggage(ctx)
	userID := UserIDFromContext(ctx)
	provider := ProviderFromContext(ctx)

	// D-08: operation is now stack-frame-PRIMARY. Capture the program counters
	// synchronously here (cheap, no symbol resolution — D-11); the Producer
	// resolves them to the fine "<pkg>.<Func>" label on its async goroutine,
	// falling back to the baggage operation then the origin name when no service
	// frame is present (Operation.Resolve). user_id never enters baggage.
	op := CaptureOperationPCs(ctx)

	// Defense-in-depth (T-02-PII): strip any user_id baggage member from the
	// request context BEFORE the base RoundTrip runs, so otelhttp's outbound
	// baggage propagation cannot inject user_id onto this 3rd-party-bound request.
	// By design user_id is never in baggage; this guarantees it regardless of
	// caller mistakes. origin/operation are preserved.
	if stripped := stripWireBaggagePII(ctx); stripped != ctx {
		req = req.WithContext(stripped)
	}

	var bytesOut int
	if req.ContentLength > 0 {
		bytesOut = int(req.ContentLength)
	}

	host := req.URL.Host

	resp, err := t.base.RoundTrip(req)

	build := func(status, bytesIn int) Effect {
		return Effect{
			Origin:     origin,
			UserID:     userID,
			EffectKind: "egress",
			Host:       host,
			Provider:   provider,
			Target:     host,
			// TargetKind left empty → post() labels egress rows "host".
			Status:     status,
			BytesIn:    bytesIn,
			BytesOut:   bytesOut,
			DurationMS: int(time.Since(start).Milliseconds()),
			Requests:   1,
		}.WithOperationPCs(op)
	}

	if err != nil {
		// Error path: no body to read — record immediately with status 0.
		t.rec.Record(build(0, 0))
		return resp, err
	}

	status := resp.StatusCode
	// Wrap the body so BytesIn is counted as the caller reads, and the effect
	// emits exactly once on the first of (read-to-EOF) or (Close). We never
	// buffer the body here.
	resp.Body = &countingBody{
		rc: resp.Body,
		emit: func(n int) {
			t.rec.Record(build(status, n))
		},
	}
	return resp, nil
}

// countingBody wraps a response body, counting bytes read and firing emit
// exactly once (idempotent, CR-01) with the running total on the FIRST of:
//   - a terminal read (the underlying Read returns a non-nil error — io.EOF on a
//     fully-consumed body, or any hard error), so a read-to-EOF-without-Close
//     caller still records its egress row and never leaks the effect; or
//   - Close, so a caller that closes early (partial / no read) still records.
//
// The fired flag guarantees the common read-to-EOF-then-Close path emits
// exactly once, not twice.
type countingBody struct {
	rc    io.ReadCloser
	n     int
	fired bool
	emit  func(n int)
}

// fireOnce emits the effect at most once with the current byte count.
func (c *countingBody) fireOnce() {
	if !c.fired {
		c.fired = true
		if c.emit != nil {
			c.emit(c.n)
		}
	}
}

func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	c.n += n
	// A non-nil err marks a terminal read (io.EOF on full consumption, or a
	// hard read error). Emit here so a fully-read-but-never-Closed body still
	// records exactly one effect. Close (if it later runs) is a no-op via the
	// fired guard.
	if err != nil {
		c.fireOnce()
	}
	return n, err
}

func (c *countingBody) Close() error {
	err := c.rc.Close()
	c.fireOnce()
	return err
}
