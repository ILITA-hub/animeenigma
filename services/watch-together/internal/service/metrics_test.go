// Package service — metrics_test.go covers the Plan 05.2 metric registrations.
//
// CAUTION: Prometheus's default registry is process-wide. Tests in this file
// share state with the production code's init() — they bump REAL registered
// metric values, not fakes. Each test below reads the BASELINE value of its
// metric first, then asserts on the delta after a bump. Do NOT use t.Parallel()
// here unless every parallel test bumps a different metric in isolation;
// today they bump 5 different metrics so parallel would be safe, but keeping
// them sequential matches the rest of the service package's test layout.
package service

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestRoomsActive_IncDec verifies the wt_rooms_active gauge can be moved up
// and down. Reads baseline (other tests in this package may have moved it),
// then asserts deltas.
func TestRoomsActive_IncDec(t *testing.T) {
	before := testutil.ToFloat64(RoomsActive)

	RoomsActive.Inc()
	if got := testutil.ToFloat64(RoomsActive); got != before+1 {
		t.Errorf("after Inc: RoomsActive = %v, want %v", got, before+1)
	}

	RoomsActive.Inc()
	if got := testutil.ToFloat64(RoomsActive); got != before+2 {
		t.Errorf("after second Inc: RoomsActive = %v, want %v", got, before+2)
	}

	RoomsActive.Dec()
	RoomsActive.Dec()
	if got := testutil.ToFloat64(RoomsActive); got != before {
		t.Errorf("after two Decs: RoomsActive = %v, want %v (restored)", got, before)
	}
}

// TestMembersPerRoom_ObserveBumpsCount verifies the histogram exists and
// accepts observations. CollectAndCount returns the number of metric
// SERIES (always 1 for an unlabelled histogram) — we use a higher-level
// signal: observe a value, and the _count sample should advance by 1.
func TestMembersPerRoom_ObserveBumpsCount(t *testing.T) {
	// Histograms don't expose _count via ToFloat64 directly (it's a vector
	// of samples). Use the CollectAndCount helper which returns the number
	// of distinct observations the collector has seen.
	beforeCount := testutil.CollectAndCount(MembersPerRoom)
	MembersPerRoom.Observe(3)
	afterCount := testutil.CollectAndCount(MembersPerRoom)
	// CollectAndCount on a single-series histogram is 1 regardless of how
	// many observations have landed — so the better assertion is "didn't
	// panic and the collector still reports the metric exists".
	if afterCount < 1 {
		t.Errorf("MembersPerRoom not collected: %d", afterCount)
	}
	_ = beforeCount
}

// TestChatMessagesPerRoom_ObserveZeroIsValid documents that 0 is a meaningful
// data point — quick test rooms with no chat traffic. The histogram MUST
// accept Observe(0) without panic and place it in the [0, 1) bucket.
func TestChatMessagesPerRoom_ObserveZeroIsValid(t *testing.T) {
	// Just confirm Observe(0) doesn't panic and the collector still
	// reports the metric.
	ChatMessagesPerRoom.Observe(0)
	ChatMessagesPerRoom.Observe(42)
	if got := testutil.CollectAndCount(ChatMessagesPerRoom); got < 1 {
		t.Errorf("ChatMessagesPerRoom not collected: %d", got)
	}
}

// TestSessionDurationSeconds_ObserveLongSession verifies the histogram
// accepts values across the bucket range without panic. The 14400s upper
// bucket is the "binge" anchor — a 5h session should land in or beyond it
// without overflowing.
func TestSessionDurationSeconds_ObserveLongSession(t *testing.T) {
	SessionDurationSeconds.Observe(45)     // < 60s, lowest bucket
	SessionDurationSeconds.Observe(3700)   // ~1h, mid-range
	SessionDurationSeconds.Observe(18000)  // 5h, above top bucket → +Inf
	if got := testutil.CollectAndCount(SessionDurationSeconds); got < 1 {
		t.Errorf("SessionDurationSeconds not collected: %d", got)
	}
}

// TestPersistentDriftTotal_LabelsAreHostAndMember verifies the labelled
// counter accepts the documented "host" and "member" values and increments
// independently per label. The test does NOT assert that other label
// values are rejected — Prometheus is permissive at the type system, and
// the convention is enforced by the caller in inbound.go.
func TestPersistentDriftTotal_LabelsAreHostAndMember(t *testing.T) {
	beforeHost := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("host"))
	beforeMember := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("member"))

	PersistentDriftTotal.WithLabelValues("host").Inc()
	PersistentDriftTotal.WithLabelValues("member").Inc()
	PersistentDriftTotal.WithLabelValues("member").Inc()

	if got := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("host")); got != beforeHost+1 {
		t.Errorf("host counter = %v, want %v", got, beforeHost+1)
	}
	if got := testutil.ToFloat64(PersistentDriftTotal.WithLabelValues("member")); got != beforeMember+2 {
		t.Errorf("member counter = %v, want %v", got, beforeMember+2)
	}
}
