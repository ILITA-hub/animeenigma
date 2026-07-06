package service

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

func TestBuildMessages_RU_Mature(t *testing.T) {
	req := domain.GenerateRequest{
		Anime:      domain.AnimeRef{Title: "Frieren", Japanese: "葬送のフリーレン"},
		Characters: []domain.CharacterRef{{Name: "Frieren"}, {Name: "Fern"}},
		Tags:       []string{"slow-burn", "angst"},
		Length:     "oneshot", POV: "third", Rating: "mature", Language: "ru",
		Prompt: "тихий вечер у костра",
	}
	sys, usr := BuildMessages(req)
	if !strings.Contains(sys, "РУССКИЙ") {
		t.Error("system prompt should pin Russian output")
	}
	if !strings.Contains(sys, "# ") {
		t.Error("system prompt should instruct a leading '# Title' line")
	}
	if !strings.Contains(sys, "18+") {
		t.Error("system prompt should frame characters as adults")
	}
	if !strings.Contains(usr, "Frieren") || !strings.Contains(usr, "Fern") {
		t.Error("user prompt should list characters")
	}
	if !strings.Contains(usr, "slow-burn") {
		t.Error("user prompt should list tags")
	}
	if !strings.Contains(usr, "тихий вечер") {
		t.Error("user prompt should include the author brief")
	}
}

func TestBuildMessages_EN_Teen_NoExplicit(t *testing.T) {
	req := domain.GenerateRequest{
		Anime: domain.AnimeRef{Title: "Bocchi"}, Length: "drabble",
		POV: "first", Rating: "teen", Language: "en",
	}
	sys, _ := BuildMessages(req)
	if !strings.Contains(sys, "ENGLISH") {
		t.Error("system prompt should pin English output")
	}
	if !strings.Contains(strings.ToLower(sys), "no explicit") {
		t.Error("teen tier should forbid explicit content")
	}
}

func TestMaxTokensFor(t *testing.T) {
	if MaxTokensFor("drabble") >= MaxTokensFor("oneshot") || MaxTokensFor("oneshot") >= MaxTokensFor("short") {
		t.Error("token budget must increase with length")
	}
	if MaxTokensFor("unknown") == 0 {
		t.Error("unknown length should fall back to a sane default")
	}
}

func TestSplitTitle(t *testing.T) {
	title, body := SplitTitle("# Тяжесть столетий\n\nКогда солнце...")
	if title != "Тяжесть столетий" {
		t.Errorf("title = %q", title)
	}
	if strings.HasPrefix(body, "#") {
		t.Error("body should have the H1 stripped")
	}
	title2, body2 := SplitTitle("no heading here")
	if title2 != "" || body2 != "no heading here" {
		t.Errorf("no-heading case: title=%q body=%q", title2, body2)
	}
}
