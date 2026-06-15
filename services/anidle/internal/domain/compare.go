package domain

// MatchStatus is the per-column verdict.
type MatchStatus string

const (
	MatchCorrect MatchStatus = "correct" // 🟩
	MatchPartial MatchStatus = "partial" // 🟨
	MatchWrong   MatchStatus = "wrong"   // ⬜
)

// Hint is the numeric direction (secret relative to the guess).
type Hint string

const (
	HintHigher Hint = "higher" // ↑
	HintLower  Hint = "lower"  // ↓
	HintNone   Hint = ""
)

// ColumnResult is one cell of a guess row.
type ColumnResult struct {
	Status MatchStatus `json:"status"`
	Hint   Hint        `json:"hint,omitempty"`
}

// GuessComparison is the full per-column result for one guess.
type GuessComparison struct {
	Genres   ColumnResult `json:"genres"`
	Studios  ColumnResult `json:"studios"`
	Year     ColumnResult `json:"year"`
	Episodes ColumnResult `json:"episodes"`
	Score    ColumnResult `json:"score"`
	Status   ColumnResult `json:"status"`
	Rating   ColumnResult `json:"rating"`
	Tags     ColumnResult `json:"tags"`
}
