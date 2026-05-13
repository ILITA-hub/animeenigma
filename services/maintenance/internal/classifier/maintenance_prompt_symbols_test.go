// Package classifier — symbol-stability tests for the v3.1 Scraper Self-
// Healing maintenance bot dispatch path.
//
// SCRAPER-HEAL-16 + Phase 23 CONTEXT.md D6:
//   .claude/maintenance-prompt.md is shipped state. This phase does NOT
//   edit it. Instead, we assert here that:
//     (a) Pattern 6, Pattern 7, and the Scraper Playability Regression
//         sections are still present in the prompt body.
//     (b) Every libs/streamprobe.Reason value appears textually in the
//         prompt (so the bot's reason-enum dispatch table covers every
//         case the canary can emit).
//     (c) The Go symbols the prompt references for stream-cache TTL
//         tuning (`cacheStream` / `computeStreamTTL`) still exist in
//         services/scraper/internal/providers/gogoanime/ so the prompt's
//         hint resolves to real code.
//
// Failing any of these tests is the alarm — the operator's fix is either
// to rename the Go symbol back, OR to edit .claude/maintenance-prompt.md
// to reference the new symbol name (the latter being a follow-up, NOT
// done in this phase per D6).
//
// Path resolution: the test discovers the project root by walking up
// from its own file location until a `.claude/maintenance-prompt.md` is
// found, OR by reading ANIMEENIGMA_ROOT if set (fallback for non-
// standard CI layouts).
package classifier

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

// projectRoot walks up from the test source-file directory until it
// finds a directory containing .claude/maintenance-prompt.md, OR returns
// $ANIMEENIGMA_ROOT if set. Returns an empty string + an error if
// neither path resolves to a readable file.
func projectRoot(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("ANIMEENIGMA_ROOT"); env != "" {
		if _, err := os.Stat(filepath.Join(env, ".claude", "maintenance-prompt.md")); err == nil {
			return env
		}
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed — cannot resolve project root")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".claude", "maintenance-prompt.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate project root containing .claude/maintenance-prompt.md (started from %s); set ANIMEENIGMA_ROOT", file)
	return ""
}

// readPrompt returns the bytes of .claude/maintenance-prompt.md or fails
// the test with a path-anchored error message.
func readPrompt(t *testing.T) []byte {
	t.Helper()
	root := projectRoot(t)
	path := filepath.Join(root, ".claude", "maintenance-prompt.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(b) == 0 {
		t.Fatalf("%s is empty", path)
	}
	return b
}

// TestMaintenancePrompt_FilePresentInWorkingDir is a sanity test that
// the prompt file is reachable from the test's working directory via the
// projectRoot walker. Failure-message names the resolved path so a
// relocation can be diagnosed quickly.
func TestMaintenancePrompt_FilePresentInWorkingDir(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, ".claude", "maintenance-prompt.md")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() < 1024 {
		t.Errorf("%s is suspiciously small (%d bytes); expected ≥ 1KB of prompt content", path, info.Size())
	}
}

// TestMaintenancePrompt_ContainsPatterns6And7 asserts the three section
// headings the maintenance-bot's dispatcher matches on are all present.
// SCRAPER-HEAL-16.
func TestMaintenancePrompt_ContainsPatterns6And7(t *testing.T) {
	content := string(readPrompt(t))
	required := []string{
		"### Pattern 6:",
		"### Pattern 7:",
		"### Scraper Playability Regression",
	}
	for _, needle := range required {
		if !strings.Contains(content, needle) {
			t.Errorf("maintenance-prompt missing required section heading: %q", needle)
		}
	}
}

// TestMaintenancePrompt_AllReasonsCovered loops over every value the
// canary can emit on the `reason` label (the libs/streamprobe.Reason
// enum) and asserts each one appears textually in the prompt body. A new
// reason added to the enum without updating the prompt fails this test.
//
// Special-case: ReasonStatus403 ("status_403") MAY appear in the prompt
// as either the literal "status_403" OR the human-readable variant
// "403_upstream" — the prompt at line 170/174 uses the slash form
// `status_403 / 403_upstream`. Both forms count as covered.
func TestMaintenancePrompt_AllReasonsCovered(t *testing.T) {
	content := string(readPrompt(t))
	// Iterate libs/streamprobe.AllReasons() so a new Reason added to the
	// enum without prompt update fails this test automatically — a
	// libs-driven drift detector. Per-reason aliases below map the
	// canonical Prometheus-label form to the human-readable variants the
	// prompt may use ("status_403" appears as "status_403 / 403_upstream"
	// at line 174, for example). Both forms count as covered.
	aliasesByReason := map[streamprobe.Reason][]string{
		streamprobe.ReasonPlayable:         {"playability"},
		streamprobe.ReasonAdDecoy:          {"ad-decoy"},
		streamprobe.ReasonZeroMatch:        {"zero-match"},
		streamprobe.ReasonStatus403:        {"403_upstream", "403 upstream"},
		streamprobe.ReasonSignedURLExpired: {"signed-url-expired"},
		streamprobe.ReasonCDNUnreachable:   {"cdn-unreachable"},
		streamprobe.ReasonEmptyResponse:    {"empty-response"},
	}
	for _, reason := range streamprobe.AllReasons() {
		canonical := string(reason)
		found := strings.Contains(content, canonical)
		aliases := aliasesByReason[reason]
		if !found {
			for _, alias := range aliases {
				if strings.Contains(content, alias) {
					found = true
					t.Logf("reason %q covered via alias %q", canonical, alias)
					break
				}
			}
		}
		if !found {
			t.Errorf("maintenance-prompt does not mention reason %q (or any alias %v) — dispatch table is incomplete", canonical, aliases)
		}
	}
}

// TestScraperGoSymbols_StillExist greps every .go file under
// services/scraper/internal/providers/gogoanime/ for the two symbols
// the maintenance-prompt's "signed_url_expired" guidance references
// (line 173: "search for `cacheStream` / `computeStreamTTL`"). At least
// ONE of the two MUST appear in some .go file — both is preferred, one
// is acceptable (the prompt's slash means "either grounds the hint").
//
// If BOTH names have been refactored away, the test fails with a list of
// missing names. The fix is either to rename one back, or to update the
// prompt to reference the new symbol name (a P-23 follow-up — NOT done
// in this phase per CONTEXT.md D6).
func TestScraperGoSymbols_StillExist(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, "services", "scraper", "internal", "providers", "gogoanime")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("scraper gogoanime dir not found at %s: %v", dir, err)
	}

	symbols := []string{"cacheStream", "computeStreamTTL"}
	found := map[string]bool{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip test files — we want the production symbol presence, not
		// test-only stub names that happen to match.
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		for _, sym := range symbols {
			if bytes.Contains(content, []byte(sym)) {
				found[sym] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}

	// At least one of the two named symbols MUST be present — that's what
	// grounds the prompt's hint to real code. If both are missing, the
	// hint resolves to dead code and the dispatcher's signed_url_expired
	// guidance is wrong.
	anyFound := false
	for _, sym := range symbols {
		if found[sym] {
			anyFound = true
		}
	}
	if !anyFound {
		t.Fatalf("none of %v found in any non-test .go file under %s — .claude/maintenance-prompt.md line 173 references dead code. Either rename the symbol back, or update the prompt (P-23 follow-up).", symbols, dir)
	}

	// Log the per-symbol presence so a future refactor that drops one of
	// the two leaves a breadcrumb in test output. Don't fail — the prompt
	// uses slash semantics ("cacheStream / computeStreamTTL") so finding
	// either one keeps the hint grounded.
	for _, sym := range symbols {
		if found[sym] {
			t.Logf("symbol %q is present (prompt hint grounded)", sym)
		} else {
			t.Logf("symbol %q is ABSENT — prompt slash-alternative still satisfied by surviving symbol(s); follow-up to update the prompt is acceptable", sym)
		}
	}
}
