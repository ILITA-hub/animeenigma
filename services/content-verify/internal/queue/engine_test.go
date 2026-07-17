package queue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
)

// engineFixture wires a single-candidate world: one ongoing anime "o1" with
// exactly one probeable unit (gogoanime / hd-1 / sub). capsHandler lets each
// test override the /capabilities response to simulate enumerate failure.
type engineFixture struct {
	engine *Engine
	sig    *signals.Signals
	store  *repo.Store
	srv    *httptest.Server
}

func newEngineFixture(t *testing.T, capsFail bool) *engineFixture {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":1}],"top":[]}}`))
	})
	mux.HandleFunc("/api/anime/o1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		if capsFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"success":true,"data":{"families":[{"family":"others","providers":[
			{"provider":"gogoanime","state":"active","group":"en"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/o1/scraper/episodes", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep-1","number":1}]}}`))
	})
	mux.HandleFunc("/api/anime/o1/scraper/servers", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"hd-1","name":"HD-1","type":"sub"}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sig := signals.New(rdb)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ContentVerification{}); err != nil {
		t.Fatal(err)
	}
	// sqlite ":memory:" is per-connection: a pooled connection other than
	// the one AutoMigrate ran on sees an empty database ("no such table").
	// Pin the pool to one connection so concurrent goroutines (see
	// TestClaimSnapshotConcurrent) all share the migrated schema. Real
	// deployments use Postgres via libs/database, which has no such quirk.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	store := repo.NewStore(db)

	e := NewEngine(cat, sig, store, 720*time.Hour, nil)
	return &engineFixture{engine: e, sig: sig, store: store, srv: srv}
}

func TestClaimHappyPath(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	u, ongoing, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("expected a claimed unit, got nil")
	}
	if !ongoing {
		t.Fatal("o1 is ongoing; Claim must report ongoing=true")
	}
	if u.AnimeID != "o1" || u.Provider != "gogoanime" || u.Key.Server != "hd-1" {
		t.Fatalf("unexpected unit: %+v", u)
	}
}

func TestClaimSkipsCoolingCandidate(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	f.sig.SetCooldown(ctx, "o1", time.Hour)
	u, _, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("cooling candidate must not be claimed, got %+v", u)
	}
}

func TestClaimEnumerateFailureSetsOneHourCooldown(t *testing.T) {
	f := newEngineFixture(t, true) // capabilities endpoint 500s
	ctx := context.Background()
	u, _, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("broken enumerate must yield no claim, got %+v", u)
	}
	if !f.sig.InCooldown(ctx, "o1") {
		t.Fatal("enumerate failure must set a cooldown so we don't hammer a broken title")
	}
}

func TestClaimEmptyPendingSetsCooldownTTL(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	// Pre-seed the store with a fresh verdict for the only unit gogoanime
	// exposes, so PendingUnits finds nothing left to do.
	err := f.store.UpsertUnit(ctx, "o1", "gogoanime", domain.UnitVerdict{
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 1,
		Status: domain.StatusVerified, ProbedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("fully-verified title must yield no claim, got %+v", u)
	}
	if !f.sig.InCooldown(ctx, "o1") {
		t.Fatal("idle (nothing pending) title must be cooled down (CooldownTTL)")
	}
}

func TestSnapshot(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	entries := f.engine.Snapshot(ctx, 10)
	if len(entries) != 1 || entries[0].AnimeID != "o1" {
		t.Fatalf("snapshot: %+v", entries)
	}
	if entries[0].Score != 10 { // ongoing only, no visitors/top
		t.Fatalf("snapshot score = %d, want 10", entries[0].Score)
	}
	if entries[0].Cooling {
		t.Fatal("fresh candidate must not be cooling in snapshot")
	}
}

// TestClaimSnapshotConcurrent is a smoke test for the membership cache's
// mutex: in production, Claim runs on a worker goroutine while Snapshot
// serves an admin/debug HTTP handler against the same *Engine — both read
// and write e.memb/e.membAt via membership(). The assertion is simply "no
// data race, no panic" under `go test -race`.
func TestClaimSnapshotConcurrent(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, _, _ = f.engine.Claim(ctx)
			} else {
				_ = f.engine.Snapshot(ctx, 5)
			}
		}(i)
	}
	wg.Wait()
}
