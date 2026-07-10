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

func TestValidate_TooManyTags(t *testing.T) {
	r := validReq()
	r.Tags = []string{}
	for i := 0; i < 9; i++ {
		r.Tags = append(r.Tags, "tag")
	}
	if err := r.Validate(); err == nil {
		t.Error("expected too-many-tags error")
	}
}

func TestValidate_TagTooLong(t *testing.T) {
	r := validReq()
	r.Tags = []string{strings.Repeat("a", 33)}
	if err := r.Validate(); err == nil {
		t.Error("expected tag-too-long error for 33-char ASCII tag")
	}
}

func TestValidate_CyrillicRuneCounting(t *testing.T) {
	// A byte-based len() would count 64 bytes (2 bytes/rune) and wrongly reject
	// this as over the 32-char cap. Rune counting must accept it.
	r := validReq()
	r.Tags = []string{strings.Repeat("я", 32)}
	if err := r.Validate(); err != nil {
		t.Errorf("expected 32-Cyrillic-char tag to be accepted, got %v", err)
	}

	r = validReq()
	r.Tags = []string{strings.Repeat("я", 33)}
	if err := r.Validate(); err == nil {
		t.Error("expected 33-Cyrillic-char tag to be rejected")
	}

	r = validReq()
	r.Prompt = strings.Repeat("я", 2000)
	if err := r.Validate(); err != nil {
		t.Errorf("expected 2000-Cyrillic-char prompt to be accepted, got %v", err)
	}

	r = validReq()
	r.Prompt = strings.Repeat("я", 2001)
	if err := r.Validate(); err == nil {
		t.Error("expected 2001-Cyrillic-char prompt to be rejected")
	}
}

func TestValidate_CanonRequiresAnimeIdentity(t *testing.T) {
	base := GenerateRequest{
		Anime:    AnimeRef{Title: "Frieren"}, // title set, but no id / shikimori_id
		Length:   "oneshot",
		POV:      "third",
		Rating:   "teen",
		Language: "ru",
		Canon:    true,
	}
	if err := base.Validate(); err == nil {
		t.Fatal("expected error: canon without anime id/shikimori_id must be rejected")
	}

	withID := base
	withID.Anime.ID = "11111111-1111-1111-1111-111111111111"
	if err := withID.Validate(); err != nil {
		t.Fatalf("canon with anime.id should pass, got %v", err)
	}

	withShiki := base
	withShiki.Anime.ShikimoriID = "52991"
	if err := withShiki.Validate(); err != nil {
		t.Fatalf("canon with shikimori_id should pass, got %v", err)
	}

	// Non-canon with no identity stays valid (unchanged behavior).
	nonCanon := base
	nonCanon.Canon = false
	if err := nonCanon.Validate(); err != nil {
		t.Fatalf("non-canon without identity should pass, got %v", err)
	}
}
