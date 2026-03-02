package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShikimoriImportHandler_ImportShikimoriList_Unauthorized(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewShikimoriImportHandler(nil, syncRepo, log)

	reqBody := map[string]string{"nickname": "testuser"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/import/shikimori", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ImportShikimoriList(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestShikimoriImportHandler_ImportShikimoriList_MissingNickname(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewShikimoriImportHandler(nil, syncRepo, log)

	reqBody := map[string]string{"nickname": ""}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/import/shikimori", bytes.NewReader(body))

	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.ImportShikimoriList(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestShikimoriImportHandler_ImportShikimoriList_ReturnsActiveJob(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewShikimoriImportHandler(nil, syncRepo, log)

	// Create an active Shikimori job in DB
	activeJob := &domain.SyncJob{
		ID:             "shiki-existing",
		UserID:         "user-1",
		Source:         "shikimori",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          150,
		StartedAt:      time.Now(),
	}
	require.NoError(t, syncRepo.Create(context.Background(), activeJob))

	reqBody := map[string]string{"nickname": "testuser"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/import/shikimori", bytes.NewReader(body))

	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.ImportShikimoriList(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "shiki-existing", data["job_id"])
	assert.Equal(t, float64(150), data["total"])
}

func TestConvertShikimoriStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"planned", "plan_to_watch"},
		{"watching", "watching"},
		{"rewatching", "watching"},
		{"completed", "completed"},
		{"on_hold", "on_hold"},
		{"dropped", "dropped"},
		{"", ""},
		{"unknown", ""},
		{"WATCHING", ""},
	}

	for _, tt := range tests {
		result := convertShikimoriStatus(tt.status)
		assert.Equal(t, tt.expected, result, "convertShikimoriStatus(%q)", tt.status)
	}
}

func TestShikimoriImportHandler_MigrateShikimoriEntries_Unauthorized(t *testing.T) {
	log := logger.Default()
	handler := NewShikimoriImportHandler(nil, nil, log)

	req := httptest.NewRequest("POST", "/api/users/import/shikimori/migrate", nil)
	w := httptest.NewRecorder()

	handler.MigrateShikimoriEntries(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
