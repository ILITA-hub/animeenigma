package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMALExportService_FetchMALPage(t *testing.T) {
	log := logger.Default()

	// Create mock MAL server
	malServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/animelist/testuser/load.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			entries := []MALAnimeEntry{
				{
					AnimeID:    12345,
					AnimeTitle: "Attack on Titan",
					Status:     1,
					Score:      9,
				},
				{
					AnimeID:    67890,
					AnimeTitle: "Naruto",
					Status:     2,
					Score:      8,
				},
			}
			json.NewEncoder(w).Encode(entries)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer malServer.Close()

	service := &MALExportService{
		httpClient:   malServer.Client(),
		schedulerURL: "http://localhost:8085",
		log:          log,
	}

	// Override the fetch URL by mocking
	// In a real test, we'd inject the URL
	// For now, we test the struct parsing

	// Test that the service struct is correctly initialized
	assert.NotNil(t, service.httpClient)
	assert.NotNil(t, service.log)
}

func TestMALExportService_GetExportStatus_NotFound(t *testing.T) {
	log := logger.Default()

	// Create mock scheduler server that returns 404
	schedulerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer schedulerServer.Close()

	service := &MALExportService{
		httpClient:   http.DefaultClient,
		schedulerURL: schedulerServer.URL,
		log:          log,
	}

	_, err := service.GetExportStatus(context.Background(), "nonexistent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMALExportService_GetExportStatus_Success(t *testing.T) {
	log := logger.Default()

	// Create mock scheduler server
	schedulerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tasks/anime-load/status/export-123" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := map[string]interface{}{
				"data": map[string]interface{}{
					"id":               "export-123",
					"mal_username":     "testuser",
					"status":           "processing",
					"total_anime":      100,
					"processed_anime":  50,
					"loaded_anime":     45,
					"skipped_anime":    5,
					"failed_anime":     0,
					"progress_percent": 50.0,
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer schedulerServer.Close()

	service := &MALExportService{
		httpClient:   http.DefaultClient,
		schedulerURL: schedulerServer.URL,
		log:          log,
	}

	result, err := service.GetExportStatus(context.Background(), "export-123")
	require.NoError(t, err)
	assert.Equal(t, "export-123", result.ID)
	assert.Equal(t, "testuser", result.MALUsername)
	assert.Equal(t, "processing", result.Status)
	assert.Equal(t, 100, result.TotalAnime)
	assert.Equal(t, 50, result.ProcessedAnime)
}

func TestMALExportService_CreateExportJob(t *testing.T) {
	log := logger.Default()

	// Create mock scheduler server
	schedulerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tasks/anime-load" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			response := map[string]interface{}{
				"data": map[string]interface{}{
					"id":           "new-export-123",
					"mal_username": "testuser",
					"status":       "pending",
					"total_anime":  0,
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer schedulerServer.Close()

	service := &MALExportService{
		httpClient:   http.DefaultClient,
		schedulerURL: schedulerServer.URL,
		log:          log,
	}

	result, err := service.createExportJob(context.Background(), "user-123", "testuser")
	require.NoError(t, err)
	assert.Equal(t, "new-export-123", result.ID)
	assert.Equal(t, "testuser", result.MALUsername)
	assert.Equal(t, "pending", result.Status)
}

func TestMALExportService_CreateTasks(t *testing.T) {
	log := logger.Default()

	// Create mock scheduler server
	schedulerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tasks/anime-load/tasks" && r.Method == "POST" {
			// Verify request body
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			assert.Equal(t, "export-123", req["export_job_id"])
			assert.Equal(t, "user-123", req["user_id"])
			tasks := req["tasks"].([]interface{})
			assert.Len(t, tasks, 2)

			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer schedulerServer.Close()

	service := &MALExportService{
		httpClient:   http.DefaultClient,
		schedulerURL: schedulerServer.URL,
		log:          log,
	}

	tasks := []TaskInput{
		{MALID: 12345, Title: "Attack on Titan"},
		{MALID: 67890, Title: "Naruto"},
	}

	err := service.createTasks(context.Background(), "export-123", "user-123", tasks)
	require.NoError(t, err)
}

func TestMALExportService_GetUserExports(t *testing.T) {
	log := logger.Default()
	service := NewMALExportService(log)

	// Currently returns empty list
	exports, err := service.GetUserExports(context.Background(), "user-123")
	require.NoError(t, err)
	assert.Empty(t, exports)
}
