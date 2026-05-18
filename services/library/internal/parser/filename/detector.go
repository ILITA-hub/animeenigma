// Package filename implements per-uploader episode-number extraction
// for torrent payload filenames. The detector loads regex patterns
// once at startup (compiled via regexp.MustCompile-equivalent path)
// and provides DetectEpisode(filename, uploader) → (n, ok).
//
// Flow:
//
//  1. Lookup the uploader-specific pattern (case-insensitive key).
//  2. If absent or it doesn't match, run the generic fallback
//     `- (\d{1,3})\s*[\(\[]`. On fallback match increment a
//     `library_filename_detect_fallback_total{uploader}` counter so
//     ops can spot uploaders that need a dedicated pattern.
//  3. Return (0, false) if neither matches.
//
// Captured episode numbers are clamped to [1, 9999]; values outside
// that range are treated as a non-match.
package filename

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// FallbackMetric is the optional collector incremented when the
// generic fallback regex fires. nil is safe — the detector skips the
// increment in that case (used by tests that don't need a real
// prometheus registry).
type FallbackMetric interface {
	IncFilenameDetectFallback(uploader string)
}

// PatternLoader is the surface NewDetectorFromDB needs from the
// FilenamePatternRepository. Pulled out so tests can substitute a
// stub without spinning up GORM.
type PatternLoader interface {
	LoadAll(ctx context.Context) ([]domain.FilenamePattern, error)
}

// genericFallbackRegex matches "Title - 01 (" and "Title - 01 [".
// Hard-coded at compile time so a bad SQL seed can't break it.
var genericFallbackRegex = regexp.MustCompile(`- (\d{1,3})\s*[\(\[]`)

// Detector holds compiled per-uploader regexes + the generic
// fallback. Safe for concurrent reads after construction.
type Detector struct {
	patterns map[string]*regexp.Regexp // keyed by strings.ToLower(uploader)
	fallback *regexp.Regexp
	metrics  FallbackMetric
}

// NewDetector compiles every loaded row. Returns an error if any
// regex is malformed — bad seed data fails fast at startup, not
// mid-job. metrics may be nil.
func NewDetector(loaded []domain.FilenamePattern, metrics FallbackMetric) (*Detector, error) {
	d := &Detector{
		patterns: make(map[string]*regexp.Regexp, len(loaded)),
		fallback: genericFallbackRegex,
		metrics:  metrics,
	}
	for _, p := range loaded {
		re, err := regexp.Compile(p.PatternRegex)
		if err != nil {
			return nil, fmt.Errorf("compile pattern for uploader %q: %w", p.Uploader, err)
		}
		if re.NumSubexp() < 1 {
			return nil, fmt.Errorf("pattern for uploader %q has no capture group", p.Uploader)
		}
		d.patterns[strings.ToLower(p.Uploader)] = re
	}
	return d, nil
}

// NewDetectorFromDB is the production constructor that loads patterns
// from the DB and forwards to NewDetector. Called once at startup.
func NewDetectorFromDB(ctx context.Context, repo PatternLoader, metrics FallbackMetric) (*Detector, error) {
	rows, err := repo.LoadAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load filename patterns: %w", err)
	}
	return NewDetector(rows, metrics)
}

// DetectEpisode runs the uploader-specific regex first, falls back to
// the generic regex on miss. On a fallback hit the FallbackMetric
// counter is incremented with the ORIGINAL (un-lowercased) uploader
// label so ops can see which uploader is missing dedicated patterns.
// Empty uploader → label "unknown".
//
// Returns (0, false) when no pattern matches OR when the captured
// number falls outside [1, 9999].
func (d *Detector) DetectEpisode(filename, uploader string) (int, bool) {
	if d == nil {
		return 0, false
	}
	// 1. Uploader-specific path.
	if uploader != "" {
		if re, ok := d.patterns[strings.ToLower(uploader)]; ok {
			if m := re.FindStringSubmatch(filename); m != nil && len(m) >= 2 {
				if n, ok := parseEpisode(m[1]); ok {
					return n, true
				}
			}
		}
	}
	// 2. Generic fallback.
	if m := d.fallback.FindStringSubmatch(filename); m != nil && len(m) >= 2 {
		if n, ok := parseEpisode(m[1]); ok {
			if d.metrics != nil {
				label := uploader
				if label == "" {
					label = "unknown"
				}
				d.metrics.IncFilenameDetectFallback(label)
			}
			return n, true
		}
	}
	return 0, false
}

// parseEpisode trims whitespace, runs strconv.Atoi, and clamps to
// [1, 9999]. Returns (n, true) when valid.
func parseEpisode(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	if n < 1 || n > 9999 {
		return 0, false
	}
	return n, true
}
