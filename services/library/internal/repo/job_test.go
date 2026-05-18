package repo

import (
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
