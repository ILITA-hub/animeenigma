package config

import (
	"reflect"
	"testing"
)

func TestParseRPOrigins(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "single origin",
			raw:  "https://animeenigma.org",
			want: []string{"https://animeenigma.org"},
		},
		{
			name: "comma separated, no whitespace",
			raw:  "https://a.com,https://b.com",
			want: []string{"https://a.com", "https://b.com"},
		},
		{
			name: "whitespace around commas is trimmed",
			raw:  "https://a.com, https://b.com ,  https://c.com",
			want: []string{"https://a.com", "https://b.com", "https://c.com"},
		},
		{
			name: "empty entries from stray commas are dropped",
			raw:  "https://a.com,,https://b.com,",
			want: []string{"https://a.com", "https://b.com"},
		},
		{
			name: "blank input yields no origins",
			raw:  "",
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRPOrigins(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseRPOrigins(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}
