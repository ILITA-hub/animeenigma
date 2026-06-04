package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

// DualWriteStore fans every write to BOTH a primary and a secondary
// domain.EventStore. The primary is authoritative (Postgres, the source of
// truth) — its error is returned verbatim. The secondary (ClickHouse) is
// best-effort: any secondary error is logged and SWALLOWED so a ClickHouse
// write failure can NEVER fail the Postgres write (T-01-07; RESEARCH §Migration
// step 2). This is the reversible-migration seam — flipping the backend to
// "clickhouse" later drops the wrapper entirely.
type DualWriteStore struct {
	primary   domain.EventStore
	secondary domain.EventStore
	log       *logger.Logger
}

// Compile-time assertion that DualWriteStore satisfies the swap seam.
var _ domain.EventStore = (*DualWriteStore)(nil)

// NewDualWriteStore wraps a primary (authoritative) and secondary (best-effort)
// EventStore. log may be nil (failures are then silently swallowed), though a
// real logger is expected so secondary failures are observable.
func NewDualWriteStore(primary, secondary domain.EventStore, log *logger.Logger) *DualWriteStore {
	return &DualWriteStore{primary: primary, secondary: secondary, log: log}
}

// InsertBatch writes to the primary first and returns its error verbatim
// (Postgres is the source of truth). The secondary write is then ALWAYS
// attempted and its error is logged + swallowed — a CH failure must not
// propagate. Returns nil whenever the primary succeeded, regardless of the
// secondary outcome.
func (s *DualWriteStore) InsertBatch(ctx context.Context, events []domain.Event) error {
	if err := s.primary.InsertBatch(ctx, events); err != nil {
		return err
	}
	if err := s.secondary.InsertBatch(ctx, events); err != nil {
		s.logSecondaryFailure("InsertBatch", err, "events", len(events))
	}
	return nil
}

// UpsertIdentity follows the same primary-authoritative / secondary-best-effort
// pattern as InsertBatch.
func (s *DualWriteStore) UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error {
	if err := s.primary.UpsertIdentity(ctx, anonymousID, userID, ts); err != nil {
		return err
	}
	if err := s.secondary.UpsertIdentity(ctx, anonymousID, userID, ts); err != nil {
		s.logSecondaryFailure("UpsertIdentity", err, "anonymous_id", anonymousID)
	}
	return nil
}

// logSecondaryFailure records a best-effort secondary write failure without
// blocking or adding latency to the primary path.
func (s *DualWriteStore) logSecondaryFailure(op string, err error, kvs ...interface{}) {
	if s.log == nil {
		return
	}
	args := append([]interface{}{"op", op, "error", err}, kvs...)
	s.log.Warnw("dual-write: secondary (clickhouse) write failed; swallowed (postgres is authoritative)", args...)
}
