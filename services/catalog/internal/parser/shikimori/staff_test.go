package shikimori

import "testing"

func TestFilterStaffRoles(t *testing.T) {
	// Director is whitelisted (rank 0); Key Animation is not → dropped.
	got := filterStaffRoles(
		[]string{"Key Animation", "Director"},
		[]string{"Аниматор", "Режиссёр"},
	)
	if len(got) != 1 {
		t.Fatalf("want 1 kept role, got %d (%+v)", len(got), got)
	}
	if got[0].Role != "Director" || got[0].RoleRU != "Режиссёр" || got[0].Rank != 0 {
		t.Fatalf("bad mapping: %+v", got[0])
	}

	// A person with two whitelisted roles yields two entries.
	got = filterStaffRoles(
		[]string{"Script", "Series Composition"},
		[]string{"Сценарий", "Компоновка серий"},
	)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}

	// rolesRu shorter than rolesEn → RoleRU is empty, no panic.
	got = filterStaffRoles([]string{"Music"}, nil)
	if len(got) != 1 || got[0].RoleRU != "" {
		t.Fatalf("nil rolesRu handling: %+v", got)
	}

	// Nothing whitelisted → empty.
	if got = filterStaffRoles([]string{"In-Between Animation"}, []string{"x"}); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}
