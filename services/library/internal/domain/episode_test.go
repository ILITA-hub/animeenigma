package domain

import "testing"

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

func TestEpisodeTableNameUnchanged(t *testing.T) {
	if (Episode{}).TableName() != "library_episodes" {
		t.Fatalf("Episode.TableName() = %q, want %q", (Episode{}).TableName(), "library_episodes")
	}
}
