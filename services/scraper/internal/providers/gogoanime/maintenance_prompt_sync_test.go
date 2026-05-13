// maintenance_prompt_sync_test.go — Phase 21 D7 invariant: every
// libs/streamprobe.Reason value must appear as a literal substring in
// .claude/maintenance-prompt.md so the maintenance bot's reason-enum
// dispatch (Patterns 6/7 + "Scraper Playability Regression" alert) has
// a fix path for every possible failure mode.
//
// Drift surfaces at CI time, not in production. If a new Reason value is
// added to libs/streamprobe without updating the prompt, this test
// fails — the developer must update both in lock-step.
package gogoanime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

// TestMaintenancePromptCoversAllReasons enforces the SCRAPER-HEAL D7
// invariant: every libs/streamprobe.Reason value (except the
// success-only ReasonPlayable, which is not a failure-mode dispatch)
// must appear in .claude/maintenance-prompt.md.
//
// When the file is unreadable (some CI container layouts), the test
// Skip's rather than fails — keeps the test useful locally without
// blocking container builds where the file path may differ.
func TestMaintenancePromptCoversAllReasons(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// services/scraper/internal/providers/gogoanime/ → 5 levels up to repo root.
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
	promptPath := filepath.Join(repoRoot, ".claude", "maintenance-prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Skipf("maintenance-prompt.md not readable at %s: %v", promptPath, err)
	}
	content := string(data)

	for _, r := range streamprobe.AllReasons() {
		// ReasonPlayable is the success case — not a failure-mode dispatch.
		// The bot only needs fix paths for the SIX failure reasons.
		if r == streamprobe.ReasonPlayable {
			continue
		}
		if !strings.Contains(content, string(r)) {
			t.Errorf("maintenance-prompt.md does not mention Reason=%q; "+
				"add it to the reason-enum dispatch table (Patterns 6/7 or "+
				"the Scraper Playability Regression section)", r)
		}
	}
}
