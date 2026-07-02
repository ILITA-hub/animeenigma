package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderCapFeedFieldsRoundTrip(t *testing.T) {
	in := ProviderCap{
		Provider: "gogoanime", DisplayName: "GogoAnime",
		State: "active", Selectable: true, HackerOnly: false,
		Order: 85, Group: "en", Audios: []string{"sub", "dub"}, Reason: "",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ProviderCap
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.State != "active" || !out.Selectable || out.Order != 85 ||
		out.Group != "en" || len(out.Audios) != 2 {
		t.Fatalf("round-trip lost feed fields: %+v", out)
	}
	// JSON keys are snake_case for the FE.
	if got := string(b); !strings.Contains(got, `"state":"active"`) || !strings.Contains(got, `"hacker_only":false`) {
		t.Fatalf("unexpected json: %s", got)
	}
}
