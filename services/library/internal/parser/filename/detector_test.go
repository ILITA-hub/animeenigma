package filename

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// stubFallback records every IncFilenameDetectFallback call.
type stubFallback struct {
	calls []string
}

func (s *stubFallback) IncFilenameDetectFallback(uploader string) {
	s.calls = append(s.calls, uploader)
}

// seedPatterns mirrors the five SPEC-locked rows from migration 003.
// Kept in sync with services/library/migrations/003_library_filename_patterns.sql.
func seedPatterns() []domain.FilenamePattern {
	return []domain.FilenamePattern{
		{Uploader: "Ohys-Raws", PatternRegex: `^\[Ohys-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+\(`, Example: "[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4"},
		{Uploader: "SubsPlease", PatternRegex: `^\[SubsPlease\]\s+.+?\s+-\s+(\d{1,3})\s+\(`, Example: "[SubsPlease] Frieren - 12 (1080p) [ABCD1234].mkv"},
		{Uploader: "Erai-raws", PatternRegex: `^\[Erai-raws\]\s+.+?\s+-\s+(\d{1,3})\s+\[`, Example: "[Erai-raws] Spy x Family - 07 [1080p][Multiple Subtitle].mkv"},
		{Uploader: "Leopard-Raws", PatternRegex: `^\[Leopard-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+RAW\s+`, Example: "[Leopard-Raws] Re-Zero - 03 RAW (BS11 1280x720 x264 AAC).mp4"},
		{Uploader: "ARC-Raws", PatternRegex: `^\[ARC-Raws\]\s+.+?\s+-\s+(\d{1,3})\s*[\[\(]`, Example: "[ARC-Raws] Made in Abyss - 05 [1080p].mkv"},
	}
}

// TestDetect_SeededExamples runs each uploader-specific pattern
// against its own SPEC example.
func TestDetect_SeededExamples(t *testing.T) {
	stub := &stubFallback{}
	d, err := NewDetector(seedPatterns(), stub)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}

	cases := []struct {
		uploader string
		filename string
		wantEp   int
	}{
		{"Ohys-Raws", "[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4", 1},
		{"SubsPlease", "[SubsPlease] Frieren - 12 (1080p) [ABCD1234].mkv", 12},
		{"Erai-raws", "[Erai-raws] Spy x Family - 07 [1080p][Multiple Subtitle].mkv", 7},
		{"Leopard-Raws", "[Leopard-Raws] Re-Zero - 03 RAW (BS11 1280x720 x264 AAC).mp4", 3},
		{"ARC-Raws", "[ARC-Raws] Made in Abyss - 05 [1080p].mkv", 5},
	}
	for _, tc := range cases {
		t.Run(tc.uploader, func(t *testing.T) {
			ep, ok := d.DetectEpisode(tc.filename, tc.uploader)
			if !ok {
				t.Fatalf("DetectEpisode(%q, %q) returned ok=false", tc.filename, tc.uploader)
			}
			if ep != tc.wantEp {
				t.Fatalf("episode = %d, want %d", ep, tc.wantEp)
			}
		})
	}
	// None of the uploader-specific paths should fall back.
	if len(stub.calls) != 0 {
		t.Errorf("uploader-specific matches must NOT increment fallback metric; got %d calls: %v", len(stub.calls), stub.calls)
	}
}

// TestDetect_CaseInsensitiveUploader — the lookup key is lowercased
// so callers passing "OHYS-RAWS" or "Ohys-Raws" both hit the
// "ohys-raws" key.
func TestDetect_CaseInsensitiveUploader(t *testing.T) {
	d, err := NewDetector(seedPatterns(), nil)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	for _, u := range []string{"OHYS-RAWS", "Ohys-Raws", "ohys-raws", "oHyS-rAwS"} {
		ep, ok := d.DetectEpisode("[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4", u)
		if !ok || ep != 1 {
			t.Errorf("uploader=%q: ep=%d ok=%v, want 1,true", u, ep, ok)
		}
	}
}

// TestDetect_FallbackPath — unknown uploader, generic regex fires +
// metric increments once with the ORIGINAL (un-lowercased) label.
func TestDetect_FallbackPath(t *testing.T) {
	stub := &stubFallback{}
	d, err := NewDetector(seedPatterns(), stub)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	ep, ok := d.DetectEpisode("Generic Anime - 04 (1080p).mkv", "Unknown")
	if !ok || ep != 4 {
		t.Fatalf("fallback: ep=%d ok=%v, want 4,true", ep, ok)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected exactly one fallback increment, got %d", len(stub.calls))
	}
	if stub.calls[0] != "Unknown" {
		t.Fatalf("fallback label = %q, want %q (un-lowercased)", stub.calls[0], "Unknown")
	}
}

// TestDetect_FallbackEmptyUploader — empty uploader label maps to
// "unknown" so prometheus doesn't see an empty-string label.
func TestDetect_FallbackEmptyUploader(t *testing.T) {
	stub := &stubFallback{}
	d, err := NewDetector(seedPatterns(), stub)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	_, _ = d.DetectEpisode("Anime - 09 (720p).mkv", "")
	if len(stub.calls) != 1 || stub.calls[0] != "unknown" {
		t.Fatalf("empty uploader → expected one fallback call labeled %q; got %v", "unknown", stub.calls)
	}
}

// TestDetect_FallbackBracketShape — generic regex must match both
// "- 01 (" and "- 01 [" shapes.
func TestDetect_FallbackBracketShape(t *testing.T) {
	d, err := NewDetector(seedPatterns(), nil)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	for _, fn := range []string{
		"Title - 07 (something).mkv",
		"Title - 07 [something].mkv",
	} {
		ep, ok := d.DetectEpisode(fn, "Other")
		if !ok || ep != 7 {
			t.Errorf("%q → ep=%d ok=%v, want 7,true", fn, ep, ok)
		}
	}
}

// TestDetect_NoMatch — nonsense filename returns (0, false).
func TestDetect_NoMatch(t *testing.T) {
	d, err := NewDetector(seedPatterns(), nil)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	for _, fn := range []string{
		"no_episode_number.mp4",
		"random.mkv",
		"",
	} {
		ep, ok := d.DetectEpisode(fn, "")
		if ok {
			t.Errorf("expected miss for %q, got ep=%d ok=true", fn, ep)
		}
	}
}

// TestDetect_OutOfRange — captured number outside [1, 9999] → miss.
// Hard to trigger via the regex (capped at 3 digits) but parseEpisode
// is exercised by passing a custom pattern that captures 4+ digits.
func TestDetect_OutOfRange(t *testing.T) {
	patterns := []domain.FilenamePattern{
		{Uploader: "Wide", PatternRegex: `EP(\d+)`, Example: "EP10000.mkv"},
	}
	d, err := NewDetector(patterns, nil)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	_, ok := d.DetectEpisode("EP10000.mkv", "Wide")
	if ok {
		t.Fatalf("episode > 9999 must be rejected as miss")
	}
	// Boundary inside the wide pattern: 9999 is allowed.
	ep, ok := d.DetectEpisode("EP9999.mkv", "Wide")
	if !ok || ep != 9999 {
		t.Fatalf("EP9999 → ep=%d ok=%v, want 9999,true", ep, ok)
	}
}

// TestNewDetector_BadRegex — a row whose regex fails to compile must
// surface at startup as a non-nil error.
func TestNewDetector_BadRegex(t *testing.T) {
	bad := []domain.FilenamePattern{
		{Uploader: "BadGuy", PatternRegex: `(unbalanced`, Example: ""},
	}
	_, err := NewDetector(bad, nil)
	if err == nil {
		t.Fatalf("expected error on bad regex, got nil")
	}
}

// TestNewDetector_PatternMissingCaptureGroup — regex without a
// capture group must fail at construction too.
func TestNewDetector_PatternMissingCaptureGroup(t *testing.T) {
	bad := []domain.FilenamePattern{
		{Uploader: "NoCap", PatternRegex: `\d+`, Example: ""},
	}
	_, err := NewDetector(bad, nil)
	if err == nil {
		t.Fatalf("expected error on regex without capture group, got nil")
	}
}

// TestNewDetectorFromDB_BubblesUpLoadError — the loader error must
// reach the caller (used by main.go fatal-log path).
func TestNewDetectorFromDB_BubblesUpLoadError(t *testing.T) {
	bad := errors.New("simulated db failure")
	_, err := NewDetectorFromDB(context.Background(), &errLoader{err: bad}, nil)
	if err == nil {
		t.Fatalf("expected error from loader to propagate")
	}
}

type errLoader struct{ err error }

func (e *errLoader) LoadAll(_ context.Context) ([]domain.FilenamePattern, error) {
	return nil, e.err
}
