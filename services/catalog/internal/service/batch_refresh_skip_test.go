package service

// Unit test for sameStringSet, the genre-set equality check that lets
// BatchRefreshAnime skip rewriting an unchanged anime_genres join row.

import "testing"

func TestSameStringSet(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both empty", nil, []string{}, true},
		{"same order", []string{"1", "2"}, []string{"1", "2"}, true},
		{"reordered", []string{"2", "1", "3"}, []string{"3", "1", "2"}, true},
		{"duplicates collapse", []string{"1", "1", "2"}, []string{"2", "1"}, true},
		{"disjoint", []string{"1"}, []string{"2"}, false},
		{"subset", []string{"1", "2"}, []string{"1"}, false},
		{"superset", []string{"1"}, []string{"1", "2"}, false},
		{"empty vs one", nil, []string{"1"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameStringSet(c.a, c.b); got != c.want {
				t.Errorf("sameStringSet(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}
