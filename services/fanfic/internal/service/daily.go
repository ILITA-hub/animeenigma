package service

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// DailySeed is a UTC day-stable integer (same formula as catalog spotlight's
// DateSeedUTC) so the pick rolls over at UTC midnight.
func DailySeed(t time.Time) int {
	u := t.UTC()
	return u.Year()*100*32 + int(u.Month())*32 + u.Day()
}

// EligibleWindowStart returns the pick-eligibility window start: UTC midnight
// of the PREVIOUS day. Day-aligned (vs a rolling now-24h) so an in-window
// fanfic never ages out mid-day — the pool only shrinks at the UTC midnight
// rollover, the same instant DailySeed and the spotlight cache key change.
// (Regression 2026-07-17: the rolling window let the day's only pick expire
// at 04:41, 404-ing the reader while the cached card still advertised it.)
func EligibleWindowStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC).Add(-24 * time.Hour)
}

// PickDaily deterministically selects one fanfic for the day. User-authored
// fanfics are preferred (date-seeded pick); AI-generated ones are the
// fallback pool, from which the OLDEST in-window bot wins — that is
// yesterday's generation, and it stays the winner for the whole UTC day even
// after the cron inserts today's bot. Returns nil when nothing is eligible.
// `eligible` is assumed pre-sorted (created_at,id).
func PickDaily(eligible []domain.Fanfic, seed int) *domain.Fanfic {
	var users, bots []domain.Fanfic
	for _, f := range eligible {
		if f.AIGenerated {
			bots = append(bots, f)
		} else {
			users = append(users, f)
		}
	}
	if len(users) == 0 {
		if len(bots) == 0 {
			return nil
		}
		return &bots[0]
	}
	idx := seed % len(users)
	if idx < 0 {
		idx += len(users)
	}
	return &users[idx]
}

// DailyDTO is the wire shape shared by GET /internal/fanfic/daily (spotlight,
// excerpt only) and the metadata half of GET /api/fanfic/daily (public reader).
type DailyDTO struct {
	ID             string    `json:"id"`
	FanficTitle    string    `json:"fanfic_title"`
	AnimeTitle     string    `json:"anime_title"`
	AnimeJapanese  string    `json:"anime_japanese"`
	AnimePoster    string    `json:"anime_poster"`
	Excerpt        string    `json:"excerpt"`
	Rating         string    `json:"rating"`
	Language       string    `json:"language"`
	Explicit       bool      `json:"explicit"`
	AuthorUsername string    `json:"author_username"`
	Credited       bool      `json:"credited"`
	AIGenerated    bool      `json:"ai_generated"`
	PartCount      int       `json:"part_count"`
	CreatedAt      time.Time `json:"created_at"`
}

const excerptRunes = 240

// ToDTO shapes a fanfic into the compact DTO. Explicit content carries NO
// excerpt (so nothing explicit enters the globally-cached spotlight payload);
// the author name is included only when the author opted in (SpotlightCredit).
func ToDTO(f *domain.Fanfic) DailyDTO {
	explicit := f.Rating == "explicit"
	d := DailyDTO{
		ID:            f.ID,
		FanficTitle:   f.Title,
		AnimeTitle:    f.AnimeTitle,
		AnimeJapanese: f.AnimeJapanese,
		AnimePoster:   f.AnimePoster,
		Rating:        f.Rating,
		Language:      f.Language,
		Explicit:      explicit,
		AIGenerated:   f.AIGenerated,
		PartCount:     f.PartCount,
		CreatedAt:     f.CreatedAt,
	}
	if !explicit {
		d.Excerpt = BuildExcerpt(f.Content, excerptRunes)
	}
	if f.SpotlightCredit && f.AuthorUsername != "" {
		d.AuthorUsername = f.AuthorUsername
		d.Credited = true
	}
	return d
}

// BuildExcerpt returns a clean plain-text teaser: drops markdown headings and
// decoration-only paragraphs (horizontal rules, "-_-_-…" dividers — anything
// without a letter or digit), takes the first prose paragraph, and clamps to
// maxRunes on a word boundary.
func BuildExcerpt(content string, maxRunes int) string {
	for _, para := range strings.Split(content, "\n\n") {
		p := strings.TrimSpace(para)
		if strings.HasPrefix(p, "#") || !hasProse(p) {
			continue
		}
		p = strings.ReplaceAll(p, "\n", " ")
		if utf8.RuneCountInString(p) <= maxRunes {
			return p
		}
		runes := []rune(p)[:maxRunes]
		cut := string(runes)
		if i := strings.LastIndex(cut, " "); i > 0 {
			cut = cut[:i]
		}
		return strings.TrimSpace(cut) + "…"
	}
	return ""
}

// hasProse reports whether s contains at least one letter or digit.
func hasProse(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}
