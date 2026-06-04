package main

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/ingest"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// countingSink bumps the received counter, then enqueues.
type countingSink struct{ b *ingest.Batcher }

func (c countingSink) Enqueue(e domain.Event) bool {
	observ.EventsReceived.Inc()
	return c.b.Enqueue(e)
}

// repoEraser adapts repo erase funcs to handler.Eraser. When the ClickHouse
// backend is active (chConn != nil), GDPR erasure spans BOTH stores so
// right-to-erasure stays complete across PG + CH (T-01-11). The Postgres erase
// is authoritative (its error propagates); the CH erase is best-effort, so a CH
// blip cannot fail a user's PG erasure — but a CH error IS surfaced so it isn't
// silently dropped.
type repoEraser struct {
	db     *database.DB
	chConn driver.Conn
}

func (r repoEraser) EraseByUserID(ctx context.Context, id string) error {
	if err := repo.EraseByUserID(ctx, r.db.DB, id); err != nil {
		return err
	}
	if r.chConn != nil {
		return repo.EraseByUserIDCH(ctx, r.chConn, id)
	}
	return nil
}
func (r repoEraser) EraseByAnonymousID(ctx context.Context, id string) error {
	if err := repo.EraseByAnonymousID(ctx, r.db.DB, id); err != nil {
		return err
	}
	if r.chConn != nil {
		return repo.EraseByAnonymousIDCH(ctx, r.chConn, id)
	}
	return nil
}

// repoPurger adapts repo.PurgeOlderThan to job.Purger.
type repoPurger struct{ db *database.DB }

func (r repoPurger) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	return repo.PurgeOlderThan(ctx, r.db.DB, cutoff)
}
