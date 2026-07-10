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
	sys, usr := BuildMessages(req, "")
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
	sys, _ := BuildMessages(req, "")
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

func TestBuildMessages_CanonInjectsSynopsis(t *testing.T) {
	req := domain.GenerateRequest{
		Anime:    domain.AnimeRef{Title: "Frieren", Japanese: "葬送のフリーレン"},
		Length:   "oneshot", POV: "third", Rating: "teen", Language: "ru", Canon: true,
		Prompt:   "куда дальше",
	}
	sys, usr := BuildMessages(req, "Фрирен путешествует после смерти Химмеля.")
	if !strings.Contains(usr, "Фрирен путешествует") {
		t.Errorf("synopsis not injected into user prompt: %q", usr)
	}
	if !strings.Contains(sys, "канон") && !strings.Contains(sys, "РЕАЛЬНЫЙ") {
		t.Errorf("canon instruction missing from system prompt: %q", sys)
	}
}

func TestBuildMessages_NonCanonUnchanged(t *testing.T) {
	req := domain.GenerateRequest{
		Anime: domain.AnimeRef{Title: "Frieren"}, Length: "drabble",
		POV: "first", Rating: "teen", Language: "en",
	}
	sys, _ := BuildMessages(req, "")
	if !strings.Contains(sys, "# Title") {
		t.Errorf("non-canon system prompt should keep the title instruction: %q", sys)
	}
}

func TestBuildContinueMessages(t *testing.T) {
	f := domain.Fanfic{
		AnimeTitle: "Frieren", Length: "oneshot", POV: "third",
		Rating: "teen", Language: "ru",
	}
	sys, usr := BuildContinueMessages(f, "конец первой части")
	if strings.Contains(sys, "# Заголовок") || strings.Contains(sys, "# Title") {
		t.Errorf("continue system prompt must NOT instruct a title: %q", sys)
	}
	if !strings.Contains(usr, "конец первой части") {
		t.Errorf("prior context missing from continue user prompt: %q", usr)
	}
}

func TestTailRunes(t *testing.T) {
	if got := TailRunes("abcdef", 3); got != "def" {
		t.Errorf("TailRunes = %q, want def", got)
	}
	if got := TailRunes("ab", 5); got != "ab" {
		t.Errorf("TailRunes short = %q, want ab", got)
	}
	// Multibyte-safe: 5 Cyrillic runes, keep last 2.
	if got := TailRunes("абвгд", 2); got != "гд" {
		t.Errorf("TailRunes cyrillic = %q, want гд", got)
	}
}
