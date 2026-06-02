package domain

import (
	"context"
	"time"
)

// EventStore is the write port for clickstream persistence. The Postgres
// implementation backs v1; a ClickHouse implementation can replace it
// later without touching the ingestion path (see spec §3.2, deferred).
type EventStore interface {
	InsertBatch(ctx context.Context, events []Event) error
	UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error
}
