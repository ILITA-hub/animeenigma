package animejoy

import "testing"

// LegEmbedURL is the single place that maps a leg name onto the Episode field it
// lives in. These tests pin that mapping (PURE, no network).
func TestLegEmbedURL(t *testing.T) {
	ep := Episode{
		Num:      1,
		Sibnet:   "https://iv.sibnet.ru/shell.php?videoid=10",
		AllVideo: "https://fsst.online/embed/a1",
	}

	if got := LegEmbedURL(ep, "sibnet"); got != ep.Sibnet {
		t.Fatalf("sibnet leg = %q, want %q", got, ep.Sibnet)
	}
	if got := LegEmbedURL(ep, "allvideo"); got != ep.AllVideo {
		t.Fatalf("allvideo leg = %q, want %q", got, ep.AllVideo)
	}
	if got := LegEmbedURL(ep, "dzen"); got != "" {
		t.Fatalf("unknown leg = %q, want empty", got)
	}
}

func TestLegEmbedURL_AbsentField(t *testing.T) {
	// Episode carries only AllVideo — sibnet leg must report empty.
	ep := Episode{Num: 2, AllVideo: "https://fsst.online/embed/b2"}
	if got := LegEmbedURL(ep, "sibnet"); got != "" {
		t.Fatalf("sibnet on AllVideo-only episode = %q, want empty", got)
	}
	if got := LegEmbedURL(ep, "allvideo"); got != ep.AllVideo {
		t.Fatalf("allvideo = %q, want %q", got, ep.AllVideo)
	}
}
