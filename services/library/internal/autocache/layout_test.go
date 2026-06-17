package autocache

import (
	"strings"
	"testing"
)

func TestRawPrefix(t *testing.T) {
	cases := []struct {
		name    string
		malID   string
		episode int
		want    string
	}{
		{name: "basic", malID: "12345", episode: 3, want: "aeProvider/12345/RAW/3/"},
		// MALID == shikimori_id passthrough — no remapping (CONTEXT line 42).
		{name: "shikimori_passthrough", malID: "57466", episode: 12, want: "aeProvider/57466/RAW/12/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RawPrefix(tc.malID, tc.episode)
			if got != tc.want {
				t.Fatalf("RawPrefix(%q, %d) = %q, want %q", tc.malID, tc.episode, got, tc.want)
			}
		})
	}
}

func TestRawPrefixTrailingSlash(t *testing.T) {
	// Move/Upload force a trailing slash; the helper must already supply one so
	// minio.Writer accepts the prefix unchanged.
	got := RawPrefix("999", 1)
	if !strings.HasSuffix(got, "/") {
		t.Fatalf("RawPrefix output %q must end with a trailing slash", got)
	}
}
