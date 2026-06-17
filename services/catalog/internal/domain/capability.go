package domain

// CapabilityReport is the assembled, ranked per-provider capability view for an
// anime (spec 2026-06-15-scraper-capability-api). The future player renders it:
// best provider first per family, the rest available behind a "hacker mode".
type CapabilityReport struct {
	AnimeID  string         `json:"anime_id"`
	Families []SourceFamily `json:"families"`
}

// SourceFamily groups providers of one source kind; Providers is ranked best-first.
type SourceFamily struct {
	Family    string        `json:"family"` // "ourenglish" | "kodik" | "animelib" | "hanime"
	Providers []ProviderCap `json:"providers"`
}

// ProviderCap is one provider's capability + liveness + rank within a family.
type ProviderCap struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
	// Degraded marks a soft-degraded provider: the player ranks it LAST, never
	// auto-selects/auto-falls-back to it, and only offers it (behind a "degraded"
	// pill) when hacker mode is on. EN family only; RU/Hanime families never set it.
	Degraded bool      `json:"degraded"`
	Health   string    `json:"health"`             // "up" | "down" | "unknown"
	Playable *bool     `json:"playable,omitempty"` // real-bytes oracle, if known
	Rank     float64   `json:"rank"`
	Variants []Variant `json:"variants"`
}

// Variant is a watchable unit: a category (+ optional translation team for RU),
// its subtitle delivery, and quality info. Source records provenance.
type Variant struct {
	Category      string   `json:"category"`       // "sub" | "dub" | "raw"
	Team          *Team    `json:"team,omitempty"` // RU only; nil for EN (reserved — backlog)
	SubDelivery   string   `json:"sub_delivery"`   // "soft" | "hard" | "none"
	Qualities     []string `json:"qualities,omitempty"`
	QualitySource string   `json:"quality_source"` // "hls_master" | "discrete" | "unknown" | "trait"
	Source        string   `json:"source"`         // "trait" | "discovered"
}

// Team is a translation/dub group (real for Kodik/AniLib; nil for EN providers).
type Team struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}
