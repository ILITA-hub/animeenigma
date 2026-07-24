//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// usersDB connects to the dev postgres and migrates the users table (incl. the
// new telegram_username / telegram_first_name columns). Run via `make dev`, then:
//
//	INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -v
func usersDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost port=5432 user=postgres password=postgres dbname=animeenigma sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}))
	return db
}

func containsID(users []domain.User, id string) bool {
	for _, u := range users {
		if u.ID == id {
			return true
		}
	}
	return false
}

func TestUpdateTelegramProfile_PersistsAndSkipsEmpty(t *testing.T) {
	db := usersDB(t)
	r := repo.NewUserRepository(db)
	ctx := context.Background()

	tg := int64(998811)
	u := &domain.User{Username: "tgprof_zzz", Role: authz.RoleUser, TelegramID: &tg}
	require.NoError(t, r.Create(ctx, u))
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", u.ID) })

	require.NoError(t, r.UpdateTelegramProfile(ctx, u.ID, "neo_tg", "Neo"))
	got, err := r.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TelegramUsername)
	require.Equal(t, "neo_tg", *got.TelegramUsername)
	require.NotNil(t, got.TelegramFirstName)
	require.Equal(t, "Neo", *got.TelegramFirstName)

	// Empty values must NOT null out previously-stored ones.
	require.NoError(t, r.UpdateTelegramProfile(ctx, u.ID, "", ""))
	got, err = r.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TelegramUsername)
	require.Equal(t, "neo_tg", *got.TelegramUsername)
}
