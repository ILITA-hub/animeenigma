package health

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

// TestGolden_PoolNotEmpty — the static pool has between 5 and 10 entries
// (small enough to avoid amplifying probe traffic; large enough to surface
// per-anime selector drift).
func TestGolden_PoolNotEmpty(t *testing.T) {
	t.Parallel()
	n := len(DefaultGoldenPool)
	if n < 5 || n > 10 {
		t.Fatalf("len(DefaultGoldenPool) = %d; want 5..10", n)
	}
}

// TestGolden_PickDeterministicWithSeed — with a seeded rand.Rand, Pick is
// reproducible. The probe seeds its own RNG with time-based entropy, but
// tests rely on this determinism property.
func TestGolden_PickDeterministicWithSeed(t *testing.T) {
	t.Parallel()
	rng1 := rand.New(rand.NewPCG(42, 0))
	rng2 := rand.New(rand.NewPCG(42, 0))

	for i := 0; i < 100; i++ {
		a := Pick(DefaultGoldenPool, rng1)
		b := Pick(DefaultGoldenPool, rng2)
		// AnimeRef now carries an AltTitles slice (ISS-017), so it's no longer
		// comparable with !=; use reflect.DeepEqual.
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("iter %d: Pick diverged with same seed: %v vs %v", i, a, b)
		}
	}
}

// TestGolden_AllEntriesHaveMalID — every goldenEntry has a non-empty MalID,
// and the derived Ref.ShikimoriID matches (per the package contract:
// ShikimoriID == MAL ID). Without a MAL ID, FindID can't resolve the anime
// via malsync.moe and the probe records false-negative search failures.
func TestGolden_AllEntriesHaveMalID(t *testing.T) {
	t.Parallel()
	if len(goldenEntries) == 0 {
		t.Fatal("goldenEntries is empty")
	}
	for i, e := range goldenEntries {
		if e.MalID == "" {
			t.Errorf("goldenEntries[%d] (%s) has empty MalID", i, e.Ref.Title)
		}
		if e.Ref.ShikimoriID == "" {
			t.Errorf("goldenEntries[%d] (%s) has empty Ref.ShikimoriID", i, e.Ref.Title)
		}
		if e.MalID != e.Ref.ShikimoriID {
			t.Errorf("goldenEntries[%d] (%s) MalID=%q != ShikimoriID=%q (must match per contract)",
				i, e.Ref.Title, e.MalID, e.Ref.ShikimoriID)
		}
	}
}
