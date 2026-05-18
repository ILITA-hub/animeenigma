package repo

import (
	"encoding/json"
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
