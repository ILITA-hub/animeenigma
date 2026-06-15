package domain

// Taxon is an id+name pair (genre/studio/tag). anidle compares by ID.
type Taxon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PoolAnime is one guessable anime, decoded from catalog's
// GET /internal/guessgame/pool. Field shape MUST match the catalog DTO
// (services/catalog/internal/service/guesspool.go GuessPoolEntry).
type PoolAnime struct {
	ID        string  `json:"id"`
	NameRU    string  `json:"name_ru"`
	NameEN    string  `json:"name_en"`
	NameJP    string  `json:"name_jp"`
	PosterURL string  `json:"poster_url"`
	Year      int     `json:"year"`
	Episodes  int     `json:"episodes"`
	Score     float64 `json:"score"`
	Status    string  `json:"status"`
	Rating    string  `json:"rating"`
	Genres    []Taxon `json:"genres"`
	Studios   []Taxon `json:"studios"`
	Tags      []Taxon `json:"tags"`
}
