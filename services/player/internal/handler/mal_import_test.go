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

func TestMALImportHandler_ImportMALList_Unauthorized(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewMALImportHandler(nil, syncRepo, log)

	reqBody := map[string]string{"username": "testuser"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/import/mal", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ImportMALList(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMALImportHandler_ImportMALList_MissingUsername(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewMALImportHandler(nil, syncRepo, log)

	reqBody := map[string]string{"username": ""}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/import/mal", bytes.NewReader(body))

	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.ImportMALList(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMALImportHandler_ImportMALList_ReturnsActiveJob(t *testing.T) {
	db := setupSyncTestDB(t)
	syncRepo := repo.NewSyncRepository(db)
	log := logger.Default()
	handler := NewMALImportHandler(nil, syncRepo, log)

	// Create an active MAL job in DB
	activeJob := &domain.SyncJob{
		ID:             "existing-job",
		UserID:         "user-1",
		Source:         "mal",
		SourceUsername: "testuser",
		Status:         "processing",
		Total:          200,
		StartedAt:      time.Now(),
	}
	require.NoError(t, syncRepo.Create(context.Background(), activeJob))

	reqBody := map[string]string{"username": "testuser"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/import/mal", bytes.NewReader(body))

	claims := &authz.Claims{UserID: "user-1"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.ImportMALList(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	data, ok := response["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "existing-job", data["job_id"])
	assert.Equal(t, float64(200), data["total"])
}

func TestMALImportHandler_ConvertMALStatus(t *testing.T) {
	log := logger.Default()
	handler := NewMALImportHandler(nil, nil, log)

	tests := []struct {
		malStatus int
		expected  string
	}{
		{1, "watching"},
		{2, "completed"},
		{3, "on_hold"},
		{4, "dropped"},
		{6, "plan_to_watch"},
		{0, ""},
		{5, ""},
		{7, ""},
		{99, ""},
	}

	for _, tt := range tests {
		result := handler.convertMALStatus(tt.malStatus)
		assert.Equal(t, tt.expected, result, "convertMALStatus(%d)", tt.malStatus)
	}
}

func TestMALImportHandler_ParseMALDate(t *testing.T) {
	log := logger.Default()
	handler := NewMALImportHandler(nil, nil, log)

	tests := []struct {
		name     string
		dateStr  string
		wantNil  bool
		wantYear int
	}{
		{"empty string", "", true, 0},
		{"dash", "-", true, 0},
		{"MM-DD-YYYY", "01-15-2024", false, 2024},
		{"YYYY-MM-DD", "2024-01-15", false, 2024},
		{"invalid format", "not-a-date", true, 0},
		{"partial date", "2024-01", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.parseMALDate(tt.dateStr)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantYear, result.Year())
			}
		})
	}
}
