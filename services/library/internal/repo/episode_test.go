package repo

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// TestEpisode_TableName pins the GORM table name so a future refactor
// that accidentally pluralizes back to "episodes" is caught.
func TestEpisode_TableName(t *testing.T) {
	if got := (domain.Episode{}).TableName(); got != "library_episodes" {
		t.Fatalf("TableName() = %q, want library_episodes", got)
	}
}

// TestDedupeNewestPerAnime covers the ae-probe target dedupe rule: from an
// already newest-first slice, keep the first (newest) row per shikimori_id and
// cap at limit.
func TestDedupeNewestPerAnime(t *testing.T) {
	// Newest-first input: anime "1" appears twice (ep5 newer than ep4),
	// anime "2" once, anime "3" once.
	in := []domain.Episode{
		{ShikimoriID: "1", EpisodeNumber: 5},
		{ShikimoriID: "1", EpisodeNumber: 4},
		{ShikimoriID: "2", EpisodeNumber: 12},
		{ShikimoriID: "3", EpisodeNumber: 1},
	}
	got := dedupeNewestPerAnime(in, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (distinct anime), got %+v", len(got), got)
	}
	wantIDs := []string{"1", "2", "3"}
	for i, w := range wantIDs {
		if got[i].ShikimoriID != w {
			t.Fatalf("got[%d].ShikimoriID = %q, want %q", i, got[i].ShikimoriID, w)
		}
	}
	// Anime "1" must be its NEWEST episode (5), not 4.
	if got[0].EpisodeNumber != 5 {
		t.Fatalf("anime 1 episode = %d, want 5 (newest)", got[0].EpisodeNumber)
	}
}

// TestDedupeNewestPerAnime_Cap caps at limit even with more distinct anime.
func TestDedupeNewestPerAnime_Cap(t *testing.T) {
	in := []domain.Episode{
		{ShikimoriID: "1", EpisodeNumber: 1},
		{ShikimoriID: "2", EpisodeNumber: 1},
		{ShikimoriID: "3", EpisodeNumber: 1},
		{ShikimoriID: "4", EpisodeNumber: 1},
	}
	if got := dedupeNewestPerAnime(in, 2); len(got) != 2 {
		t.Fatalf("len = %d, want 2 (capped)", len(got))
	}
}

// TestEpisode_JSON_NilPointersOmitted asserts pointer fields stay out
// of JSON output when nil. The HTTP handler relies on this so the
// public response shape matches the spec.
func TestEpisode_JSON_NilPointersOmitted(t *testing.T) {
	ep := domain.Episode{
		ShikimoriID:   "12345",
		EpisodeNumber: 3,
		MinioPath:     "12345/3/",
	}
	b, err := json.Marshal(ep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, `"job_id"`) {
		t.Errorf("job_id should be omitted when nil; got %s", s)
	}
	if strings.Contains(s, `"duration_sec"`) {
		t.Errorf("duration_sec should be omitted when nil; got %s", s)
	}
	if strings.Contains(s, `"size_bytes"`) {
		t.Errorf("size_bytes should be omitted when nil; got %s", s)
	}
}

// TestEpisode_JSON_PointersIncluded verifies populated pointer fields
// DO appear in JSON (the omitempty kicks in only for nil).
func TestEpisode_JSON_PointersIncluded(t *testing.T) {
	jobID := "abc-def"
	dur := 1450
	size := int64(123456)
	ep := domain.Episode{
		ShikimoriID:   "12345",
		EpisodeNumber: 3,
		JobID:         &jobID,
		MinioPath:     "12345/3/",
		DurationSec:   &dur,
		SizeBytes:     &size,
	}
	b, err := json.Marshal(ep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"job_id":"abc-def"`) {
		t.Errorf("job_id missing from JSON: %s", s)
	}
	if !strings.Contains(s, `"duration_sec":1450`) {
		t.Errorf("duration_sec missing from JSON: %s", s)
	}
	if !strings.Contains(s, `"size_bytes":123456`) {
		t.Errorf("size_bytes missing from JSON: %s", s)
	}
}

// TestFilenamePattern_TableName pins the table name for the second
// new model.
func TestFilenamePattern_TableName(t *testing.T) {
	if got := (domain.FilenamePattern{}).TableName(); got != "library_filename_patterns" {
		t.Fatalf("TableName() = %q, want library_filename_patterns", got)
	}
}

// TestEpisodeRepository_MigratorMethods_Signatures pins the method set the
// Phase-7 admin-content migrator depends on (07-03 Task 1). The DB-backed
// behavior (legacy-prefix filtering, single-column repoint) is exercised in
// episode_integration_test.go behind the `integration` build tag; this no-DB
// test guards the signatures so a refactor can't silently drop or reshape them.
func TestEpisodeRepository_MigratorMethods_Signatures(t *testing.T) {
	rt := reflect.TypeOf(&EpisodeRepository{})

	up, ok := rt.MethodByName("UpdateMinioPath")
	if !ok {
		t.Fatal("EpisodeRepository.UpdateMinioPath missing")
	}
	// (recv, ctx, id, path) → error
	if got := up.Type.NumIn(); got != 4 {
		t.Fatalf("UpdateMinioPath NumIn = %d, want 4 (recv, ctx, id, path)", got)
	}
	if up.Type.In(2).Kind() != reflect.String || up.Type.In(3).Kind() != reflect.String {
		t.Fatalf("UpdateMinioPath args must be (string id, string path), got (%s, %s)",
			up.Type.In(2), up.Type.In(3))
	}
	if up.Type.NumOut() != 1 || !up.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatalf("UpdateMinioPath must return a single error")
	}

	lp, ok := rt.MethodByName("ListAdminLegacyPath")
	if !ok {
		t.Fatal("EpisodeRepository.ListAdminLegacyPath missing")
	}
	// (recv, ctx) → ([]domain.Episode, error)
	if got := lp.Type.NumIn(); got != 2 {
		t.Fatalf("ListAdminLegacyPath NumIn = %d, want 2 (recv, ctx)", got)
	}
	if lp.Type.NumOut() != 2 {
		t.Fatalf("ListAdminLegacyPath NumOut = %d, want 2 ([]Episode, error)", lp.Type.NumOut())
	}
	if lp.Type.Out(0) != reflect.TypeOf([]domain.Episode(nil)) {
		t.Fatalf("ListAdminLegacyPath first return = %s, want []domain.Episode", lp.Type.Out(0))
	}
}

// TestEpisodeRepository_LegacyPredicate_SourceLiteral guards the exact LIKE
// predicate the migrator relies on to exclude already-migrated rows. The
// behavioral DB assertion (only legacy-prefix rows returned) lives in
// episode_integration_test.go; this is the no-DB tripwire so a refactor can't
// quietly change the filter to something that re-migrates aeProvider/ rows.
func TestEpisodeRepository_LegacyPredicate_SourceLiteral(t *testing.T) {
	const want = "minio_path NOT LIKE 'aeProvider/%'"
	src, err := os.ReadFile("episode.go")
	if err != nil {
		t.Fatalf("read episode.go: %v", err)
	}
	if !strings.Contains(string(src), want) {
		t.Fatalf("episode.go must filter legacy rows with %q (idempotency tripwire)", want)
	}
}

// TestEpisodeRepository_BumpFetch_Signature pins the BumpFetch method shape the
// Phase-08 ae serve HIT path depends on (SERVE-02):
// (recv, ctx, malID string, episode int) → error. The DB-backed behavior
// (atomic last_fetch_at/fetch_count update, no-op on absent row) is exercised
// behind the `integration` build tag; this no-DB test guards the signature so a
// refactor can't silently drop or reshape it.
func TestEpisodeRepository_BumpFetch_Signature(t *testing.T) {
	rt := reflect.TypeOf(&EpisodeRepository{})
	m, ok := rt.MethodByName("BumpFetch")
	if !ok {
		t.Fatal("EpisodeRepository.BumpFetch missing")
	}
	// (recv, ctx, malID, episode) → error
	if got := m.Type.NumIn(); got != 4 {
		t.Fatalf("BumpFetch NumIn = %d, want 4 (recv, ctx, malID, episode)", got)
	}
	if m.Type.In(2).Kind() != reflect.String {
		t.Fatalf("BumpFetch malID arg must be string, got %s", m.Type.In(2))
	}
	if m.Type.In(3).Kind() != reflect.Int {
		t.Fatalf("BumpFetch episode arg must be int, got %s", m.Type.In(3))
	}
	if m.Type.NumOut() != 1 || !m.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatal("BumpFetch must return a single error")
	}
}

// TestEpisodeRepository_EvictionMethods_Signatures pins the method set the
// Phase-10 Evictor (Plan 02) + pre-admit gate (Plan 03) compose against:
//
//	SumPoolBytes(ctx) (int64, error)
//	ListStaleEvictionCandidates(ctx, *AutocacheConfig, time.Time) ([]Episode, error)
//	DeleteByID(ctx, string) error
//
// DB-backed behavior (LIKE-prefix scoping, 4-tier order, Stale filter, nil-id
// no-op) is exercised behind the `integration` build tag; this no-DB test guards
// the signatures so a refactor can't silently drop or reshape them.
func TestEpisodeRepository_EvictionMethods_Signatures(t *testing.T) {
	rt := reflect.TypeOf(&EpisodeRepository{})
	errType := reflect.TypeOf((*error)(nil)).Elem()

	sp, ok := rt.MethodByName("SumPoolBytes")
	if !ok {
		t.Fatal("EpisodeRepository.SumPoolBytes missing")
	}
	// (recv, ctx) → (int64, error)
	if sp.Type.NumIn() != 2 {
		t.Fatalf("SumPoolBytes NumIn = %d, want 2 (recv, ctx)", sp.Type.NumIn())
	}
	if sp.Type.NumOut() != 2 || sp.Type.Out(0).Kind() != reflect.Int64 || !sp.Type.Out(1).Implements(errType) {
		t.Fatalf("SumPoolBytes must return (int64, error)")
	}

	lc, ok := rt.MethodByName("ListStaleEvictionCandidates")
	if !ok {
		t.Fatal("EpisodeRepository.ListStaleEvictionCandidates missing")
	}
	// (recv, ctx, *AutocacheConfig, time.Time) → ([]Episode, error)
	if lc.Type.NumIn() != 4 {
		t.Fatalf("ListStaleEvictionCandidates NumIn = %d, want 4 (recv, ctx, cfg, now)", lc.Type.NumIn())
	}
	if lc.Type.In(2) != reflect.TypeOf((*domain.AutocacheConfig)(nil)) {
		t.Fatalf("ListStaleEvictionCandidates cfg arg = %s, want *domain.AutocacheConfig", lc.Type.In(2))
	}
	if lc.Type.In(3) != reflect.TypeOf(time.Time{}) {
		t.Fatalf("ListStaleEvictionCandidates now arg = %s, want time.Time", lc.Type.In(3))
	}
	if lc.Type.NumOut() != 2 || lc.Type.Out(0) != reflect.TypeOf([]domain.Episode(nil)) || !lc.Type.Out(1).Implements(errType) {
		t.Fatalf("ListStaleEvictionCandidates must return ([]domain.Episode, error)")
	}

	dl, ok := rt.MethodByName("DeleteByID")
	if !ok {
		t.Fatal("EpisodeRepository.DeleteByID missing")
	}
	// (recv, ctx, id) → error
	if dl.Type.NumIn() != 3 {
		t.Fatalf("DeleteByID NumIn = %d, want 3 (recv, ctx, id)", dl.Type.NumIn())
	}
	if dl.Type.In(2).Kind() != reflect.String {
		t.Fatalf("DeleteByID id arg = %s, want string", dl.Type.In(2))
	}
	if dl.Type.NumOut() != 1 || !dl.Type.Out(0).Implements(errType) {
		t.Fatalf("DeleteByID must return a single error")
	}
}

// TestEpisodeRepository_SumPoolBytes_CoalesceTripwire guards that SumPoolBytes
// folds an empty/all-NULL pool to 0 via COALESCE (the budget math relies on a
// 0, never a SQL NULL) and scopes to the aeProvider/ pool prefix. The DB-backed
// zero-pool assertion lives behind `integration`; this is the no-DB source
// tripwire so a refactor can't quietly drop the COALESCE or the prefix scope.
func TestEpisodeRepository_SumPoolBytes_CoalesceTripwire(t *testing.T) {
	src := readEpisodeSource(t)
	if !strings.Contains(src, "COALESCE(SUM(size_bytes), 0)") {
		t.Fatal("SumPoolBytes must COALESCE(SUM(size_bytes), 0) so an empty pool yields 0, not NULL")
	}
	if !strings.Contains(src, "minio_path LIKE 'aeProvider/%'") {
		t.Fatal("SumPoolBytes must scope to the aeProvider/ pool prefix")
	}
}

// TestEpisodeRepository_StaleCandidates_OrderingTripwire guards the locked
// 4-tier eviction CASE order + the COALESCE within-tier sort. The behavioral DB
// assertion (only-Stale rows, tier order) lives behind `integration`; this is
// the no-DB source tripwire so a refactor can't silently change the ordering.
func TestEpisodeRepository_StaleCandidates_OrderingTripwire(t *testing.T) {
	src := readEpisodeSource(t)
	// Tier CASE: the four locked branches.
	for _, want := range []string{
		"WHEN source = 'autocache' AND last_fetch_at IS NULL THEN 1",
		"WHEN source = 'autocache' THEN 2",
		"WHEN source = 'admin' AND last_fetch_at IS NULL THEN 3",
		"ELSE 4 END ASC",
		"COALESCE(last_fetch_at, downloaded_at, created_at) ASC",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("ListStaleEvictionCandidates must contain ordering clause %q", want)
		}
	}
}

// TestEpisodeRepository_StaleCandidates_BoundParamsTripwire guards T-10-01: the
// freshness day-windows must be passed as bound `?` params (computed from cfg
// ints via now.AddDate), NEVER string-interpolated into the WHERE clause. A
// quiet refactor to fmt.Sprintf(cfg.Auto*/Admin*) would reintroduce an
// injection-shaped SQL path — this tripwire fails fast.
func TestEpisodeRepository_StaleCandidates_BoundParamsTripwire(t *testing.T) {
	src := readEpisodeSource(t)
	if !strings.Contains(src, "now.AddDate(0, 0, -cfg.AutoFreshDownloadDays)") {
		t.Fatal("freshness cutoffs must be computed via now.AddDate (then bound), not interpolated")
	}
	// No fmt.Sprintf of cfg windows into SQL.
	for _, bad := range []string{
		"Sprintf",
		`+ cfg.AutoFreshDownloadDays`,
		`+ cfg.AutoFreshFetchDays`,
		`+ cfg.AdminFreshDays`,
	} {
		if strings.Contains(src, bad) {
			t.Fatalf("eviction-candidate SQL must use bound ? params, found interpolation-shaped %q", bad)
		}
	}
}

// readEpisodeSource is a small helper that reads episode.go for the source
// tripwire tests in this file.
func readEpisodeSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("episode.go")
	if err != nil {
		t.Fatalf("read episode.go: %v", err)
	}
	return string(src)
}

// TestEpisodeRepository_BumpFetch_AtomicIncrementTripwire guards that BumpFetch
// uses an atomic gorm.Expr increment (no read-modify-write race) and bumps
// last_fetch_at. The behavioral DB assertion lives behind `integration`; this is
// the no-DB source tripwire so a refactor can't quietly change the increment.
func TestEpisodeRepository_BumpFetch_AtomicIncrementTripwire(t *testing.T) {
	src, err := os.ReadFile("episode.go")
	if err != nil {
		t.Fatalf("read episode.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "fetch_count + 1") {
		t.Fatal("episode.go BumpFetch must use an atomic gorm.Expr(\"fetch_count + 1\") increment")
	}
	if !strings.Contains(s, `"last_fetch_at"`) {
		t.Fatal("episode.go BumpFetch must bump last_fetch_at")
	}
}
