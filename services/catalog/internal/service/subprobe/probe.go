package subprobe

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
)

// Pinger is one provider's reachability check. Satisfied by *jimaku.Client and
// *opensubtitles.Client (both expose Ping(ctx) (time.Duration, error)).
type Pinger interface {
	Ping(ctx context.Context) (time.Duration, error)
}

// Probe pings each configured provider on demand (driven by the scheduler-fired
// /internal/subtitle-probe/run endpoint), records verdicts in the store, and
// emits the probe_subtitle_* gauges.
type Probe struct {
	pingers         map[string]Pinger
	store           *Store
	degradedLatency time.Duration
	timeout         time.Duration
	log             *logger.Logger
	now             func() time.Time
}

// New builds a Probe. pingers holds only the CONFIGURED providers (unconfigured
// ones are omitted by the caller so they never show a permanent "down").
func New(store *Store, pingers map[string]Pinger, degradedLatency, timeout time.Duration, log *logger.Logger) *Probe {
	return &Probe{
		pingers:         pingers,
		store:           store,
		degradedLatency: degradedLatency,
		timeout:         timeout,
		log:             log,
		now:             time.Now,
	}
}

// RunOnce probes every configured provider once. Each provider is isolated by a
// per-provider timeout + panic-recover so one slow/broken provider can neither
// hang nor abort the others. Verdicts are recorded in the store and emitted as
// gauges; ProbeSubtitleProviderUp is Reset() first so a dropped provider does
// not leave a stale series.
func (p *Probe) RunOnce(ctx context.Context) {
	metrics.ProbeSubtitleProviderUp.Reset()
	for provider, pinger := range p.pingers {
		h := p.probeOne(ctx, provider, pinger)
		p.store.Record(provider, h)
		metrics.ProbeSubtitleProviderUp.WithLabelValues(provider).Set(gaugeValue(h.Status))
		metrics.ProbeSubtitleLatencySeconds.WithLabelValues(provider).Set(float64(h.LatencyMS) / 1000)
	}
	metrics.ProbeSubtitleLastRun.Set(float64(p.now().Unix()))
}

func (p *Probe) probeOne(ctx context.Context, provider string, pinger Pinger) (h Health) {
	defer func() {
		if r := recover(); r != nil {
			if p.log != nil {
				p.log.Errorw("subtitle probe panicked", "provider", provider, "panic", r)
			}
			h = Health{Status: StatusDown, CheckedAt: p.now()}
		}
	}()
	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	lat, err := pinger.Ping(cctx)
	if err != nil && p.log != nil {
		p.log.Warnw("subtitle probe ping failed", "provider", provider, "error", err, "latency_ms", lat.Milliseconds())
	}
	return Health{Status: classify(lat, err, p.degradedLatency), LatencyMS: lat.Milliseconds(), CheckedAt: p.now()}
}

// classify maps a ping result to a verdict: a transient rate-limit is degraded,
// any other error is down, a slow success is degraded, a fast success is up.
func classify(lat time.Duration, err error, degradedLatency time.Duration) Status {
	if err != nil {
		if errors.Is(err, opensubtitles.ErrRateLimited) {
			return StatusDegraded
		}
		return StatusDown
	}
	if lat > degradedLatency {
		return StatusDegraded
	}
	return StatusUp
}

func gaugeValue(s Status) float64 {
	switch s {
	case StatusUp:
		return 1
	case StatusDegraded:
		return 0.5
	default:
		return 0
	}
}
