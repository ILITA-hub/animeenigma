package spotlight

import (
	"encoding/json"
	"math/rand"
	"time"
)

const telegramNewsMaxAge = 7 * 24 * time.Hour

var spotlightCardGroups = [][]string{
	{"personal_pick", "upcoming_for_you"},
	{"featured", "random_tail", "curated"},
}

// prepareCards applies the final carousel eligibility rules after resolver
// fan-out. Keeping this at the response boundary also protects snapshot
// fallbacks produced by older builds.
func prepareCards(cards []Card, now time.Time, randomIndex func(int) int) []Card {
	filtered := make([]Card, 0, len(cards))
	for _, card := range cards {
		if card.Type == "telegram_news" && !telegramNewsIsRecent(card.Data, now) {
			continue
		}
		filtered = append(filtered, card)
	}

	for _, group := range spotlightCardGroups {
		matching := make([]int, 0, len(group))
		for i, card := range filtered {
			if containsCardType(group, card.Type) {
				matching = append(matching, i)
			}
		}
		if len(matching) <= 1 {
			continue
		}

		keep := matching[randomIndex(len(matching))]
		next := make([]Card, 0, len(filtered)-len(matching)+1)
		for i, card := range filtered {
			if !containsCardType(group, card.Type) || i == keep {
				next = append(next, card)
			}
		}
		filtered = next
	}

	return filtered
}

func prepareCardsRandomly(cards []Card, now time.Time) []Card {
	return prepareCards(cards, now, rand.Intn)
}

func containsCardType(types []string, cardType string) bool {
	for _, candidate := range types {
		if candidate == cardType {
			return true
		}
	}
	return false
}

func telegramNewsIsRecent(data any, now time.Time) bool {
	var news TelegramNewsData
	switch typed := data.(type) {
	case TelegramNewsData:
		news = typed
	case *TelegramNewsData:
		if typed == nil {
			return false
		}
		news = *typed
	default:
		raw, err := json.Marshal(data)
		if err != nil || json.Unmarshal(raw, &news) != nil {
			return false
		}
	}

	cutoff := now.Add(-telegramNewsMaxAge)
	for _, post := range news.Posts {
		published, err := time.Parse(time.RFC3339, post.Date)
		if err == nil && !published.Before(cutoff) {
			return true
		}
	}
	return false
}
