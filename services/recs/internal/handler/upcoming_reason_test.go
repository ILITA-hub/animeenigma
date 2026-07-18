package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/repo"
)

// attributeReason resolves the strongest shared attribute. Studio is covered
// end-to-end by TestUpcoming_StandaloneAdmittedByAttributeAffinity; here we
// pin the source fallback and the no-overlap nil.
func TestAttributeReason_SourceFallbackWhenNoStudioShared(t *testing.T) {
	db := newUpcomingTestDB(t)
	// Candidate shares NO studio, but is a manga adaptation and the user's
	// history is manga-heavy (>= upcomingSourceMinWatched).
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, material_source, status) VALUES
		('cand', 'Manga Thing', 'manga', 'announced'),
		('w1', 'A', 'manga', 'released'),
		('w2', 'B', 'manga', 'released'),
		('w3', 'C', 'manga', 'released')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO watch_history (id, user_id, anime_id) VALUES
		('h1','u1','w1'), ('h2','u1','w2'), ('h3','u1','w3')`).Error)

	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), nil,
		UpcomingConfig{})
	// nil logger is fine — resolveReason logs only on error; attributeReason here
	// returns cleanly. Call attributeReason directly.
	got, err := h.attributeReason(context.Background(), "u1", "cand")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "attribute", got.Kind)
	assert.Equal(t, "source", got.Attribute)
	assert.Equal(t, "manga", got.AttributeName)
}

func TestAttributeReason_NilWhenNothingShared(t *testing.T) {
	db := newUpcomingTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO animes (id, name, material_source, status) VALUES
		('cand', 'Lonely Original', 'original', 'announced')`).Error)
	h := NewUpcomingHandler(db, repo.NewAnnouncementDismissalsRepository(db), newFakeRecsCache(), nil,
		UpcomingConfig{})
	got, err := h.attributeReason(context.Background(), "u1", "cand")
	require.NoError(t, err)
	assert.Nil(t, got, "no shared studio and an 'original' source yields no attribute reason")
}
