package domain

import (
	"strings"
	"testing"
)

func validReq() GenerateRequest {
	return GenerateRequest{
		Anime:      AnimeRef{Title: "Frieren"},
		Characters: []CharacterRef{{Name: "Frieren"}, {Name: "Fern"}},
		Tags:       []string{"slow-burn"},
		Length:     "oneshot",
		POV:        "third",
		Rating:     "mature",
		Language:   "ru",
		Prompt:     "тихий вечер у костра",
	}
}

func TestValidate_OK(t *testing.T) {
	if err := validReq().Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_BadEnums(t *testing.T) {
	cases := map[string]func(*GenerateRequest){
		"length":   func(r *GenerateRequest) { r.Length = "epic" },
		"pov":      func(r *GenerateRequest) { r.POV = "second" },
		"rating":   func(r *GenerateRequest) { r.Rating = "nsfw" },
		"language": func(r *GenerateRequest) { r.Language = "de" },
		"title":    func(r *GenerateRequest) { r.Anime.Title = "" },
	}
	for name, mut := range cases {
		r := validReq()
		mut(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestValidate_Caps(t *testing.T) {
	r := validReq()
	for i := 0; i < 7; i++ {
		r.Characters = append(r.Characters, CharacterRef{Name: "X"})
	}
	if err := r.Validate(); err == nil {
		t.Error("expected too-many-characters error")
	}
	r = validReq()
	r.Prompt = strings.Repeat("a", 2001)
	if err := r.Validate(); err == nil {
		t.Error("expected prompt-too-long error")
	}
}
