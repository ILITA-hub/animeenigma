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
	Family    string        `json:"family"` // "18+" | "others" | "aeProvider"
	Providers []ProviderCap `json:"providers"`
}

// ProviderCap is one provider's capability + rank within a family. Liveness is
// no longer a wire field — the Phase-1 feed fields below encode render/select
// state; internal ranking still factors live health via Rank.
type ProviderCap struct {
	Provider    string  `json:"provider"`
	DisplayName string  `json:"display_name"`
	Rank        float64 `json:"rank"`

	// Phase-1 single-source-of-truth feed fields. Computed server-side from the
	// DB row via deriveProviderView; the player renders these verbatim.
	State      string   `json:"state"`        // active | recovering | degraded | no_content
	Selectable bool     `json:"selectable"`
	HackerOnly bool     `json:"hacker_only"`  // true only for degraded
	Order      int      `json:"order"`        // preference_weight; FE sorts desc
	Group      string   `json:"group"`        // en | ru | adult | firstparty
	Audios     []string `json:"audios"`       // ["sub","dub"] from supports_* (binary audio model)
	Reason     string   `json:"reason,omitempty"`

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
