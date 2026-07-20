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
	mux.HandleFunc("/internal/interest/bands", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":1}],"top":[],"planned":[],"idle_window":[],"idle_total":0}}`))
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
	e := NewEngine(cat, sig, store, 720*time.Hour, false, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, nil)
	return &engineFixture{engine: e, sig: sig, store: store, srv: srv, capsHits: capsHits}
}

// newTwoAnimeGogoanimeFixture wires a two-candidate world: ongoing anime
// "o1" and "o2", each with exactly one probeable unit on the SAME provider
// (gogoanime / hd-1 / sub) — used to test provider exclusivity ACROSS
// anime, not just within one title's pending units.
func newTwoAnimeGogoanimeFixture(t *testing.T) *engineFixture {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/interest/bands", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F1","episodes_aired":1},{"id":"o2","name":"F2","episodes_aired":1}],"top":[],"planned":[],"idle_window":[],"idle_total":0}}`))
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

	e := NewEngine(cat, sig, store, 720*time.Hour, false, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, nil)
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
	mux.HandleFunc("/internal/interest/bands", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":2}],"top":[],"planned":[],"idle_window":[],"idle_total":0}}`))
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

	e := NewEngine(cat, sig, store, 720*time.Hour, skipEnabled, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, nil)
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
	if entries[0].Band != int(BandOngoing) { // ongoing only, no visitors/top
		t.Fatalf("snapshot band = %d, want %d (BandOngoing)", entries[0].Band, int(BandOngoing))
	}
	if entries[0].Cooling {
		t.Fatal("fresh candidate must not be cooling in snapshot")
	}
}

// TestClaimSnapshotConcurrent is a smoke test for the interest cache's
// mutex: in production, Claim runs on a worker goroutine while Snapshot
// serves an admin/debug HTTP handler against the same *Engine — both read
// and write e.interestCache/e.interestAt via interest(). The assertion is
// simply "no data race, no panic" under `go test -race`.
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

// TestClaimSkipFullyCoveredByAniskipSettles: both skip episodes carry full
// AniSkip coverage (op+ed) → the gate filters every unit, the skip lane is
// settled without a single probe, and the title cools down exactly like an
// idle one.
func TestClaimSkipFullyCoveredByAniskipSettles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/interest/bands", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":2}],"top":[],"planned":[],"idle_window":[],"idle_total":0}}`))
	})
	mux.HandleFunc("/api/anime/o1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"families":[{"family":"others","providers":[
			{"provider":"kodik","state":"active","group":"ru"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/o1/kodik/translations", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":[{"id":610,"title":"Kodik","type":"voice","episodes_count":2}]}`))
	})
	mux.HandleFunc("/api/anime/o1", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"id":"o1","mal_id":"59970"}}`))
	})
	aniskipHits := new(int64)
	mux.HandleFunc("/api/skip-times/59970/", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(aniskipHits, 1)
		w.Write([]byte(`{"success":true,"data":{"found":true,"results":[
			{"interval":{"startTime":90,"endTime":180},"skipType":"op"},
			{"interval":{"startTime":1300,"endTime":1390},"skipType":"ed"}]}}`))
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
	ctx := context.Background()
	if err := store.UpsertUnit(ctx, "o1", "kodik", domain.UnitVerdict{
		Key: domain.UnitKey{Team: "610", Category: "dub"}, Episode: 2,
		Status: domain.StatusVerified, ProbedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	e := NewEngine(cat, sig, store, 720*time.Hour, true, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, nil)

	u, task, release, err := e.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil || task != nil || release != nil {
		t.Fatalf("fully AniSkip-covered title must be idle, got unit=%+v task=%+v", u, task)
	}
	if !sig.InCooldown(ctx, "o1") {
		t.Fatal("fully covered title must cool down like a settled one")
	}
	if hits := atomic.LoadInt64(aniskipHits); hits != 2 {
		t.Fatalf("aniskip coverage fetches = %d, want 2 (one per episode)", hits)
	}

	// Second claim within the coverage TTL: the cached coverage must serve —
	// no new skip-times fetches. (Cooldown is bypassed via a pin to force the
	// engine to actually re-plan the skip lane.)
	e.pins = map[string]string{"o1": ""}
	if _, task, _, err := e.Claim(ctx); err != nil || task != nil {
		t.Fatalf("second claim: err=%v task=%+v, want idle", err, task)
	}
	if hits := atomic.LoadInt64(aniskipHits); hits != 2 {
		t.Fatalf("aniskip coverage fetches after cached re-plan = %d, want still 2", hits)
	}
}

// TestClaimPinBypassesCooldown: a cooled-down title is invisible to Claim —
// unless it's pinned, in which case its pending verify unit must be claimed
// anyway.
func TestClaimPinBypassesCooldown(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()
	f.sig.SetCooldown(ctx, "o1", time.Hour)

	if u, _, _, err := f.engine.Claim(ctx); err != nil || u != nil {
		t.Fatalf("unpinned cooled-down title must be idle, got unit=%+v err=%v", u, err)
	}

	f.engine.pins = map[string]string{"o1": ""}
	u, _, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("pinned title must bypass its cooldown and claim the pending verify unit")
	}
	release()
}

// newBandedTwoAnimeFixture wires a two-candidate world: ongoing "o1" (Band
// Ongoing, via the interest ongoing bucket) and non-ongoing "o2" (Band
// WatchedTop, made a candidate purely via a recorded visit — see
// signals.RecordVisit). Both expose exactly one pending unit on the SAME
// provider (gogoanime/hd-1/sub), so provider exclusivity makes "whichever
// band's candidate Claim reaches first" observable: only the winner can be
// claimed on this Claim call.
func newBandedTwoAnimeFixture(t *testing.T) *engineFixture {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/interest/bands", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F1","episodes_aired":1}],"top":[],"planned":[],"idle_window":[],"idle_total":0}}`))
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
	// o2 has no interest-bands membership at all — it becomes a candidate
	// (Band WatchedTop, Visitors=1) purely through a recorded visit.
	if err := sig.RecordVisit(context.Background(), "o2", "u1"); err != nil {
		t.Fatal(err)
	}

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

	e := NewEngine(cat, sig, store, 720*time.Hour, false, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, nil)
	return &engineFixture{engine: e, sig: sig, store: store, srv: srv}
}

// TestBandedClaimPrefersOngoing: with the lottery's primary band forced to
// Band 1 (rng=0.0 → weightedPick picks BandOngoing under the default 60/30/10
// weights), Claim must reach and claim the ongoing candidate's unit even
// though a watched (visited) candidate shares the same exclusive provider.
func TestBandedClaimPrefersOngoing(t *testing.T) {
	f := newBandedTwoAnimeFixture(t)
	ctx := context.Background()
	f.engine.rng = func() float64 { return 0 } // primary = BandOngoing

	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task != nil {
		t.Fatalf("expected a verify claim, not a skip task: %+v", task)
	}
	if u == nil || u.AnimeID != "o1" {
		t.Fatalf("expected the ongoing candidate (o1) claimed first, got %+v", u)
	}
	release()
}

// TestBandedClaimWatchedTopPrimaryClaimsBeforeOngoing proves bandOrder's
// weighting actually reaches Claim: when the lottery's primary band is Band 2
// (rng inside the WatchedTop slice of the 60/30/10 weights), bandOrder puts
// BandWatchedTop ahead of BandOngoing, so the watched (visited) candidate
// wins the shared provider instead of the ongoing one — the inverse of
// TestBandedClaimPrefersOngoing.
func TestBandedClaimWatchedTopPrimaryClaimsBeforeOngoing(t *testing.T) {
	f := newBandedTwoAnimeFixture(t)
	ctx := context.Background()
	f.engine.rng = func() float64 { return 0.7 } // 70 of 100 → BandWatchedTop (60..90)

	u, task, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task != nil {
		t.Fatalf("expected a verify claim, not a skip task: %+v", task)
	}
	if u == nil || u.AnimeID != "o2" {
		t.Fatalf("expected the watched candidate (o2) claimed first when its band is the lottery primary, got %+v", u)
	}
	release()
}

// TestClaimAdvancesIdleCursorOnFreshInterestFetch covers the idle
// round-robin sweep (spec §3.3): interest()'s fresh (cache-miss) fetch reads
// the current idle cursor as idle_offset, then advances it by idleWindow
// (wrapping at the interest response's idle_total) — regardless of which
// band ends up primary, every fresh fetch sweeps the cursor forward so the
// catalog tail is walked over successive claims.
func TestClaimAdvancesIdleCursorOnFreshInterestFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/interest/bands", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[],"top":[],"planned":[],
			"idle_window":[{"id":"i1","name":"Idle1","top_rank":1}],"idle_total":250}}`))
	})
	mux.HandleFunc("/api/anime/i1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"families":[{"family":"others","providers":[
			{"provider":"gogoanime","state":"active","group":"en"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/i1/scraper/episodes", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep-1","number":1}]}}`))
	})
	mux.HandleFunc("/api/anime/i1/scraper/servers", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"hd-1","name":"HD-1","type":"sub"}]}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	sig := signals.New(rdb)
	ctx := context.Background()

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

	e := NewEngine(cat, sig, store, 720*time.Hour, false, nil, [3]int{60, 30, 10}, 48*time.Hour, 168*time.Hour, 100, nil)
	e.rng = func() float64 { return 0.99 } // primary = BandIdle

	if got := sig.IdleCursor(ctx); got != 0 {
		t.Fatalf("cold cursor = %d, want 0", got)
	}
	u, _, release, err := e.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.AnimeID != "i1" {
		t.Fatalf("expected the idle-window candidate claimed, got %+v", u)
	}
	release()
	if got := sig.IdleCursor(ctx); got != 100 {
		t.Fatalf("idle cursor after fresh interest fetch = %d, want 100 (idleWindow), got %d", got, got)
	}
}
