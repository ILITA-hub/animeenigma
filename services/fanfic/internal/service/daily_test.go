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

func TestPickDaily_BotFallbackIsOldestStable(t *testing.T) {
	// Bots rotate oldest-first (yesterday's generation shows today), so the
	// cron inserting today's bot mid-day can never flip the day's pick.
	// `eligible` arrives oldest-first from the repo.
	old := domain.Fanfic{ID: "bot-old", AIGenerated: true, Status: domain.StatusComplete}
	fresh := domain.Fanfic{ID: "bot-new", AIGenerated: true, Status: domain.StatusComplete}
	for seed := range 5 {
		if got := PickDaily([]domain.Fanfic{old, fresh}, seed); got == nil || got.ID != "bot-old" {
			t.Fatalf("seed %d: want oldest bot regardless of seed, got %v", seed, got)
		}
	}
}

func TestEligibleWindowStart_DayAligned(t *testing.T) {
	// Window start = UTC midnight of the PREVIOUS day and is stable for the
	// whole day — an in-window fanfic can only age out exactly at the midnight
	// rollover, the same instant the seed and spotlight cache key change.
	want := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	early := EligibleWindowStart(time.Date(2026, 7, 17, 0, 1, 0, 0, time.UTC))
	late := EligibleWindowStart(time.Date(2026, 7, 17, 23, 59, 0, 0, time.UTC))
	if !early.Equal(want) || !late.Equal(want) {
		t.Fatalf("want %v all day, got %v / %v", want, early, late)
	}
	// Non-UTC input must be normalized to UTC first: JST 2026-07-17 08:00 is
	// still UTC day 2026-07-16, so the window starts at 2026-07-15 00:00 UTC.
	jst := time.FixedZone("JST", 9*60*60)
	wantJST := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	if got := EligibleWindowStart(time.Date(2026, 7, 17, 8, 0, 0, 0, jst)); !got.Equal(wantJST) {
		t.Fatalf("JST input: want %v, got %v", wantJST, got)
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

func TestBuildExcerpt_SkipsDecorativeDividers(t *testing.T) {
	// Regression (2026-07-17): a prod bot fanfic opened with a "-_-_-_…" divider
	// paragraph, which shipped verbatim as the spotlight card's excerpt. Any
	// paragraph without a single letter or digit is decoration, not prose.
	in := "-_-_-_-_-_-_-_-_-_-_\n\nЯ сидел на берегу реки, наблюдая за водой."
	if got := BuildExcerpt(in, 240); got != "Я сидел на берегу реки, наблюдая за водой." {
		t.Fatalf("divider must be skipped, got %q", got)
	}
	if got := BuildExcerpt("***\n\n---\n\n_ _ _", 240); got != "" {
		t.Fatalf("content with no prose must produce an empty excerpt, got %q", got)
	}
}
