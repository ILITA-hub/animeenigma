package ingest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type fakeStore struct {
	mu         sync.Mutex
	inserted   []domain.Event
	identifies int
}

func (f *fakeStore) InsertBatch(_ context.Context, e []domain.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserted = append(f.inserted, e...)
	return nil
}
func (f *fakeStore) UpsertIdentity(_ context.Context, _, _ string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.identifies++
	return nil
}
func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inserted)
}

func TestBatcher_FlushesOnInterval(t *testing.T) {
	store := &fakeStore{}
	b := New(store, Config{MaxBatch: 1000, FlushInterval: 20 * time.Millisecond, BufferSize: 100})
	b.Start()
	defer b.Stop()

	b.Enqueue(domain.Event{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1"})

	deadline := time.After(time.Second)
	for store.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("event was not flushed within 1s")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatcher_FlushesOnSize(t *testing.T) {
	store := &fakeStore{}
	b := New(store, Config{MaxBatch: 2, FlushInterval: time.Hour, BufferSize: 100})
	b.Start()
	defer b.Stop()

	b.Enqueue(domain.Event{EventID: "e1", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1"})
	b.Enqueue(domain.Event{EventID: "e2", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1"})

	deadline := time.After(time.Second)
	for store.count() < 2 {
		select {
		case <-deadline:
			t.Fatalf("size-triggered flush failed, got %d", store.count())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatcher_IdentifyUpsert(t *testing.T) {
	store := &fakeStore{}
	b := New(store, Config{MaxBatch: 1, FlushInterval: time.Hour, BufferSize: 100})
	b.Start()
	defer b.Stop()

	b.Enqueue(domain.Event{EventID: "e1", EventType: domain.EventTypeIdentify, AnonymousID: "a1", UserID: "u1", SessionID: "s1", Timestamp: time.Now()})

	deadline := time.After(time.Second)
	for store.count() < 1 {
		select {
		case <-deadline:
			t.Fatal("identify event not flushed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	store.mu.Lock()
	got := store.identifies
	store.mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 identity upsert, got %d", got)
	}
}
