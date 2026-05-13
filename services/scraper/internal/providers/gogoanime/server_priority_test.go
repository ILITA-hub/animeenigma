package gogoanime

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// TestValidatePriorityList_AcceptsKnown — happy path: every priority entry
// matches a known extractor name. nil error.
func TestValidatePriorityList_AcceptsKnown(t *testing.T) {
	t.Parallel()
	known := []string{"streamhg", "earnvids", "vibeplayer", "kwik", "megacloud"}
	priority := []string{"streamhg", "earnvids", "vibeplayer"}
	if err := ValidatePriorityList(priority, known); err != nil {
		t.Fatalf("ValidatePriorityList: %v; want nil", err)
	}
}

// TestValidatePriorityList_RejectsTypo — CONTEXT.md risks: a single typo'd
// entry surfaces in the error. The boot log MUST name the typo'd value so
// operators can grep and fix the env.
func TestValidatePriorityList_RejectsTypo(t *testing.T) {
	t.Parallel()
	known := []string{"streamhg", "earnvids", "vibeplayer", "kwik", "megacloud"}
	priority := []string{"streamg", "earnvids", "vibeplayer"} // streamg = typo of streamhg
	err := ValidatePriorityList(priority, known)
	if err == nil {
		t.Fatal("ValidatePriorityList: nil; want error for typo'd entry")
	}
	msg := err.Error()
	if !strings.Contains(msg, "streamg") {
		t.Errorf("error %q must contain the typo'd value 'streamg' so operators see their mistake", msg)
	}
	if !strings.Contains(msg, "unknown server") {
		t.Errorf("error %q must contain the literal 'unknown server' phrasing for log-grep ergonomics", msg)
	}
}

// TestValidatePriorityList_AllUnknown — multiple unknown entries are all
// listed in the error message so operators see every typo at once.
func TestValidatePriorityList_AllUnknown(t *testing.T) {
	t.Parallel()
	known := []string{"streamhg", "earnvids", "vibeplayer"}
	priority := []string{"foo", "bar", "baz"}
	err := ValidatePriorityList(priority, known)
	if err == nil {
		t.Fatal("nil error; want non-nil")
	}
	msg := err.Error()
	for _, want := range []string{"foo", "bar", "baz"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q must contain %q", msg, want)
		}
	}
}

// TestValidatePriorityList_CaseInsensitive — case is folded for matching.
func TestValidatePriorityList_CaseInsensitive(t *testing.T) {
	t.Parallel()
	known := []string{"streamhg", "earnvids"}
	priority := []string{"StreamHG", "EARNVIDS"}
	if err := ValidatePriorityList(priority, known); err != nil {
		t.Fatalf("ValidatePriorityList: %v; want nil", err)
	}
}

// TestSortByPriority_TableTests — the determinism + tie-breaking matrix.
func TestSortByPriority_TableTests(t *testing.T) {
	t.Parallel()
	// hostExtractor mimics what main.go will build at boot from the embeds
	// registry's host slices.
	host := map[string]string{
		"otakuhg.site":    "streamhg",
		"otakuvid.online": "earnvids",
		"vibeplayer.site": "vibeplayer",
	}

	server := func(name, urlStr string) domain.Server {
		return domain.Server{ID: urlStr, Name: name, Type: domain.CategorySub}
	}
	hg := server("HD-1", "https://otakuhg.site/e/abc")
	ev := server("HD-2", "https://otakuvid.online/e/def")
	vp := server("StreamX", "https://vibeplayer.site/e/ghi")
	unknown := server("MystrySrv", "https://unknown.example/e/jkl")

	cases := []struct {
		name     string
		input    []domain.Server
		priority []string
		want     []string // expected URL order
	}{
		{
			name:     "exact priority order — already sorted",
			input:    []domain.Server{hg, ev, vp},
			priority: []string{"streamhg", "earnvids", "vibeplayer"},
			want:     []string{hg.ID, ev.ID, vp.ID},
		},
		{
			name:     "reverse input — sorter pulls priority to front",
			input:    []domain.Server{vp, ev, hg},
			priority: []string{"streamhg", "earnvids", "vibeplayer"},
			want:     []string{hg.ID, ev.ID, vp.ID},
		},
		{
			name:     "custom priority — earnvids first",
			input:    []domain.Server{hg, ev, vp},
			priority: []string{"earnvids", "streamhg", "vibeplayer"},
			want:     []string{ev.ID, hg.ID, vp.ID},
		},
		{
			name:     "unknown server trails — original-index tiebreaker",
			input:    []domain.Server{unknown, hg, ev, vp},
			priority: []string{"streamhg", "earnvids", "vibeplayer"},
			want:     []string{hg.ID, ev.ID, vp.ID, unknown.ID},
		},
		{
			name:     "two unknowns — stable original order preserved",
			input:    []domain.Server{server("U1", "https://a.test/e/1"), hg, server("U2", "https://b.test/e/2")},
			priority: []string{"streamhg"},
			want:     []string{hg.ID, "https://a.test/e/1", "https://b.test/e/2"},
		},
		{
			name:     "empty priority — everything is 'unknown', original order preserved",
			input:    []domain.Server{vp, hg, ev},
			priority: []string{},
			want:     []string{vp.ID, hg.ID, ev.ID},
		},
		{
			name:     "priority entry not in any server — others still sort",
			input:    []domain.Server{vp, hg},
			priority: []string{"missing", "streamhg", "vibeplayer"},
			want:     []string{hg.ID, vp.ID},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := SortByPriority(tc.input, tc.priority, host)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d; want %d (got %v)", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if got[i].ID != want {
					t.Errorf("got[%d].ID = %q; want %q (full result: %v)", i, got[i].ID, want, got)
				}
			}
		})
	}
}

// TestSortByPriority_Determinism — running the sort twice on the same input
// must produce identical output. Locks the documented determinism guarantee.
func TestSortByPriority_Determinism(t *testing.T) {
	t.Parallel()
	host := map[string]string{
		"otakuhg.site":    "streamhg",
		"otakuvid.online": "earnvids",
		"vibeplayer.site": "vibeplayer",
	}
	servers := []domain.Server{
		{ID: "https://vibeplayer.site/x"},
		{ID: "https://otakuvid.online/y"},
		{ID: "https://otakuhg.site/z"},
		{ID: "https://other.test/w"},
	}
	priority := []string{"streamhg", "earnvids", "vibeplayer"}
	a := SortByPriority(servers, priority, host)
	b := SortByPriority(servers, priority, host)
	if len(a) != len(b) {
		t.Fatalf("len mismatch: a=%d b=%d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Errorf("non-deterministic: pos %d a=%q b=%q", i, a[i].ID, b[i].ID)
		}
	}
}

// TestSortByPriority_DoesNotMutateInput — SortByPriority returns a NEW
// slice. The caller's input is left in its original order.
func TestSortByPriority_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	host := map[string]string{
		"otakuhg.site":    "streamhg",
		"vibeplayer.site": "vibeplayer",
	}
	input := []domain.Server{
		{ID: "https://vibeplayer.site/x"},
		{ID: "https://otakuhg.site/y"},
	}
	inputBefore := []string{input[0].ID, input[1].ID}
	_ = SortByPriority(input, []string{"streamhg", "vibeplayer"}, host)
	if input[0].ID != inputBefore[0] || input[1].ID != inputBefore[1] {
		t.Errorf("SortByPriority mutated the caller's slice: before=%v after=[%s, %s]",
			inputBefore, input[0].ID, input[1].ID)
	}
}

// TestHostnameToExtractorName_SuffixMatch — *.host suffix matches resolve
// to the parent extractor name.
func TestHostnameToExtractorName_SuffixMatch(t *testing.T) {
	t.Parallel()
	host := map[string]string{
		"otakuhg.site": "streamhg",
	}
	if got := hostnameToExtractorName("https://cdn.otakuhg.site/e/abc", host); got != "streamhg" {
		t.Errorf("hostnameToExtractorName(cdn.otakuhg.site) = %q; want streamhg (suffix match)", got)
	}
}

// TestHostnameToExtractorName_NoMatch — returns "" when neither exact nor
// suffix matches.
func TestHostnameToExtractorName_NoMatch(t *testing.T) {
	t.Parallel()
	host := map[string]string{
		"otakuhg.site": "streamhg",
	}
	if got := hostnameToExtractorName("https://example.com/e/abc", host); got != "" {
		t.Errorf("hostnameToExtractorName(example.com) = %q; want empty string", got)
	}
}
