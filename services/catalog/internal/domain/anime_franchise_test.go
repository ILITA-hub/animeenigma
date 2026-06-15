package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnime_FranchiseSerialization(t *testing.T) {
	a := Anime{ID: "uuid-1", Franchise: "frieren"}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"franchise":"frieren"`) {
		t.Fatalf("expected franchise in JSON, got %s", string(b))
	}
}

func TestAnime_FranchiseOmitEmpty(t *testing.T) {
	a := Anime{ID: "uuid-1"}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "franchise") {
		t.Fatalf("expected franchise omitted when empty, got %s", string(b))
	}
}
