package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestMALExportHandler_InitiateExport_Unauthorized(t *testing.T) {
	log := logger.Default()
	exportService := service.NewMALExportService(log)
	handler := NewMALExportHandler(exportService, log)

	// Request without auth context
	reqBody := map[string]string{"mal_username": "testuser"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/mal-export", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.InitiateExport(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMALExportHandler_InitiateExport_MissingUsername(t *testing.T) {
	log := logger.Default()
	exportService := service.NewMALExportService(log)
	handler := NewMALExportHandler(exportService, log)

	// Request with empty username
	reqBody := map[string]string{"mal_username": ""}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/users/mal-export", bytes.NewReader(body))

	// Add auth context
	claims := &authz.Claims{UserID: "user-123"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.InitiateExport(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMALExportHandler_GetExportStatus_Unauthorized(t *testing.T) {
	log := logger.Default()
	exportService := service.NewMALExportService(log)
	handler := NewMALExportHandler(exportService, log)

	// Create router with chi context
	r := chi.NewRouter()
	r.Get("/api/users/mal-export/{exportId}", handler.GetExportStatus)

	req := httptest.NewRequest("GET", "/api/users/mal-export/export-123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMALExportHandler_CancelExport_Unauthorized(t *testing.T) {
	log := logger.Default()
	exportService := service.NewMALExportService(log)
	handler := NewMALExportHandler(exportService, log)

	// Create router with chi context
	r := chi.NewRouter()
	r.Delete("/api/users/mal-export/{exportId}", handler.CancelExport)

	req := httptest.NewRequest("DELETE", "/api/users/mal-export/export-123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMALExportHandler_GetUserExports_Unauthorized(t *testing.T) {
	log := logger.Default()
	exportService := service.NewMALExportService(log)
	handler := NewMALExportHandler(exportService, log)

	req := httptest.NewRequest("GET", "/api/users/mal-export", nil)
	w := httptest.NewRecorder()

	handler.GetUserExports(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMALExportHandler_GetUserExports_Authorized(t *testing.T) {
	log := logger.Default()
	exportService := service.NewMALExportService(log)
	handler := NewMALExportHandler(exportService, log)

	req := httptest.NewRequest("GET", "/api/users/mal-export", nil)

	// Add auth context
	claims := &authz.Claims{UserID: "user-123"}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.GetUserExports(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "data")
}
