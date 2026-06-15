package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

func tx(ids ...string) []domain.Taxon {
	out := make([]domain.Taxon, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.Taxon{ID: id, Name: id})
	}
	return out
}

func TestCompare_SetColumns(t *testing.T) {
	secret := domain.PoolAnime{Genres: tx("a", "b")}
	// equal set -> correct
	assert.Equal(t, domain.MatchCorrect, Compare(secret, domain.PoolAnime{Genres: tx("a", "b")}).Genres.Status)
	// overlap -> partial
	assert.Equal(t, domain.MatchPartial, Compare(secret, domain.PoolAnime{Genres: tx("a", "c")}).Genres.Status)
	// disjoint -> wrong
	assert.Equal(t, domain.MatchWrong, Compare(secret, domain.PoolAnime{Genres: tx("x", "y")}).Genres.Status)
	// both empty -> correct (equal empty sets)
	assert.Equal(t, domain.MatchCorrect, Compare(domain.PoolAnime{}, domain.PoolAnime{}).Genres.Status)
}

func TestCompare_Numeric(t *testing.T) {
	secret := domain.PoolAnime{Year: 2023, Episodes: 28, Score: 9.3}
	r := Compare(secret, domain.PoolAnime{Year: 2020, Episodes: 24, Score: 8.6})
	assert.Equal(t, domain.MatchWrong, r.Year.Status)
	assert.Equal(t, domain.HintHigher, r.Year.Hint) // secret 2023 > guess 2020
	assert.Equal(t, domain.HintHigher, r.Episodes.Hint)
	assert.Equal(t, domain.HintHigher, r.Score.Hint)

	r2 := Compare(secret, domain.PoolAnime{Year: 2025, Episodes: 28, Score: 9.3})
	assert.Equal(t, domain.HintLower, r2.Year.Hint) // secret 2023 < guess 2025
	assert.Equal(t, domain.MatchCorrect, r2.Episodes.Status)
	assert.Equal(t, domain.HintNone, r2.Episodes.Hint)
	assert.Equal(t, domain.MatchCorrect, r2.Score.Status)
}

func TestCompare_Enum(t *testing.T) {
	secret := domain.PoolAnime{Status: "released", Rating: "pg_13"}
	r := Compare(secret, domain.PoolAnime{Status: "released", Rating: "r"})
	assert.Equal(t, domain.MatchCorrect, r.Status.Status)
	assert.Equal(t, domain.MatchWrong, r.Rating.Status)
}

func TestCompare_FullMatchAllGreen(t *testing.T) {
	a := domain.PoolAnime{
		Year: 2023, Episodes: 28, Score: 9.3, Status: "released", Rating: "pg_13",
		Genres: tx("a"), Studios: tx("s"), Tags: tx("t"),
	}
	r := Compare(a, a)
	for _, c := range []domain.ColumnResult{r.Genres, r.Studios, r.Year, r.Episodes, r.Score, r.Status, r.Rating, r.Tags} {
		assert.Equal(t, domain.MatchCorrect, c.Status)
		assert.Equal(t, domain.HintNone, c.Hint)
	}
}
