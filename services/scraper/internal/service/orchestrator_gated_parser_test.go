package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestGetStreamGated_EmitsParserMetrics verifies that attemptGatedStream
// records parser_requests_total{operation="get_stream"} when the non-gated
// (plain GetStream) fallback path runs. fakeProvider does not implement
// gatedProvider, so attemptGatedStream takes the plain GetStream branch and
// the defer ObserveParser fires with status="success" (GetStream returns
// nil stream + nil error by default).
func TestGetStreamGated_EmitsParserMetrics(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{nameVal: "p3_gated"}
	// getStreamFn is nil → GetStream returns (nil, nil) → err==nil → "success"
	o := newTestOrchestrator(t, fp)

	before := testutil.ToFloat64(
		metrics.ParserRequestsTotal.WithLabelValues("p3_gated", "get_stream", "success"),
	)

	// GetStreamGated with a plain provider: takes the p.GetStream fallback
	// branch inside attemptGatedStream. Returns (nil, false, nil) — nil stream
	// is allowed (no-content success from the fake).
	_, _, err := o.GetStreamGated(context.Background(), "pid", "eid", "sid", domain.CategorySub, "", false)
	if err != nil {
		t.Fatalf("GetStreamGated err = %v", err)
	}

	after := testutil.ToFloat64(
		metrics.ParserRequestsTotal.WithLabelValues("p3_gated", "get_stream", "success"),
	)
	if after != before+1 {
		t.Errorf(
			"parser_requests_total{p3_gated,get_stream,success}: got %v, want %v",
			after, before+1,
		)
	}
}
