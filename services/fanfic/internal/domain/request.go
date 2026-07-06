package domain

import (
	"fmt"
	"strings"
)

// AnimeRef is the anime snapshot the client sends (already fetched for the picker).
type AnimeRef struct {
	ID          string `json:"id"`
	ShikimoriID string `json:"shikimori_id"`
	Title       string `json:"title"`
	Japanese    string `json:"japanese"`
	Poster      string `json:"poster"`
}

// CharacterRef is one selected character (id optional).
type CharacterRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GenerateRequest is the POST /api/fanfic/generate body.
type GenerateRequest struct {
	Anime      AnimeRef       `json:"anime"`
	Characters []CharacterRef `json:"characters"`
	Tags       []string       `json:"tags"`
	Length     string         `json:"length"`
	POV        string         `json:"pov"`
	Rating     string         `json:"rating"`
	Language   string         `json:"language"`
	Prompt     string         `json:"prompt"`
}

var (
	validLength   = map[string]bool{"drabble": true, "oneshot": true, "short": true}
	validPOV      = map[string]bool{"first": true, "third": true}
	validRating   = map[string]bool{"teen": true, "mature": true, "explicit": true}
	validLanguage = map[string]bool{"ru": true, "en": true}
)

// Validate implements httputil.Validator.
func (r GenerateRequest) Validate() error {
	if strings.TrimSpace(r.Anime.Title) == "" {
		return fmt.Errorf("anime title is required")
	}
	if !validLength[r.Length] {
		return fmt.Errorf("invalid length %q", r.Length)
	}
	if !validPOV[r.POV] {
		return fmt.Errorf("invalid pov %q", r.POV)
	}
	if !validRating[r.Rating] {
		return fmt.Errorf("invalid rating %q", r.Rating)
	}
	if !validLanguage[r.Language] {
		return fmt.Errorf("invalid language %q", r.Language)
	}
	if len(r.Characters) > 6 {
		return fmt.Errorf("too many characters (max 6)")
	}
	if len(r.Tags) > 8 {
		return fmt.Errorf("too many tags (max 8)")
	}
	for _, t := range r.Tags {
		if len(t) > 32 {
			return fmt.Errorf("tag too long (max 32): %q", t)
		}
	}
	if len(r.Prompt) > 2000 {
		return fmt.Errorf("prompt too long (max 2000)")
	}
	return nil
}
