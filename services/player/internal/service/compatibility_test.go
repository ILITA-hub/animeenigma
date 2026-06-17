package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeCompatRepo struct{ data map[string][]domain.UserListEntry }

func (f *fakeCompatRepo) ListEntries(_ context.Context, uid string) ([]domain.UserListEntry, error) {
	return f.data[uid], nil
}

func TestCompatibility_IdenticalLists100(t *testing.T) {
	e := []domain.UserListEntry{{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "b", Score: 9, GenreIDs: []string{"g1"}}}
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{"v": e, "o": e}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 100, r.Percent)
	require.Equal(t, 2, r.SharedCount)
}

func TestCompatibility_NoOverlap0(t *testing.T) {
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{
		"v": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}},
		"o": {{AnimeID: "z", Score: 8, GenreIDs: []string{"g9"}}},
	}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 0, r.Percent)
	require.Equal(t, 0, r.SharedCount)
}

func TestCompatibility_PartialBlend(t *testing.T) {
	// overlap 1/3 titles, identical scores on shared, identical genre vectors
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{
		"v": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "b", Score: 7, GenreIDs: []string{"g1"}}},
		"o": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "c", Score: 6, GenreIDs: []string{"g1"}}},
	}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	// overlap = 1/3 ; scoreAgreement = 1 (identical on shared "a") ; genreSim = 1
	// score = 0.5*0.333 + 0.4*1 + 0.1*1 = 0.6667 -> 67
	require.InDelta(t, 67, r.Percent, 1)
	require.Equal(t, 1, r.SharedCount)
}
