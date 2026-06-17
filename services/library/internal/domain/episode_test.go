package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The enum const string values MUST match the episode_source / episode_track
// labels in migrations/005_autocache_pool.sql exactly (the GORM model mirrors
// the SQL 1:1 — no AutoMigrate).
func TestEpisodeSourceValues(t *testing.T) {
	if EpisodeSourceAdmin != "admin" {
		t.Fatalf("EpisodeSourceAdmin = %q, want %q", EpisodeSourceAdmin, "admin")
	}
	if EpisodeSourceAutocache != "autocache" {
		t.Fatalf("EpisodeSourceAutocache = %q, want %q", EpisodeSourceAutocache, "autocache")
	}
}

func TestEpisodeTrackValues(t *testing.T) {
	if EpisodeTrackRaw != "raw" {
		t.Fatalf("EpisodeTrackRaw = %q, want %q", EpisodeTrackRaw, "raw")
	}
	// sub/dub are reserved (D2) — present in the enum, never written in v1.
	if EpisodeTrackSub != "sub" {
		t.Fatalf("EpisodeTrackSub = %q, want %q", EpisodeTrackSub, "sub")
	}
	if EpisodeTrackDub != "dub" {
		t.Fatalf("EpisodeTrackDub = %q, want %q", EpisodeTrackDub, "dub")
	}
}

// CR-01 regression: downloaded_at is a NULLABLE column (migration 005), so the
// field must be a *time.Time. A non-pointer time.Time would make GORM insert the
// Go zero value (year 0001) instead of NULL whenever a Create omits it, poisoning
// the budget/freshness ledger. This test proves the omitted value serializes as
// absent (nil → NULL), never as 0001-01-01.
func TestEpisodeDownloadedAtNullableRoundTrip(t *testing.T) {
	// Omitted DownloadedAt → omitted from JSON (omitempty on a nil pointer),
	// and never the year-0001 zero value.
	var ep Episode
	if ep.DownloadedAt != nil {
		t.Fatalf("zero-value Episode.DownloadedAt = %v, want nil (nullable)", ep.DownloadedAt)
	}
	b, err := json.Marshal(ep)
	if err != nil {
		t.Fatalf("marshal episode: %v", err)
	}
	// The nil pointer must be omitted entirely — never serialized as the
	// year-0001 zero value a non-pointer time.Time would produce.
	if strings.Contains(string(b), "downloaded_at") {
		t.Fatalf("nil DownloadedAt must be omitted from JSON, got: %s", b)
	}

	// A set DownloadedAt round-trips faithfully.
	want := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	ep.DownloadedAt = &want
	b, err = json.Marshal(ep)
	if err != nil {
		t.Fatalf("marshal episode with downloaded_at: %v", err)
	}
	var back Episode
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal episode: %v", err)
	}
	if back.DownloadedAt == nil || !back.DownloadedAt.Equal(want) {
		t.Fatalf("DownloadedAt round-trip = %v, want %v", back.DownloadedAt, want)
	}
}

func TestEpisodeTableNameUnchanged(t *testing.T) {
	if (Episode{}).TableName() != "library_episodes" {
		t.Fatalf("Episode.TableName() = %q, want %q", (Episode{}).TableName(), "library_episodes")
	}
}
