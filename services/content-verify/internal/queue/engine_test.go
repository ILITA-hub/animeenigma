package queue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
// capsHits counts requests to the /capabilities endpoint (atomically, so it
// stays correct under TestClaimSnapshotConcurrent's concurrent goroutines) —
// used by the enum-cache test to assert the cache actually serves repeat
// Claims instead of re-fetching.
type engineFixture struct {
	engine   *Engine
	sig      *signals.Signals
	store    *repo.Store
	srv      *httptest.Server
	capsHits *int64
}

func newEngineFixture(t *testing.T, capsFail bool) *engineFixture {
	t.Helper()
	capsHits := new(int64)
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":1}],"top":[]}}`))
	})
	mux.HandleFunc("/api/anime/o1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(capsHits, 1)
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

	// skipEnabled=false: these fixtures/tests exercise the verify lane only;
	// with only one gogoanime unit, its single episode's skip unit would
	// otherwise ALSO come up "due" (no row) once the verify lane empties,
	// changing TestClaimEmptyPendingSetsCooldownTTL's outcome from cooldown
	// to a skip task. See TestClaimReturnsSkipTaskWhenVerifyLaneSettled for
	// skip-lane coverage.
	e := NewEngine(cat, sig, store, 720*time.Hour, false, nil)
	return &engineFixture{engine: e, sig: sig, store: store, srv: srv, capsHits: capsHits}
}

// newTwoAnimeGogoanimeFixture wires a two-candidate world: ongoing anime
// "o1" and "o2", each with exactly one probeable unit on the SAME provider
// (gogoanime / hd-1 / sub) — used to test provider exclusivity ACROSS
// anime, not just within one title's pending units.
func newTwoAnimeGogoanimeFixture(t *testing.T) *engineFixture {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F1","episodes_aired":1},{"id":"o2","name":"F2","episodes_aired":1}],"top":[]}}`))
	})
	for _, id := range []string{"o1", "o2"} {
		mux.HandleFunc("/api/anime/"+id+"/capabilities", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"success":true,"data":{"families":[{"family":"others","providers":[
				{"provider":"gogoanime","state":"active","group":"en"}]}]}}`))
		})
		mux.HandleFunc("/api/anime/"+id+"/scraper/episodes", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep-1","number":1}]}}`))
		})
		mux.HandleFunc("/api/anime/"+id+"/scraper/servers", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"hd-1","name":"HD-1","type":"sub"}]}}`))
		})
	}
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
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	store := repo.NewStore(db)

	e := NewEngine(cat, sig, store, 720*time.Hour, false, nil)
	return &engineFixture{engine: e, sig: sig, store: store, srv: srv}
}

func TestClaimHappyPath(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("expected a claimed unit, got nil")
	}
	if task != nil {
		t.Fatalf("verify claim must not also carry a skip task: %+v", task)
	}
	if release == nil {
		t.Fatal("expected a non-nil release func when a unit is claimed")
	}
	if u.AnimeID != "o1" || u.Provider != "gogoanime" || u.Key.Server != "hd-1" {
		t.Fatalf("unexpected unit: %+v", u)
	}
	release()
}

func TestClaimSkipsCoolingCandidate(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	f.sig.SetCooldown(ctx, "o1", time.Hour)
	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil || task != nil || release != nil {
		t.Fatalf("cooling candidate must not be claimed, got unit=%+v task=%+v release=%v", u, task, release != nil)
	}
}

func TestClaimEnumerateFailureSetsOneHourCooldown(t *testing.T) {
	f := newEngineFixture(t, true) // capabilities endpoint 500s
	ctx := context.Background()
	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil || task != nil || release != nil {
		t.Fatalf("broken enumerate must yield no claim, got unit=%+v task=%+v release=%v", u, task, release != nil)
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
	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil || task != nil || release != nil {
		t.Fatalf("fully-verified title with skip lane disabled must yield no claim, got unit=%+v task=%+v release=%v", u, task, release != nil)
	}
	if !f.sig.InCooldown(ctx, "o1") {
		t.Fatal("idle (nothing pending) title must be cooled down (CooldownTTL)")
	}
}

// TestClaimLeaseExcludesSecondClaimUntilReleased covers the in-flight unit
// lease: a claim held (not yet released) must exclude a second Claim from
// re-offering the SAME unit, and releasing it must make the unit claimable
// again.
func TestClaimLeaseExcludesSecondClaimUntilReleased(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()

	u1, _, release1, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u1 == nil || release1 == nil {
		t.Fatal("expected a claimed unit with a non-nil release func")
	}

	// Single-unit fixture: with the only unit already leased, a second Claim
	// must find the candidate blocked (not idle-from-emptiness) and move on
	// to nothing — idle overall.
	u2, task2, release2, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u2 != nil || task2 != nil || release2 != nil {
		t.Fatalf("second claim while the only unit is in-flight must be idle, got unit=%+v task=%+v release=%v", u2, task2, release2 != nil)
	}

	release1()

	u3, _, release3, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u3 == nil || release3 == nil {
		t.Fatal("expected the unit to be claimable again after release")
	}
	if u3.Key != u1.Key || u3.Provider != u1.Provider {
		t.Fatalf("expected the same unit reclaimed: %+v vs %+v", u3, u1)
	}
	release3()
}

// TestClaimReleaseIsIdempotentAndDoesNotFreeAnotherLease covers double-
// release safety: calling a stale release a second time — after its lease
// was already freed and reclaimed by someone else — must not panic and must
// not free that OTHER claimant's still-active lease.
func TestClaimReleaseIsIdempotentAndDoesNotFreeAnotherLease(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()

	_, _, release1, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if release1 == nil {
		t.Fatal("expected a lease on the first claim")
	}
	release1() // frees the lease

	u2, _, release2, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u2 == nil || release2 == nil {
		t.Fatal("expected the unit reclaimable after the first release")
	}

	release1() // stale re-invocation: must be a no-op, not a panic

	u3, task3, release3, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u3 != nil || task3 != nil || release3 != nil {
		t.Fatalf("release1's stale second call must not free release2's active lease, got unit=%+v task=%+v release=%v", u3, task3, release3 != nil)
	}

	release2()
}

// TestClaimBlockedDoesNotSetCooldown covers the "blocked ≠ settled" rule: a
// candidate whose only pending unit is currently leased by another in-flight
// claim must NOT be cooled down — it still has real work, just held
// elsewhere — unlike a genuinely idle (nothing pending) candidate.
func TestClaimBlockedDoesNotSetCooldown(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()

	_, _, release1, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if release1 == nil {
		t.Fatal("expected a lease on the first claim")
	}
	defer release1()

	if _, _, _, err := f.engine.Claim(ctx); err != nil {
		t.Fatal(err)
	}
	if f.sig.InCooldown(ctx, "o1") {
		t.Fatal("a candidate blocked by an in-flight lease must not be cooled down")
	}
}

// TestClaimProviderExclusivityAcrossAnime covers cross-anime provider
// exclusivity: two DIFFERENT anime whose only pending unit both use
// "gogoanime" must not both be claimable at once — one probe per provider,
// globally, regardless of which title it belongs to.
func TestClaimProviderExclusivityAcrossAnime(t *testing.T) {
	f := newTwoAnimeGogoanimeFixture(t)
	ctx := context.Background()

	u1, task1, release1, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u1 == nil || task1 != nil || release1 == nil {
		t.Fatalf("expected a claimed unit with a release func, got unit=%+v task=%+v release=%v", u1, task1, release1 != nil)
	}
	defer release1()

	u2, task2, release2, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u2 != nil || task2 != nil || release2 != nil {
		t.Fatalf("o1 and o2 both use gogoanime; the second claim must be blocked by provider exclusivity, got unit=%+v task=%+v release=%v", u2, task2, release2 != nil)
	}
}

// TestClaimEnumerationCachedAcrossClaims covers the enumeration cache: two
// sequential Claims within enumCacheTTL for the same candidate must hit the
// catalog /capabilities endpoint only once.
func TestClaimEnumerationCachedAcrossClaims(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()

	_, _, release1, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if release1 == nil {
		t.Fatal("expected a lease on the first claim")
	}
	defer release1()

	if _, _, _, err := f.engine.Claim(ctx); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(f.capsHits); got != 1 {
		t.Fatalf("capabilities endpoint hit %d times across 2 claims, want 1 (enum cache should serve the second)", got)
	}
}

// newSkipEngineFixture wires a single-candidate world with ONE kodik
// translation ("Kodik", 2 episodes) so NextSkipTask's family selection is
// unambiguous: no fingerprints yet + exactly one family with >=2 due
// episodes must pair-bootstrap episodes 1+2 (skipplan.go's bootstrapPairScan
// rule). The verify lane is pre-seeded as already verified so Claim reaches
// the skip lane immediately.
func newSkipEngineFixture(t *testing.T, skipEnabled bool) *engineFixture {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":2}],"top":[]}}`))
	})
	mux.HandleFunc("/api/anime/o1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"families":[{"family":"others","providers":[
			{"provider":"kodik","state":"active","group":"ru"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/o1/kodik/translations", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":[{"id":610,"title":"Kodik","type":"voice","episodes_count":2}]}`))
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
	if err := db.AutoMigrate(&domain.ContentVerification{}, &domain.SkipTiming{}, &domain.SkipFingerprint{}); err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	store := repo.NewStore(db)

	// Pre-verify the single kodik verify unit (voice/dub synth) so the
	// verify lane is settled and Claim falls through to the skip lane.
	ctx := context.Background()
	err = store.UpsertUnit(ctx, "o1", "kodik", domain.UnitVerdict{
		Key: domain.UnitKey{Team: "610", Category: "dub"}, Episode: 2,
		Status: domain.StatusVerified, ProbedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	e := NewEngine(cat, sig, store, 720*time.Hour, skipEnabled, nil)
	return &engineFixture{engine: e, sig: sig, store: store, srv: srv}
}

func TestClaimReturnsSkipTaskWhenVerifyLaneSettled(t *testing.T) {
	f := newSkipEngineFixture(t, true)
	ctx := context.Background()
	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("verify lane is settled; expected no verify unit, got %+v", u)
	}
	if task == nil {
		t.Fatal("expected a skip task once the verify lane is settled")
	}
	if release == nil {
		t.Fatal("expected a non-nil release func when a skip task is claimed")
	}
	defer release()
	if task.Unit.Provider != "kodik" || task.Unit.Episode != 1 {
		t.Fatalf("unexpected skip unit: %+v", task.Unit)
	}
	if task.Pair == nil || task.Pair.Episode != 2 {
		t.Fatalf("expected a pair-bootstrap task with episode 2 (no fingerprints yet), got pair=%+v", task.Pair)
	}
}

// TestClaimSkipBlockedDoesNotSetCooldown mirrors
// TestClaimBlockedDoesNotSetCooldown for the SKIP lane: while another worker
// holds the candidate's only skip task, a second Claim must come back idle
// without cooling the title down — cooldown would freeze its remaining skip
// work for the whole CooldownTTL.
func TestClaimSkipBlockedDoesNotSetCooldown(t *testing.T) {
	f := newSkipEngineFixture(t, true)
	ctx := context.Background()

	_, task1, release1, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task1 == nil || release1 == nil {
		t.Fatal("expected a leased skip task on the first claim")
	}
	defer release1()

	u2, task2, release2, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u2 != nil || task2 != nil || release2 != nil {
		t.Fatalf("skip task is leased; second claim must be idle, got unit=%+v task=%+v release=%v", u2, task2, release2 != nil)
	}
	if f.sig.InCooldown(ctx, "o1") {
		t.Fatal("a candidate whose skip task is leased by an in-flight claim must not be cooled down")
	}
}

func TestClaimSkipDisabledFallsToCooldown(t *testing.T) {
	f := newSkipEngineFixture(t, false)
	ctx := context.Background()
	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil || task != nil || release != nil {
		t.Fatalf("skip lane disabled: expected nil unit and nil task, got unit=%+v task=%+v release=%v", u, task, release != nil)
	}
	if !f.sig.InCooldown(ctx, "o1") {
		t.Fatal("settled verify lane with skip lane disabled must cooldown like idle (exactly as today)")
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
				_, _, _, _ = f.engine.Claim(ctx)
			} else {
				_ = f.engine.Snapshot(ctx, 5)
			}
		}(i)
	}
	wg.Wait()
}
