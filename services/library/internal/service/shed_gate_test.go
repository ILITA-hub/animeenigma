package service

import (
	"testing"

	gometrics "github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestStaggeredLibraryActuatorOrder(t *testing.T) {
	encode := newEncodeLimiter(2, nil, nil)
	download := newDownloadLimiter(2, nil)

	cases := []struct {
		score          float64
		storyboardShed bool
		encodeCap      int
		downloadCap    int
	}{
		{0.19, false, 2, 2},
		{0.30, true, 2, 2},
		{0.41, true, 1, 2},
		{0.56, true, 1, 1},
		{0.80, true, 0, 1},
		{0.90, true, 0, 0},
	}

	checker := &fakeLevel{}
	storyboard := newShedGate("test_storyboard", storyboardPauseScore, nil)
	storyboard.set(checker)
	for _, tc := range cases {
		checker.setScore(tc.score)
		if got := storyboard.shed(); got != tc.storyboardShed {
			t.Errorf("score %.2f: storyboard shed=%v, want %v", tc.score, got, tc.storyboardShed)
		}
		if got := encode.capFor(tc.score, 0); got != tc.encodeCap {
			t.Errorf("score %.2f: encode cap=%d, want %d", tc.score, got, tc.encodeCap)
		}
		if got := download.capFor(tc.score, 0); got != tc.downloadCap {
			t.Errorf("score %.2f: download cap=%d, want %d", tc.score, got, tc.downloadCap)
		}
	}
}

func TestGradedLimiterCriticalBackstop(t *testing.T) {
	lim := newDownloadLimiter(3, nil)
	if got := lim.capFor(0, 2); got != 0 {
		t.Fatalf("level 2 with score 0: cap=%d, want 0", got)
	}
}

func TestActuatorsPublishNormalStateOnFirstCheck(t *testing.T) {
	checker := &fakeLevel{}

	gate := newShedGate("test_gate_initial", storyboardPauseScore, nil)
	gate.set(checker)
	gometrics.DegradationShed.WithLabelValues("test_gate_initial").Set(2)
	if gate.shed() {
		t.Fatal("normal score unexpectedly shed gate")
	}
	if got := testutil.ToFloat64(gometrics.DegradationShed.WithLabelValues("test_gate_initial")); got != 0 {
		t.Fatalf("gate initial gauge=%v, want 0", got)
	}

	lim := newGradedLimiter("test_limiter_initial", 2, 0.4, 0.8, nil, nil)
	lim.set(checker)
	gometrics.DegradationShed.WithLabelValues("test_limiter_initial").Set(2)
	if got := lim.currentCap(); got != 2 {
		t.Fatalf("normal limiter cap=%d, want 2", got)
	}
	if got := testutil.ToFloat64(gometrics.DegradationShed.WithLabelValues("test_limiter_initial")); got != 0 {
		t.Fatalf("limiter initial gauge=%v, want 0", got)
	}
}
