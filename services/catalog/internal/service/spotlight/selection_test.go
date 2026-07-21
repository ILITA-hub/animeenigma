package spotlight

import (
	"testing"
	"time"
)

func TestPrepareCards_CollapsesCarouselGroups(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	cards := []Card{
		{Type: "latest_news"},
		{Type: "personal_pick"},
		{Type: "upcoming_for_you"},
		{Type: "featured"},
		{Type: "random_tail"},
		{Type: "curated"},
	}

	got := prepareCards(cards, now, func(n int) int { return n - 1 })
	want := []string{"latest_news", "upcoming_for_you", "curated"}
	if len(got) != len(want) {
		t.Fatalf("got %d cards, want %d: %+v", len(got), len(want), got)
	}
	for i, cardType := range want {
		if got[i].Type != cardType {
			t.Errorf("card %d type = %q, want %q", i, got[i].Type, cardType)
		}
	}
}

func TestPrepareCards_KeepsOnlyRecentTelegramNews(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	recent := Card{Type: "telegram_news", Data: TelegramNewsData{Posts: []TelegramPost{
		{Date: now.Add(-6 * 24 * time.Hour).Format(time.RFC3339)},
	}}}
	staleSnapshotShape := Card{Type: "telegram_news", Data: map[string]any{
		"posts": []any{map[string]any{"date": now.Add(-8 * 24 * time.Hour).Format(time.RFC3339)}},
	}}

	got := prepareCards([]Card{{Type: "latest_news"}, staleSnapshotShape, recent}, now, func(int) int { return 0 })
	if len(got) != 2 || got[0].Type != "latest_news" || got[1].Type != "telegram_news" {
		t.Fatalf("expected latest_news + recent telegram card, got %+v", got)
	}
}

func TestTelegramNewsIsRecent_IncludesExactSevenDayBoundary(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	data := TelegramNewsData{Posts: []TelegramPost{{Date: now.Add(-telegramNewsMaxAge).Format(time.RFC3339)}}}
	if !telegramNewsIsRecent(data, now) {
		t.Fatal("post at the exact seven-day boundary should be recent")
	}
}
