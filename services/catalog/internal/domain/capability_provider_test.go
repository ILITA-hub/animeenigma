package domain

import (
	"encoding/json"
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
	if got := string(b); !contains(got, `"state":"active"`) || !contains(got, `"hacker_only":false`) {
		t.Fatalf("unexpected json: %s", got)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (func() bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}()) }
