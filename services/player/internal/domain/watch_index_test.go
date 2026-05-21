package domain

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestWatchProgress_IndexTag_OnUpdatedAt asserts that WatchProgress.UpdatedAt
// carries the GORM tag declaring the dedicated B-tree index on updated_at
// (HSB-NF-02). The Phase 3 now_watching resolver issues
// `WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'`; without this index
// the predicate degrades to a sequential scan as watch_progress grows.
//
// This test guards against accidental future removal of the tag — the
// index is created by AutoMigrate on player restart, so silently dropping
// the tag would silently lose the index next deploy.
func TestWatchProgress_IndexTag_OnUpdatedAt(t *testing.T) {
	typ := reflect.TypeOf(WatchProgress{})
	field, ok := typ.FieldByName("UpdatedAt")
	if !ok {
		t.Fatalf("WatchProgress.UpdatedAt field missing — domain model regressed")
	}
	gormTag := field.Tag.Get("gorm")
	wantSubstring := "index:idx_watch_progress_updated_at"
	if !strings.Contains(gormTag, wantSubstring) {
		t.Fatalf("WatchProgress.UpdatedAt missing index tag\n  want substring: %q\n  got gorm tag:    %q",
			wantSubstring, gormTag)
	}
}

// TestInternalListItem_HasExpectedJSONFields pins the JSON contract for the
// internal player endpoint's response item shape. The catalog spotlight
// aggregator (workstream hero-spotlight v1.0 Phase 3) consumes this shape
// via player_client.go; a silent JSON-tag rename would break that consumer.
//
// We assert presence (not exact tag string) so an `omitempty` change does
// not trip the test, but a key rename does.
func TestInternalListItem_HasExpectedJSONFields(t *testing.T) {
	typ := reflect.TypeOf(InternalListItem{})
	want := []string{
		"anime_id",
		"name",
		"name_ru",
		"poster_url",
		"episodes_aired",
		"episodes_count",
		"status",
		"last_watched_episode",
		"updated_at",
	}

	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		// Strip ",omitempty" / ",string" suffix — we only care about the key.
		if comma := strings.IndexByte(tag, ','); comma >= 0 {
			tag = tag[:comma]
		}
		if tag != "" && tag != "-" {
			got = append(got, tag)
		}
	}

	for _, key := range want {
		if !slices.Contains(got, key) {
			t.Errorf("InternalListItem JSON contract missing key %q (got tags: %v)", key, got)
		}
	}
}
