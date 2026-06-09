package domain

import "testing"

// Guards the my-list sort whitelist. "genre" must survive Validate() — it was
// silently reset to "updated_at" here (not in the repo) which made the genre
// sort a no-op despite the repo supporting it.
func TestPaginationParams_Validate_SortWhitelist(t *testing.T) {
	keep := []string{"updated_at", "created_at", "score", "status", "episodes", "title", "genre"}
	for _, s := range keep {
		p := PaginationParams{Page: 1, PerPage: 24, Sort: s, Order: "asc"}
		p.Validate()
		if p.Sort != s {
			t.Errorf("Validate() reset allowed sort %q to %q", s, p.Sort)
		}
	}

	// Unknown / injection attempts fall back to the safe default.
	bad := PaginationParams{Sort: "; DROP TABLE", Order: "asc"}
	bad.Validate()
	if bad.Sort != "updated_at" {
		t.Errorf("Validate() = %q for invalid sort, want updated_at", bad.Sort)
	}
}
