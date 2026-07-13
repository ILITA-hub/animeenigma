package service

// Verifies the progress-to-cache refactor: in-flight download progress is
// written to the live ProgressStore every tick, never to the DB, and the DB row
// takes a single progress write at the terminal transition (→encoding at 100).

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// fakeProgressStore is an in-memory ProgressStore for unit tests.
type fakeProgressStore struct {
	mu   sync.Mutex
	vals map[string]int
	dels map[string]int
	sets int
}

func newFakeProgressStore() *fakeProgressStore {
	return &fakeProgressStore{vals: map[string]int{}, dels: map[string]int{}}
}

func (f *fakeProgressStore) SetProgress(_ context.Context, id string, pct int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vals[id] = pct
	f.sets++
	return nil
}

func (f *fakeProgressStore) GetProgressMany(_ context.Context, ids []string) (map[string]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]int, len(ids))
	for _, id := range ids {
		if v, ok := f.vals[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (f *fakeProgressStore) DeleteProgress(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dels[id]++
	delete(f.vals, id)
	return nil
}

func (f *fakeProgressStore) setCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sets
}

func (f *fakeProgressStore) delCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dels[id]
}

func TestWorkerPool_ProgressGoesToCacheNotDB(t *testing.T) {
	store := newFakeJobStore()
	job := &domain.Job{ID: "jp", Magnet: "m", Status: domain.JobStatusDownloading, Source: domain.JobSourceManual, Title: "t"}
	store.put(job)

	h := newFakeHandle("hash", 1000)
	h.advance(500, 5) // sit at 50% with peers so ticks fire without completing
	adder := &fakeAdder{next: h}
	prog := newFakeProgressStore()
	p, _ := newTestPool(t, store, adder, 30*time.Minute, 10*time.Millisecond)
	p.SetProgressCache(prog)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(60 * time.Millisecond) // let several 50% ticks fire, then complete
		h.advance(1000, 5)
	}()
	p.processJob(ctx, job)

	// Terminal DB row: encoding at 100% (the download's only progress write).
	got := store.snapshot(job.ID)
	if got.Status != domain.JobStatusEncoding {
		t.Fatalf("status = %q, want encoding", got.Status)
	}
	if got.ProgressPct != 100 {
		t.Fatalf("db progress_pct = %d, want 100 at encoding transition", got.ProgressPct)
	}

	// Live progress WAS written to the cache during the download.
	if prog.setCount() == 0 {
		t.Fatalf("expected live progress to be written to the cache")
	}
	// And the cache entry was cleared at the terminal transition.
	if prog.delCount(job.ID) == 0 {
		t.Fatalf("expected live cache to be cleared at the terminal transition")
	}

	// Exactly one DB progress write, and it was the terminal encoding one — no
	// intermediate 50% ever reached the DB.
	var dbProgressWrites []string
	for _, c := range store.calls {
		if strings.HasPrefix(c, "SetProgressAndStatus(") {
			dbProgressWrites = append(dbProgressWrites, c)
		}
	}
	if len(dbProgressWrites) != 1 || dbProgressWrites[0] != "SetProgressAndStatus(encoding,100)" {
		t.Fatalf("db progress writes = %v, want exactly [SetProgressAndStatus(encoding,100)]", dbProgressWrites)
	}
}
