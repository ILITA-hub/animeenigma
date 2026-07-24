// Package domain holds the content-verify verdict model. One row per
// (anime × provider); the provider's internal structure (teams / servers /
// tracks) lives in the Units JSONB column, one UnitVerdict per probe unit.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	StatusVerified     = "verified"
	StatusInconclusive = "inconclusive"
	StatusUnreachable  = "unreachable"
	// StatusDeferred is an in-flight sentinel, never persisted: the resolve
	// got a 503 (provider down / negative-cached upstream), so the worker
	// defers the (anime, provider) pair instead of recording a failure —
	// a down provider is not a failing episode. See RetryAfter.
	StatusDeferred = "deferred"

	// VerifiedThreshold is the owner-specified confidence gate (spec §3).
	VerifiedThreshold = 0.95
)

// UnitKey identifies one probeable unit inside a provider. Exactly the
// fields that apply are set: Kodik → Team (+Category claim), scraper →
// Server+Category, animejoy legs → Server=provider, ae → Track.
type UnitKey struct {
	Team     string `json:"team,omitempty"`
	Server   string `json:"server,omitempty"`
	Category string `json:"category,omitempty"`
	Track    string `json:"track,omitempty"`
}

// String is the canonical map key: sorted k=v joined by "|".
func (k UnitKey) String() string {
	parts := make([]string, 0, 4)
	if k.Category != "" {
		parts = append(parts, "category="+k.Category)
	}
	if k.Server != "" {
		parts = append(parts, "server="+k.Server)
	}
	if k.Team != "" {
		parts = append(parts, "team="+k.Team)
	}
	if k.Track != "" {
		parts = append(parts, "track="+k.Track)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

type AudioVerdict struct {
	Lang       string  `json:"lang,omitempty"` // en|ru|ja|other
	Confidence float64 `json:"confidence"`
	Verified   bool    `json:"verified"`
}

type HardsubVerdict struct {
	Present    bool    `json:"present"`
	Lang       string  `json:"lang,omitempty"`
	Confidence float64 `json:"confidence"`
	Verified   bool    `json:"verified"`
}

type SoftTrack struct {
	Lang string `json:"lang,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type SampleInfo struct {
	Fragments     int     `json:"fragments"`
	SpeechSeconds float64 `json:"speech_seconds"`
}

type UnitVerdict struct {
	Key      UnitKey         `json:"key"`
	Episode  int             `json:"episode"`
	Status   string          `json:"status"`
	Audio    *AudioVerdict   `json:"audio,omitempty"`
	Hardsub  *HardsubVerdict `json:"hardsub,omitempty"`
	Softsubs []SoftTrack     `json:"softsubs,omitempty"`
	// RawAudio: the unit carries the ORIGINAL-language audio track, whatever
	// that language is — a synthesized claim from provider-native metadata
	// (kodik `subtitles` translations = original audio + burned RU subs).
	// Unlike Audio.Lang=="ja" it stays correct for non-Japanese originals
	// (donghua etc.), which is exactly why kodik synth uses it instead of
	// asserting a language it never heard.
	RawAudio bool `json:"raw_audio,omitempty"`
	// Episodes: how many episodes this unit has ready on the provider right
	// now — a free by-product of queue enumeration (kodik per-team
	// episodes_count, scraper/animejoy episode-list length). 0 = unknown.
	Episodes int        `json:"episodes,omitempty"`
	ProbedAt time.Time  `json:"probed_at"`
	Sample   SampleInfo `json:"sample"`
	Fails    int        `json:"fails,omitempty"` // consecutive unreachable count → backoff
	// RetryAfter accompanies StatusDeferred only (in-flight sentinel — the
	// worker turns it into an engine deferral and drops the verdict, so it
	// is never serialized into the store).
	RetryAfter time.Duration `json:"-"`
}

// UnitList serializes as JSON for the jsonb column (works on postgres and
// the pure-Go sqlite driver used in tests).
type UnitList []UnitVerdict

func (u UnitList) Value() (driver.Value, error) { return json.Marshal(u) }

func (u *UnitList) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*u = nil
		return nil
	case []byte:
		return json.Unmarshal(v, u)
	case string:
		return json.Unmarshal([]byte(v), u)
	}
	return fmt.Errorf("unitlist: unsupported scan type %T", src)
}

type ContentVerification struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())" json:"id"`
	AnimeID   string    `gorm:"type:uuid;uniqueIndex:idx_cv_anime_provider" json:"anime_id"`
	Provider  string    `gorm:"size:64;uniqueIndex:idx_cv_anime_provider" json:"provider"`
	Units     UnitList  `gorm:"type:jsonb" json:"units"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate fills the UUID app-side so sqlite tests (no gen_random_uuid)
// behave like postgres.
func (c *ContentVerification) BeforeCreate(*gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	return nil
}

// ProviderSummary is the wire rollup consumed by catalog + aePlayer.
type ProviderSummary struct {
	Status       string   `json:"status"` // unverified|partial|verified
	Raw          bool     `json:"raw"`
	DubLangs     []string `json:"dub_langs"`
	HardsubLangs []string `json:"hardsub_langs"`
	// Episodes: provider-level "episodes ready now" — max across units (units
	// of one provider share the episode list; kodik teams differ, max = the
	// fullest team). 0 = no unit reported a count yet.
	Episodes int `json:"episodes,omitempty"`
	// Unreachable: EVERY probed unit came back StatusUnreachable (stream resolved
	// but its media is dead — see prober.unreachable). A provider with any
	// verified/inconclusive unit is at least partially reachable and is NOT
	// flagged. Drives the aePlayer "may not work" badge (informational only —
	// the source stays selectable). Never set for the ae/kodik synth (their
	// units are always verified). omitempty: absent == false on the wire.
	Unreachable bool `json:"unreachable,omitempty"`
}

func Summarize(units []UnitVerdict) ProviderSummary {
	s := ProviderSummary{Status: "unverified", DubLangs: []string{}, HardsubLangs: []string{}}
	if len(units) == 0 {
		return s
	}
	dub := map[string]bool{}
	hs := map[string]bool{}
	verified := 0
	unreachable := 0
	for _, u := range units {
		if u.Status == StatusVerified {
			verified++
		}
		if u.Status == StatusUnreachable {
			unreachable++
		}
		s.Episodes = max(s.Episodes, u.Episodes)
		if u.Status == StatusVerified && u.RawAudio {
			s.Raw = true // provider-native original-audio claim (kodik subtitles teams)
		}
		if u.Audio != nil && u.Audio.Verified {
			if u.Audio.Lang == "ja" {
				s.Raw = true
			} else if u.Audio.Lang == "en" || u.Audio.Lang == "ru" {
				dub[u.Audio.Lang] = true
			}
		}
		// Hardsub rollup counts RAW-track units ONLY (owner taxonomy 2026-07-17:
		// the badge is "SUB BURNED-IN <lang>" on the original-audio option).
		// Dub units routinely inherit burned subs from the raw source they were
		// voiced over — real, but noise as a provider-level claim; the detail
		// stays visible per-unit for hacker mode.
		if u.Key.Category != "dub" && u.Hardsub != nil && u.Hardsub.Verified && u.Hardsub.Present && u.Hardsub.Lang != "" {
			hs[u.Hardsub.Lang] = true
		}
	}
	for l := range dub {
		s.DubLangs = append(s.DubLangs, l)
	}
	for l := range hs {
		s.HardsubLangs = append(s.HardsubLangs, l)
	}
	sort.Strings(s.DubLangs)
	sort.Strings(s.HardsubLangs)
	switch {
	case verified == len(units):
		s.Status = "verified"
	case verified > 0 || s.Raw || len(s.DubLangs) > 0 || len(s.HardsubLangs) > 0:
		s.Status = "partial"
	}
	// Flagged only when every unit is unreachable (len>0 guaranteed above).
	s.Unreachable = unreachable == len(units)
	return s
}
