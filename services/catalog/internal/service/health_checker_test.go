package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeAePinger struct{ err error }

func (f fakeAePinger) Ping(ctx context.Context) error { return f.err }

func TestCheckAe_UpWhenPingOK(t *testing.T) {
	h := NewPlayerHealthChecker(nil, 0, logger.Default(), fakeAePinger{err: nil})
	// checkProvider(providerAe, aeStage, h.checkAe) is the pipeline that sets the
	// metric; calling it directly mirrors what checkAll does for the ae probe.
	h.checkProvider(providerAe, aeStage, h.checkAe)
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues("ae", "liveness")); got != 1 {
		t.Errorf("provider_health_up{ae,liveness} = %v, want 1", got)
	}
}

func TestCheckAe_DownWhenPingErrors(t *testing.T) {
	h := NewPlayerHealthChecker(nil, 0, logger.Default(), fakeAePinger{err: errors.New("library down")})
	h.checkProvider(providerAe, aeStage, h.checkAe)
	if got := testutil.ToFloat64(metrics.ProviderHealthUp.WithLabelValues("ae", "liveness")); got != 0 {
		t.Errorf("provider_health_up{ae,liveness} = %v, want 0", got)
	}
}
