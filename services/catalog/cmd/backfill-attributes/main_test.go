package main

import (
	"flag"
	"os"
	"testing"
)

// TestParseFlags_Defaults asserts default values match the documented
// flag set in main.go's package doc.
func TestParseFlags_Defaults(t *testing.T) {
	// Reset flag.CommandLine so the test can re-parse cleanly.
	os.Args = []string{"backfill-attributes"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	c := parseFlags()
	if c.DryRun != false {
		t.Errorf("DryRun default: got %v, want false", c.DryRun)
	}
	if c.Limit != 0 {
		t.Errorf("Limit default: got %d, want 0", c.Limit)
	}
	if c.SkipShikimori {
		t.Errorf("SkipShikimori default: got true, want false")
	}
	if c.SkipTags {
		t.Errorf("SkipTags default: got true, want false")
	}
	if c.LogEvery != 100 {
		t.Errorf("LogEvery default: got %d, want 100", c.LogEvery)
	}
	if c.ShikimoriRPS != 3 {
		t.Errorf("ShikimoriRPS default: got %d, want 3", c.ShikimoriRPS)
	}
}

// TestParseFlags_Overrides confirms flag-line overrides reach the
// cliConfig — exercises every flag main.go exposes.
func TestParseFlags_Overrides(t *testing.T) {
	os.Args = []string{
		"backfill-attributes",
		"--dry-run",
		"--limit=50",
		"--skip-shikimori",
		"--skip-tags",
		"--log-every=10",
		"--shikimori-rps=5",
	}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	c := parseFlags()
	if !c.DryRun {
		t.Error("DryRun: expected true with --dry-run")
	}
	if c.Limit != 50 {
		t.Errorf("Limit: got %d, want 50", c.Limit)
	}
	if !c.SkipShikimori {
		t.Error("SkipShikimori: expected true with --skip-shikimori")
	}
	if !c.SkipTags {
		t.Error("SkipTags: expected true with --skip-tags")
	}
	if c.LogEvery != 10 {
		t.Errorf("LogEvery: got %d, want 10", c.LogEvery)
	}
	if c.ShikimoriRPS != 5 {
		t.Errorf("ShikimoriRPS: got %d, want 5", c.ShikimoriRPS)
	}
}
