package repo

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// TestEpisode_TableName pins the GORM table name so a future refactor
// that accidentally pluralizes back to "episodes" is caught.
func TestEpisode_TableName(t *testing.T) {
	if got := (domain.Episode{}).TableName(); got != "library_episodes" {
		t.Fatalf("TableName() = %q, want library_episodes", got)
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
