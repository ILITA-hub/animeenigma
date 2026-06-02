// Package ingest buffers clickstream events in memory and bulk-writes them
// to the EventStore. Best-effort durability: a full buffer drops events
// (analytics is not billing data — see spec §3.1). Redis Streams is the
// documented upgrade path.
package ingest

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type Config struct {
	MaxBatch      int           // flush when buffer reaches this many rows
	FlushInterval time.Duration // flush at least this often
	BufferSize    int           // channel capacity; full channel drops events
}

// Batcher accepts events via Enqueue and flushes them to the store.
type Batcher struct {
	store  domain.EventStore
	cfg    Config
	ch     chan domain.Event
	stop   chan struct{}
	done   chan struct{}
	log    *logger.Logger
	onDrop func() // hook for metrics; nil-safe
}

func New(store domain.EventStore, cfg Config) *Batcher {
	if cfg.MaxBatch <= 0 {
		cfg.MaxBatch = 500
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10000
	}
	return &Batcher{
		store: store, cfg: cfg,
		ch:   make(chan domain.Event, cfg.BufferSize),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// WithLogger and WithDropHook are optional configuration.
func (b *Batcher) WithLogger(l *logger.Logger) *Batcher { b.log = l; return b }
func (b *Batcher) WithDropHook(f func()) *Batcher        { b.onDrop = f; return b }

// Enqueue adds an event without blocking. Returns false if the buffer is
// full (event dropped).
func (b *Batcher) Enqueue(e domain.Event) bool {
	select {
	case b.ch <- e:
		return true
	default:
		if b.onDrop != nil {
			b.onDrop()
		}
		return false
	}
}

func (b *Batcher) Start() {
	go b.run()
}

func (b *Batcher) run() {
	defer close(b.done)
	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()
	buf := make([]domain.Event, 0, b.cfg.MaxBatch)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		b.flush(buf)
		buf = buf[:0]
	}

	for {
		select {
		case e := <-b.ch:
			buf = append(buf, e)
			if len(buf) >= b.cfg.MaxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.stop:
			// Drain whatever is buffered, then exit.
			for {
				select {
				case e := <-b.ch:
					buf = append(buf, e)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (b *Batcher) flush(events []domain.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batch := make([]domain.Event, len(events))
	copy(batch, events)
	if err := b.store.InsertBatch(ctx, batch); err != nil && b.log != nil {
		b.log.Errorw("analytics insert batch failed", "count", len(batch), "error", err)
	}
	// Identify events also upsert the anon→user mapping.
	for _, e := range batch {
		if e.EventType == domain.EventTypeIdentify && e.UserID != "" {
			if err := b.store.UpsertIdentity(ctx, e.AnonymousID, e.UserID, e.Timestamp); err != nil && b.log != nil {
				b.log.Errorw("analytics upsert identity failed", "anon", e.AnonymousID, "error", err)
			}
		}
	}
}

// Stop signals the flush loop to drain and exit, then waits for it.
func (b *Batcher) Stop() {
	close(b.stop)
	<-b.done
}
