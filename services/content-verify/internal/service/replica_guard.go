package service

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
)

const (
	replicaKeyPrefix = "cv:replica:"
	replicaTTL       = 30 * time.Second
	replicaRefresh   = 10 * time.Second
)

// ReplicaGuard makes content-verify's single-replica invariant loud instead of
// silent. The worker throttle (one probe per unit, a bounded number per
// provider) is enforced entirely by the Engine's IN-PROCESS leases — there is
// no distributed lock — so a second REPLICA would happily probe the same units
// concurrently (see the Worker doc comment). The k8s deployment pins
// replicas:1, but nothing ENFORCES that; a stray scale-up would silently double
// every probe. This guard heartbeats a TTL'd Redis key per instance and, each
// refresh, counts the live siblings into content_verify_replicas_detected and
// WARNs when the count exceeds 1 — turning an invisible landmine into an
// alertable signal. It is observe-only: it never changes behavior.
type ReplicaGuard struct {
	rdb *redis.Client
	id  string
	log *logger.Logger
}

// NewReplicaGuard identifies this instance by hostname (unique per k8s pod),
// falling back to the pid if the hostname is unavailable.
func NewReplicaGuard(rdb *redis.Client, log *logger.Logger) *ReplicaGuard {
	id, err := os.Hostname()
	if err != nil || id == "" {
		id = "pid-" + strconv.Itoa(os.Getpid())
	}
	return &ReplicaGuard{rdb: rdb, id: id, log: log}
}

// Start heartbeats immediately, then every replicaRefresh until ctx is done.
// Nil-safe (a nil guard or nil client is a no-op). On shutdown it best-effort
// deregisters so a clean restart doesn't briefly look like two replicas.
func (g *ReplicaGuard) Start(ctx context.Context) {
	if g == nil || g.rdb == nil {
		return
	}
	go func() {
		g.refresh(ctx)
		t := time.NewTicker(replicaRefresh)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				dctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = g.rdb.Del(dctx, replicaKeyPrefix+g.id).Err()
				cancel()
				return
			case <-t.C:
				g.refresh(ctx)
			}
		}
	}()
}

func (g *ReplicaGuard) refresh(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := g.rdb.Set(rctx, replicaKeyPrefix+g.id, "1", replicaTTL).Err(); err != nil {
		g.log.Warnw("replica heartbeat failed", "error", err)
		return
	}
	n, err := g.countSiblings(rctx)
	if err != nil {
		g.log.Warnw("replica scan failed", "error", err)
		return
	}
	cvmetrics.ReplicasDetected.Set(float64(n))
	if n > 1 {
		g.log.Warnw("content-verify running >1 replica — units WILL be double-probed; k8s must stay replicas:1 (CV_WORKERS scales concurrency, not replica count)",
			"replicas", n, "instance", g.id)
	}
}

// countSiblings counts live cv:replica:* keys via SCAN (bounded key space).
func (g *ReplicaGuard) countSiblings(ctx context.Context) (int, error) {
	var n int
	var cursor uint64
	for {
		keys, cur, err := g.rdb.Scan(ctx, cursor, replicaKeyPrefix+"*", 100).Result()
		if err != nil {
			return 0, err
		}
		n += len(keys)
		cursor = cur
		if cursor == 0 {
			return n, nil
		}
	}
}
