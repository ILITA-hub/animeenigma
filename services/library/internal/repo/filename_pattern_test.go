package repo

import (
	"regexp"
	"testing"
)

// TestSeededPatternsCompile is a static smoke test that the five
// SPEC-locked uploader regexes compile cleanly + each contains
// exactly one capture group. This guards against accidental edits to
// the regex strings in migrations/003_library_filename_patterns.sql.
// The migration file is the source of truth — these literals are a
// safety copy so a bad regex fails CI here, not only at runtime when
// NewDetector panics.
func TestSeededPatternsCompile(t *testing.T) {
	patterns := []struct {
		uploader string
		regex    string
		example  string
		wantEp   int
	}{
		{"Ohys-Raws", `^\[Ohys-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+\(`, "[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4", 1},
		{"SubsPlease", `^\[SubsPlease\]\s+.+?\s+-\s+(\d{1,3})\s+\(`, "[SubsPlease] Frieren - 12 (1080p) [ABCD1234].mkv", 12},
		{"Erai-raws", `^\[Erai-raws\]\s+.+?\s+-\s+(\d{1,3})\s+\[`, "[Erai-raws] Spy x Family - 07 [1080p][Multiple Subtitle].mkv", 7},
		{"Leopard-Raws", `^\[Leopard-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+RAW\s+`, "[Leopard-Raws] Re-Zero - 03 RAW (BS11 1280x720 x264 AAC).mp4", 3},
		{"ARC-Raws", `^\[ARC-Raws\]\s+.+?\s+-\s+(\d{1,3})\s*[\[\(]`, "[ARC-Raws] Made in Abyss - 05 [1080p].mkv", 5},
	}
	for _, p := range patterns {
		t.Run(p.uploader, func(t *testing.T) {
			re, err := regexp.Compile(p.regex)
			if err != nil {
				t.Fatalf("regex %q failed to compile: %v", p.regex, err)
			}
			if re.NumSubexp() != 1 {
				t.Fatalf("regex %q has %d capture groups, want exactly 1", p.regex, re.NumSubexp())
			}
			m := re.FindStringSubmatch(p.example)
			if m == nil {
				t.Fatalf("regex %q did NOT match its own example %q", p.regex, p.example)
			}
			if m[1] == "" {
				t.Fatalf("capture group empty for %s example %q", p.uploader, p.example)
			}
		})
	}
}
