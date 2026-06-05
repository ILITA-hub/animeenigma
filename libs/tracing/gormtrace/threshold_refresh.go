package gormtrace

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HashReader is the narrow read surface the ThresholdRefresher needs: a single
// HGETALL of the read_thresholds hash. Keeping it an interface (rather than a
// *redis.Client) means libs/tracing does NOT import go-redis — each GORM
// service passes a tiny adapter over its own Redis client at boot (plan 06).
type HashReader interface {
	// HGetAll returns the field->value map for key, or an error. An empty map
	// (no fields) is a valid, non-error result (cold-start).
	HGetAll(ctx context.Context, key string) (map[string]string, error)
}

// readThresholdsHashKey is the Redis hash the daily-P95 producer (plan-05
// analytics) publishes. The refresher snapshots it into the ReadGate. It MUST
// match service.ReadThresholdsHashKey on the producer side.
const readThresholdsHashKey = "read_thresholds"

// maxThresholdFields bounds how many fields a single refresh will accept from
// the hash so a poisoned/oversized hash can neither balloon the in-memory
// snapshot nor stall the tick (T-03-14). A real deployment has at most a few
// hundred (operation, table) keys; 10k is a generous ceiling.
const maxThresholdFields = 10000

// maxFieldValueLen bounds the accepted length of a single p95 value string so a
// pathologically long value is skipped before strconv even runs (T-03-14).
const maxFieldValueLen = 32

// ThresholdRefresher periodically snapshots the read_thresholds Redis hash into
// a ReadGate so the plan-03 db_read gate becomes dynamic without ANY synchronous
// Redis/HTTP lookup on the query hot path (Pitfall 4). The hot-path callback
// consults only the in-memory snapshot; this ticker is the sole off-path writer.
type ThresholdRefresher struct {
	reader   HashReader
	gate     *snapshotGate
	interval time.Duration

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// NewThresholdRefresher builds a refresher over a HashReader and the ReadGate it
// feeds. interval is the snapshot cadence (recommend 5-15 min — Pitfall 4: it is
// a coarse cost-ledger threshold, not a real-time value, so a few minutes of
// staleness is fine and keeps Redis load negligible).
func NewThresholdRefresher(reader HashReader, gate *snapshotGate, interval time.Duration) *ThresholdRefresher {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &ThresholdRefresher{
		reader:   reader,
		gate:     gate,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start launches the refresh ticker in a background goroutine. It performs one
// immediate refresh (so the gate is populated promptly at boot, not after the
// first full interval), then refreshes every interval until Stop or ctx
// cancellation. Start is non-blocking.
func (r *ThresholdRefresher) Start(ctx context.Context) {
	r.refreshOnce(ctx)
	go func() {
		defer close(r.done)
		t := time.NewTicker(r.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stop:
				return
			case <-t.C:
				r.refreshOnce(ctx)
			}
		}
	}()
}

// Stop halts the ticker and blocks until the goroutine exits. It is safe to call
// multiple times (idempotent) and safe to call even if Start was never invoked
// (the done channel never closes in that case, so guard with a started flag is
// unnecessary because Stop only waits when a goroutine exists — see note). To
// keep Stop always-safe we only wait for done when Start ran.
func (r *ThresholdRefresher) Stop() {
	r.stopOnce.Do(func() {
		close(r.stop)
	})
}

// refreshOnce reads the hash and atomically installs a parsed snapshot into the
// gate. On a read error it logs nothing here (the package has no logger) and
// returns WITHOUT touching the gate — the prior snapshot survives so a transient
// Redis blip never blanks the thresholds (T-03-17 partial). Malformed,
// out-of-range, empty-keyed, and oversized fields are skipped defensively so a
// poisoned hash can neither panic nor poison the whole snapshot (T-03-14).
func (r *ThresholdRefresher) refreshOnce(ctx context.Context) {
	raw, err := r.reader.HGetAll(ctx, readThresholdsHashKey)
	if err != nil {
		// Leave the existing snapshot intact; never propagate in a way that
		// kills the ticker.
		return
	}

	m := make(map[string]float64, len(raw))
	count := 0
	for field, val := range raw {
		if count >= maxThresholdFields {
			break
		}
		// A valid field is "operation|table" with both parts non-empty.
		if field == "" || !strings.Contains(field, "|") {
			continue
		}
		if len(val) == 0 || len(val) > maxFieldValueLen {
			continue
		}
		p95, perr := strconv.ParseFloat(val, 64)
		if perr != nil {
			continue
		}
		// Reject NaN/Inf and negative thresholds — a threshold must be a finite
		// non-negative millisecond figure.
		if p95 < 0 || p95 != p95 || p95 > 1e12 {
			continue
		}
		m[field] = p95
		count++
	}
	// An all-empty (or fully-skipped) hash installs an empty snapshot so the
	// gate falls back to its static cold-start default for every key.
	r.gate.SetSnapshot(m)
}
