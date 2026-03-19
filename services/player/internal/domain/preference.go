package domain

import "time"

// WatchCombo describes a normalized player+translation selection
type WatchCombo struct {
	Player           string `json:"player"`           // kodik, animelib, hianime, consumet
	Language         string `json:"language"`          // ru, en
	WatchType        string `json:"watch_type"`        // dub, sub
	TranslationID    string `json:"translation_id"`    // provider-specific, always string
	TranslationTitle string `json:"translation_title"` // human-readable team name
}

// ValidPlayers is the set of allowed player values
var ValidPlayers = map[string]bool{
	"kodik": true, "animelib": true, "hianime": true, "consumet": true,
}

// ValidLanguages is the set of allowed language values
var ValidLanguages = map[string]bool{"ru": true, "en": true}

// ValidWatchTypes is the set of allowed watch type values
var ValidWatchTypes = map[string]bool{"dub": true, "sub": true}

// ValidateCombo checks if combo fields are valid (when present)
func ValidateCombo(player, language, watchType string) bool {
	if player == "" && language == "" && watchType == "" {
		return true // all empty = no combo, valid
	}
	return ValidPlayers[player] && ValidLanguages[language] && ValidWatchTypes[watchType]
}

// UserAnimePreference stores the user's last-used combo per anime
type UserAnimePreference struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_anime_pref" json:"user_id"`
	AnimeID          string    `gorm:"not null;uniqueIndex:idx_user_anime_pref" json:"anime_id"`
	Player           string    `gorm:"size:20;not null" json:"player"`
	Language         string    `gorm:"size:5;not null" json:"language"`
	WatchType        string    `gorm:"size:5;not null" json:"watch_type"`
	TranslationID    string    `gorm:"size:50" json:"translation_id"`
	TranslationTitle string    `gorm:"size:200" json:"translation_title"`
	UpdatedAt        time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

// ResolveRequest is the payload for POST /api/users/preferences/resolve
type ResolveRequest struct {
	AnimeID   string       `json:"anime_id"`
	Available []WatchCombo `json:"available"`
}

// ResolveResponse is the response for the resolve endpoint
type ResolveResponse struct {
	Resolved *ResolvedCombo `json:"resolved"`
}

// ResolvedCombo extends WatchCombo with resolution metadata
type ResolvedCombo struct {
	WatchCombo
	Tier       string `json:"tier"`        // per_anime, user_global, community, pinned, default
	TierNumber int    `json:"tier_number"` // 1-5
}

// ComboCount is a user's watch count for a specific combo (used by Tier 2)
type ComboCount struct {
	Player           string `json:"player"`
	Language         string `json:"language"`
	WatchType        string `json:"watch_type"`
	TranslationTitle string `json:"translation_title"`
	Count            int    `json:"count"`
}

// CommunityCombo is a community popularity entry for an anime (used by Tier 3)
type CommunityCombo struct {
	Player           string `json:"player"`
	Language         string `json:"language"`
	WatchType        string `json:"watch_type"`
	TranslationID    string `json:"translation_id"`
	TranslationTitle string `json:"translation_title"`
	Viewers          int    `json:"viewers"`
}

// PinnedTranslation maps to catalog's pinned_translations table (shared DB, used by Tier 4)
type PinnedTranslation struct {
	AnimeID          string `gorm:"column:anime_id"`
	TranslationID    int    `gorm:"column:translation_id"`
	TranslationTitle string `gorm:"column:translation_title"`
	TranslationType  string `gorm:"column:translation_type"` // "voice" or "subtitles"
}

func (PinnedTranslation) TableName() string { return "pinned_translations" }
