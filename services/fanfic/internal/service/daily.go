package service

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
)

// DailySeed is a UTC day-stable integer (same formula as catalog spotlight's
// DateSeedUTC) so the pick rolls over at UTC midnight.
func DailySeed(t time.Time) int {
	u := t.UTC()
	return u.Year()*100*32 + int(u.Month())*32 + u.Day()
}

// PickDaily deterministically selects one fanfic for the day. User-authored
// fanfics are preferred; AI-generated ones are the fallback pool. Returns nil
// when nothing is eligible. `eligible` is assumed pre-sorted (created_at,id).
func PickDaily(eligible []domain.Fanfic, seed int) *domain.Fanfic {
	var users, bots []domain.Fanfic
	for _, f := range eligible {
		if f.AIGenerated {
			bots = append(bots, f)
		} else {
			users = append(users, f)
		}
	}
	pool := users
	if len(pool) == 0 {
		pool = bots
	}
	if len(pool) == 0 {
		return nil
	}
	idx := seed % len(pool)
	if idx < 0 {
		idx += len(pool)
	}
	return &pool[idx]
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

// BuildExcerpt returns a clean plain-text teaser: drops leading markdown
// headings / horizontal rules, takes the first non-empty paragraph, and clamps
// to maxRunes on a word boundary.
func BuildExcerpt(content string, maxRunes int) string {
	for _, para := range strings.Split(content, "\n\n") {
		p := strings.TrimSpace(para)
		if p == "" || strings.HasPrefix(p, "#") || p == "---" {
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
