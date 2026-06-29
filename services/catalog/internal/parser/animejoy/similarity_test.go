package animejoy

import (
	"strings"
	"testing"
)

func TestJaroWinklerIdentical(t *testing.T) {
	if got := jaroWinkler("frieren", "frieren"); got != 1.0 {
		t.Fatalf("identical strings: want 1.0, got %v", got)
	}
	// Cyrillic identical.
	if got := jaroWinkler("фрирен", "фрирен"); got != 1.0 {
		t.Fatalf("identical cyrillic: want 1.0, got %v", got)
	}
}

func TestJaroWinklerDisjoint(t *testing.T) {
	if got := jaroWinkler("abc", "xyz"); got != 0.0 {
		t.Fatalf("disjoint strings: want 0.0, got %v", got)
	}
	if got := jaroWinkler("", "frieren"); got != 0.0 {
		t.Fatalf("empty operand: want 0.0, got %v", got)
	}
}

func TestJaroWinklerCloseMatchAboveThreshold(t *testing.T) {
	// A near-identical title should clear the 0.85 fuzzy gate.
	if got := jaroWinkler("code geass r2", "code geass r2 "); got < 0.85 {
		t.Fatalf("near-identical below threshold: got %v", got)
	}
}

func TestFoldSeasonStripsCyrillicSeasonMarker(t *testing.T) {
	got := foldSeason("Код Гиас: Восставший Лелуш (2 сезон)")
	if strings.Contains(got, "сезон") {
		t.Fatalf("season marker not stripped: %q", got)
	}
	if strings.Contains(got, "2") {
		t.Fatalf("season number leaked through: %q", got)
	}
	if strings.ContainsAny(got, ":()") {
		t.Fatalf("punctuation not collapsed: %q", got)
	}
	// The salient title words must survive.
	for _, w := range []string{"код", "гиас", "восставший", "лелуш"} {
		if !strings.Contains(got, w) {
			t.Fatalf("dropped title word %q from %q", w, got)
		}
	}
}

func TestFoldSeasonStripsBracketCounterAndPart(t *testing.T) {
	got := foldSeason("Провожающая в последний путь Фрирен (1 сезон) [28 из 28]")
	if strings.ContainsAny(got, "[]") || strings.Contains(got, "28") {
		t.Fatalf("bracket counter not stripped: %q", got)
	}
	if strings.Contains(got, "сезон") {
		t.Fatalf("season marker not stripped: %q", got)
	}
	if !strings.Contains(got, "фрирен") {
		t.Fatalf("expected 'фрирен' in %q", got)
	}

	// Latin "Part N" / "Season N" should also fold so the english fold matches
	// the bare title.
	if l := foldSeason("Vinland Saga Part 2"); strings.Contains(l, "part") || strings.Contains(l, "2") {
		t.Fatalf("latin part marker not stripped: %q", l)
	}
}

// Two season-variants of the same series should fold to (near) the same string,
// so the bare-title fuzzy score is high and season disambiguation is left to the
// section/number logic in scoreAndPick.
func TestFoldSeasonMakesSeasonVariantsConverge(t *testing.T) {
	a := foldSeason("Код Гиас: Восставший Лелуш (1 сезон) [25 из 25]")
	b := foldSeason("Код Гиас: Восставший Лелуш (2 сезон) [25 из 25]")
	if a != b {
		t.Fatalf("season variants did not converge: %q vs %q", a, b)
	}
	if s := jaroWinkler(a, b); s < 0.85 {
		t.Fatalf("converged folds scored below threshold: %v", s)
	}
}
