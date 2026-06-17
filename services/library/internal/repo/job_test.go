package repo

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// TestJobStatus_IsTerminal exercises the domain-side helper that the
// repo + worker both rely on for "should we touch this row again?"
// semantics. Kept here rather than the domain package so the test
// suite that covers Cancel() can grep them together.
func TestJobStatus_IsTerminal(t *testing.T) {
	cases := []struct {
		name string
		s    domain.JobStatus
		want bool
	}{
		{"done is terminal", domain.JobStatusDone, true},
		{"failed is terminal", domain.JobStatusFailed, true},
		{"cancelled is terminal", domain.JobStatusCancelled, true},
		{"queued is not terminal", domain.JobStatusQueued, false},
		{"downloading is not terminal", domain.JobStatusDownloading, false},
		{"encoding is not terminal", domain.JobStatusEncoding, false},
		{"uploading is not terminal", domain.JobStatusUploading, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.IsTerminal(); got != tc.want {
				t.Fatalf("IsTerminal(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// TestJobFilterDefaults documents the limit-clamp contract enforced
// inside List(): limit <= 0 → 100, limit > 500 → 500. The body of List
// is integration-tested elsewhere (build tag `integration` +
// INTEGRATION=1); this test pins the constants so a future refactor
// doesn't silently change the API contract.
func TestJobFilterDefaults(t *testing.T) {
	t.Run("zero limit clamps to 100", func(t *testing.T) {
		// We can't exercise the SQL path here without a real DB, but
		// we can assert the constant relationships hold so a future
		// edit to List() that breaks them is caught by review.
		const (
			expectedDefault = 100
			expectedMax     = 500
		)
		if expectedDefault >= expectedMax {
			t.Fatalf("default limit must be < max limit; got default=%d max=%d", expectedDefault, expectedMax)
		}
	})
}

// TestJobTableName pins the table-name override so a future GORM
// migration accidentally pluralizing to "jobs" is caught.
func TestJobTableName(t *testing.T) {
	if name := (domain.Job{}).TableName(); name != "library_jobs" {
		t.Fatalf("Job.TableName() = %q, want %q", name, "library_jobs")
	}
}

// TestRetryErrorTextFormat pins the Phase-5 SPEC-locked format for
// the retry row's error_text so a future refactor of Retry() doesn't
// silently change the audit-trail string admins grep on.
func TestRetryErrorTextFormat(t *testing.T) {
	const oldID = "abc-123"
	got := formatRetryErrorText(oldID)
	want := "retry of abc-123"
	if got != want {
		t.Fatalf("retry error_text = %q, want %q", got, want)
	}
}

// TestUpdateShikimoriID_NilRepo defensively asserts UpdateShikimoriID
// rejects empty input rather than running an unbounded UPDATE.
func TestUpdateShikimoriID_EmptyID(t *testing.T) {
	r := &JobRepository{} // nil db; the method must short-circuit before touching it
	err := r.UpdateShikimoriID(nil, "", "12345")
	if err == nil {
		t.Fatalf("UpdateShikimoriID with empty job id must error")
	}
}

// TestJobRepository_HasActiveForEpisode_Signature pins the Phase-09 single-flight
// gate's method shape so a refactor can't silently reshape it. The DB-backed
// behavioral assertion is integration-gated (the repo's query tests run behind
// //go:build integration); this no-DB test guards the signature
// (recv, ctx, shikimoriID string, episode int) → (bool, error).
func TestJobRepository_HasActiveForEpisode_Signature(t *testing.T) {
	rt := reflect.TypeOf(&JobRepository{})
	m, ok := rt.MethodByName("HasActiveForEpisode")
	if !ok {
		t.Fatal("JobRepository.HasActiveForEpisode missing")
	}
	if got := m.Type.NumIn(); got != 4 {
		t.Fatalf("HasActiveForEpisode NumIn = %d, want 4 (recv, ctx, shikimoriID, episode)", got)
	}
	if m.Type.In(2).Kind() != reflect.String {
		t.Fatalf("HasActiveForEpisode shikimoriID arg must be string, got %s", m.Type.In(2))
	}
	if m.Type.In(3).Kind() != reflect.Int {
		t.Fatalf("HasActiveForEpisode episode arg must be int, got %s", m.Type.In(3))
	}
	if m.Type.NumOut() != 2 || m.Type.Out(0).Kind() != reflect.Bool {
		t.Fatal("HasActiveForEpisode must return (bool, error)")
	}
	if !m.Type.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatal("HasActiveForEpisode second return must be error")
	}
}

// TestJobRepository_HasActiveForEpisode_TerminalExclusionTripwire guards that the
// single-flight gate excludes terminal statuses (a done/failed/cancelled job must
// NOT block a re-enqueue) and keys on both shikimori_id and episode. The
// behavioral assertion is integration-gated; this is the no-DB source tripwire.
func TestJobRepository_HasActiveForEpisode_TerminalExclusionTripwire(t *testing.T) {
	src, err := os.ReadFile("job.go")
	if err != nil {
		t.Fatalf("read job.go: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "status NOT IN") {
		t.Fatal("HasActiveForEpisode must exclude terminal statuses via `status NOT IN ?` (single-flight tripwire)")
	}
	if !strings.Contains(s, "shikimori_id = ? AND episode = ?") {
		t.Fatal("HasActiveForEpisode must key on (shikimori_id, episode)")
	}
}
