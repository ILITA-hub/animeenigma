package repo

import (
	"strings"
	"testing"
)

func TestKeyRoom(t *testing.T) {
	t.Parallel()
	got := KeyRoom("abc")
	want := "wt:room:abc"
	if got != want {
		t.Errorf("KeyRoom(\"abc\") = %q; want %q", got, want)
	}
}

func TestKeyRoomMembers(t *testing.T) {
	t.Parallel()
	got := KeyRoomMembers("abc")
	want := "wt:room:abc:members"
	if got != want {
		t.Errorf("KeyRoomMembers(\"abc\") = %q; want %q", got, want)
	}
}

func TestKeyRoomMessages(t *testing.T) {
	t.Parallel()
	got := KeyRoomMessages("abc")
	want := "wt:room:abc:messages"
	if got != want {
		t.Errorf("KeyRoomMessages(\"abc\") = %q; want %q", got, want)
	}
}

func TestKeyRoomEvents(t *testing.T) {
	t.Parallel()
	got := KeyRoomEvents("abc")
	want := "wt:room:abc:events"
	if got != want {
		t.Errorf("KeyRoomEvents(\"abc\") = %q; want %q", got, want)
	}
}

func TestKeyAllStartWithPrefix(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"KeyRoom":         KeyRoom("xyz"),
		"KeyRoomMembers":  KeyRoomMembers("xyz"),
		"KeyRoomMessages": KeyRoomMessages("xyz"),
		"KeyRoomEvents":   KeyRoomEvents("xyz"),
	}
	for name, key := range cases {
		if !strings.HasPrefix(key, KeyPrefix) {
			t.Errorf("%s returned %q; want prefix %q", name, key, KeyPrefix)
		}
	}
}

func TestKeyPrefixConst(t *testing.T) {
	t.Parallel()
	if KeyPrefix != "wt:" {
		t.Errorf("KeyPrefix = %q; want \"wt:\"", KeyPrefix)
	}
}

// TestKeyRoomIDVariants verifies that arbitrary room ID values are placed
// verbatim into the key (no escaping). UUIDs are the production shape.
func TestKeyRoomIDVariants(t *testing.T) {
	t.Parallel()
	for _, id := range []string{
		"550e8400-e29b-41d4-a716-446655440000", // canonical UUID
		"short",
		"with-dashes-and-numbers-123",
	} {
		if got := KeyRoom(id); got != "wt:room:"+id {
			t.Errorf("KeyRoom(%q) = %q; want %q", id, got, "wt:room:"+id)
		}
	}
}
