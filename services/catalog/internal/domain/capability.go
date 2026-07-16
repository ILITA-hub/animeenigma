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
	State      string   `json:"state"` // active | recovering | degraded | no_content
	Selectable bool     `json:"selectable"`
	HackerOnly bool     `json:"hacker_only"` // true only for degraded
	Order      int      `json:"order"`       // preference_weight; FE sorts desc
	Group      string   `json:"group"`       // en | ru | adult | firstparty
	Audios     []string `json:"audios"`      // ["sub","dub"] from supports_* (binary audio model)
	Reason     string   `json:"reason,omitempty"`

	// PlayerKey is the legacy watch_history.player namespace key for this
	// provider ('english', 'kodik', 'ae', …) from the roster row. The FE uses
	// it to persist watch combos without a hardcoded provider→player switch
	// (AUTO-608). Empty when the row has none.
	PlayerKey string `json:"player_key,omitempty"`

	// Lang overrides the group's default language set (GROUP_LANGS on the FE)
	// with the real per-title language a dub was probed in. Set ONLY for the
	// first-party `ae` provider's real dub variant ("en" | "ru") — every other
	// provider (en/ru/adult groups) leaves this empty and the FE keeps deriving
	// language from `group` as before (Phase C source-panel truth).
	Lang string `json:"lang,omitempty"`

	// PlayabilityIndex is the blended, decayed rank score (Phase B). Higher =
	// more playable. The FE sorts the `degraded` bucket by it. omitempty drops
	// it when analytics is unavailable and the blend was skipped.
	PlayabilityIndex float64 `json:"playability_index,omitempty"`

	// PartialLibrary is set ONLY for the first-party `ae` provider when its
	// self-hosted library is present but does NOT include episode 1 (a late-only
	// auto-cache, e.g. Frieren ep 27 of 28). The FE keeps such a library out of
	// the fresh-open smart default so the player opens episode 1 from a full
	// source; ae stays MANUALLY selectable. A complete ae library (covers ep 1)
	// omits this and remains the preferred default. Always false/omitted for
	// every non-ae provider (they list their full episode range).
	PartialLibrary bool `json:"partial_library,omitempty"`

	// Verify carries the content-verify probe rollup (nil = never probed).
	Verify *VerifySummary `json:"verify,omitempty"`

	Variants []Variant `json:"variants"`
}

// VerifySummary is the content-verify rollup for one provider on one anime.
type VerifySummary struct {
	Status       string   `json:"status"` // unverified|partial|verified
	Raw          bool     `json:"raw"`
	DubLangs     []string `json:"dub_langs"`
	HardsubLangs []string `json:"hardsub_langs"`
}

// Variant is a watchable unit: a category (+ optional translation team for RU),
// its subtitle delivery, and quality info. Source records provenance.
type Variant struct {
	Category      string   `json:"category"`       // "sub" | "dub" | "raw"
	Team          *Team    `json:"team,omitempty"` // RU only; nil for EN (reserved — backlog)
	SubDelivery   string   `json:"sub_delivery"`   // "soft" | "hard" | "none"
	Qualities     []string `json:"qualities,omitempty"`
	QualitySource string   `json:"quality_source"` // "hls_master" | "discrete" | "unknown" | "trait" | "probed"
	Source        string   `json:"source"`         // "trait" | "discovered"
}

// Team is a translation/dub group (real for Kodik/AniLib; nil for EN providers).
type Team struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}
