package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLooksLikeSequel(t *testing.T) {
	cases := []struct {
		name   string
		nameRU string
		want   bool
	}{
		// Real announced sequel names — must be detected.
		{"Witch Watch 2nd Season", "", true},
		{"Dandadan 3rd Season", "", true},
		{"Kusuriya no Hitorigoto 3rd Season", "", true},
		{"Spy x Family Season 3", "", true},
		{"Mahou Shoujo ni Akogarete 2nd Season", "", true},
		{"JoJo's Bizarre Adventure Part 6", "", true},
		{"Overlord IV", "", true},
		{"Bocchi the Rock! 2nd Season", "", true},
		{"Some Show 2nd Cour", "", true},
		{"", "Ван-Пис 2 сезон", true},
		{"", "Клинок 3-й сезон", true},
		{"", "Атака титанов часть 2", true},
		// First-season / standalone — must NOT be flagged.
		{"Frieren: Beyond Journey's End", "", false},
		{"Witch Watch", "", false},
		{"86", "", false},
		{"Mob Psycho 100", "", false},
		{"5-toubun no Hanayome", "", false},
		{"Kimetsu Academy", "", false},
		{"Steins;Gate", "", false},
	}
	for _, c := range cases {
		got := looksLikeSequel(c.name, c.nameRU)
		assert.Equalf(t, c.want, got, "looksLikeSequel(%q,%q)", c.name, c.nameRU)
	}
}

func newContinuationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id        TEXT PRIMARY KEY,
		franchise TEXT NOT NULL DEFAULT '',
		status    TEXT NOT NULL DEFAULT 'announced'
	)`).Error)
	return db
}

func TestFranchiseHasAiredSibling(t *testing.T) {
	db := newContinuationTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO animes (id, franchise, status) VALUES
		('s1',   'witch-watch', 'released'),   -- aired prior entry
		('cand-cont', 'witch-watch', 'announced'),
		('new-s1',    'brand-new',   'announced'), -- new franchise, only announced
		('new-s2',    'brand-new',   'announced'),
		('cand-nofr', '',            'announced')  -- empty franchise
	`).Error)

	h := &UpcomingHandler{db: db}
	got, err := h.franchiseHasAiredSibling(context.Background(),
		[]string{"cand-cont", "new-s1", "new-s2", "cand-nofr"})
	require.NoError(t, err)

	assert.True(t, got["cand-cont"], "franchise with a released sibling is a continuation")
	assert.False(t, got["new-s1"], "new franchise with only announced entries is not")
	assert.False(t, got["new-s2"], "new franchise with only announced entries is not")
	assert.False(t, got["cand-nofr"], "empty franchise is never a structural continuation")
}
