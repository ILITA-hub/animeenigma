package service

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

func TestDailySeed_UTCDayStable(t *testing.T) {
	a := DailySeed(time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC))
	b := DailySeed(time.Date(2026, 7, 14, 23, 59, 0, 0, time.UTC))
	if a != b {
		t.Fatalf("same UTC day must produce same seed: %d vs %d", a, b)
	}
	c := DailySeed(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC))
	if c == a {
		t.Fatal("different UTC day must produce a different seed")
	}
	// Non-UTC input must be normalized to UTC before computing the seed.
	jst := time.FixedZone("JST", 9*60*60)
	d := DailySeed(time.Date(2026, 7, 15, 8, 0, 0, 0, jst)) // == 2026-07-14 23:00 UTC
	if d != a {
		t.Fatalf("must normalize to UTC before computing seed: %d vs %d", d, a)
	}
}

func TestPickDaily_PrefersUserOverBot_Deterministic(t *testing.T) {
	bot := domain.Fanfic{ID: "bot", AIGenerated: true, Status: domain.StatusComplete}
	u1 := domain.Fanfic{ID: "u1", AIGenerated: false, Status: domain.StatusComplete}
	u2 := domain.Fanfic{ID: "u2", AIGenerated: false, Status: domain.StatusComplete}
	got := PickDaily([]domain.Fanfic{bot, u1, u2}, 3) // 3 % 2 == 1 -> u2
	if got == nil || got.ID != "u2" {
		t.Fatalf("want u2, got %v", got)
	}
	// same seed -> same pick
	if PickDaily([]domain.Fanfic{bot, u1, u2}, 3).ID != "u2" {
		t.Fatal("nondeterministic")
	}
}

func TestPickDaily_FallsBackToBot(t *testing.T) {
	bot := domain.Fanfic{ID: "bot", AIGenerated: true, Status: domain.StatusComplete}
	if PickDaily([]domain.Fanfic{bot}, 5).ID != "bot" {
		t.Fatal("want bot fallback")
	}
	if PickDaily(nil, 5) != nil {
		t.Fatal("want nil on empty")
	}
}

func TestToDTO_ExplicitHidesExcerpt(t *testing.T) {
	f := &domain.Fanfic{ID: "x", Title: "T", AnimeTitle: "A", Content: "Long body here.", Rating: "explicit", SpotlightCredit: true, AuthorUsername: "neo"}
	d := ToDTO(f)
	if d.Excerpt != "" || !d.Explicit {
		t.Fatalf("explicit must hide excerpt: %+v", d)
	}
	if d.AuthorUsername != "neo" || !d.Credited {
		t.Fatalf("credited author expected: %+v", d)
	}
}

func TestToDTO_AnonWhenNotCredited(t *testing.T) {
	f := &domain.Fanfic{ID: "x", Content: "Body.", Rating: "teen", SpotlightCredit: false, AuthorUsername: "neo"}
	d := ToDTO(f)
	if d.AuthorUsername != "" || d.Credited {
		t.Fatalf("must anonymize: %+v", d)
	}
	if d.Excerpt == "" {
		t.Fatal("teen must have excerpt")
	}
}

func TestBuildExcerpt_StripsHeadingAndClamps(t *testing.T) {
	in := "# Title\n\n## Часть 1\n\nОна открыла дверь и замерла на пороге, не в силах произнести ни слова."
	got := BuildExcerpt(in, 30)
	if got == "" || len([]rune(got)) > 31 || got[0] == '#' {
		t.Fatalf("bad excerpt: %q", got)
	}
}
