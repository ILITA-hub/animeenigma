package domain

// Combo is the natural key of a parser_episode_snapshots row and the
// per-combo unit the Phase 2 detector iterates over.
//
// Defined in `domain/` rather than `job/` (D-DET-04 import-cycle fix): both
// `repo/` and `job/` need to type-reference it, so it must live below
// both packages in the import graph. domain/ depends on nothing in this
// service, so it's the natural home.
//
// All six fields together form the composite key. Comparable (no slice/map
// fields) → usable as a map key in detector + repo bulk operations.
type Combo struct {
	AnimeID       string
	ShikimoriID   string
	Player        string
	Language      string
	WatchType     string
	TranslationID string
}
