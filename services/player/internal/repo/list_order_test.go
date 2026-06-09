package repo

import "testing"

func TestSanitizedOrderClause(t *testing.T) {
	cases := []struct {
		name, sort, order, want string
	}{
		{"invalid field falls back", "; DROP TABLE", "asc", "updated_at DESC"},
		{"invalid dir falls back to DESC", "score", "sideways", "anime_list.score DESC"},
		{"score asc", "score", "asc", "anime_list.score ASC"},
		{"status desc", "status", "desc", "anime_list.status DESC"},
		{"title joins animes", "title", "asc", "animes.name ASC"},
		{"genre orders by derived join column", "genre", "asc", genreSortOrderColumn + " ASC"},
		{"genre desc", "genre", "desc", genreSortOrderColumn + " DESC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizedOrderClause(c.sort, c.order); got != c.want {
				t.Errorf("sanitizedOrderClause(%q,%q) = %q, want %q", c.sort, c.order, got, c.want)
			}
		})
	}
}

func TestGenreIsAllowedSortField(t *testing.T) {
	if !allowedSortFields["genre"] {
		t.Fatal("genre must be an allowed sort field")
	}
}
