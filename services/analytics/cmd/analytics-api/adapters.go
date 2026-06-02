package main

import (
	"context"
	"time"

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

// repoEraser adapts repo erase funcs to handler.Eraser.
type repoEraser struct{ db *database.DB }

func (r repoEraser) EraseByUserID(ctx context.Context, id string) error {
	return repo.EraseByUserID(ctx, r.db.DB, id)
}
func (r repoEraser) EraseByAnonymousID(ctx context.Context, id string) error {
	return repo.EraseByAnonymousID(ctx, r.db.DB, id)
}

// repoPurger adapts repo.PurgeOlderThan to job.Purger.
type repoPurger struct{ db *database.DB }

func (r repoPurger) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	return repo.PurgeOlderThan(ctx, r.db.DB, cutoff)
}
