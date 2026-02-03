package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Auto-migrate test tables
	if err := db.AutoMigrate(&domain.MALShikimoriMapping{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase",
			input:    "Attack on Titan",
			expected: "attack on titan",
		},
		{
			name:     "remove colons",
			input:    "Sword Art Online: Alicization",
			expected: "sword art online alicization",
		},
		{
			name:     "replace dashes with spaces",
			input:    "Re-Zero",
			expected: "re zero",
		},
		{
			name:     "trim whitespace",
			input:    "  Naruto  ",
			expected: "naruto",
		},
		{
			name:     "multiple transformations",
			input:    "  Attack on Titan: Final Season - Part 2  ",
			expected: "attack on titan final season  part 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMALResolver_Resolve_CachedMapping(t *testing.T) {
	db := setupTestDB(t)
	mappingRepo := repo.NewMappingRepository(db)
	log := logger.Default()
	cfg := &config.JobsConfig{
		CatalogServiceURL: "http://localhost:8081",
		ShikimoriAPIURL:   "http://localhost:8000",
	}

	resolver := NewMALResolver(mappingRepo, cfg, log)

	// Create a cached mapping
	mapping := &domain.MALShikimoriMapping{
		MALID:       12345,
		ShikimoriID: "z12345",
		AnimeID:     "uuid-123",
		Confidence:  1.0,
		Source:      domain.MappingSourceShikimoriAPI,
	}
	err := mappingRepo.Create(context.Background(), mapping)
	assert.NoError(t, err)

	// Create test task
	task := &domain.AnimeLoadTask{
		MALID:    12345,
		MALTitle: "Test Anime",
	}

	// Resolve should return cached mapping
	result := resolver.Resolve(context.Background(), task)

	assert.Equal(t, "z12345", result.ShikimoriID)
	assert.Equal(t, "uuid-123", result.AnimeID)
	assert.Equal(t, domain.ResolutionCached, result.Method)
	assert.Equal(t, 1.0, result.Confidence)
}

func TestMALResolver_Resolve_CatalogLookup(t *testing.T) {
	db := setupTestDB(t)
	mappingRepo := repo.NewMappingRepository(db)
	log := logger.Default()

	// Create mock catalog server
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/anime/mal/12345" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":{"id":"uuid-catalog","shikimori_id":"z12345","name":"Test Anime"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer catalogServer.Close()

	cfg := &config.JobsConfig{
		CatalogServiceURL: catalogServer.URL,
		ShikimoriAPIURL:   "http://localhost:8000",
	}

	resolver := NewMALResolver(mappingRepo, cfg, log)

	task := &domain.AnimeLoadTask{
		MALID:    12345,
		MALTitle: "Test Anime",
	}

	result := resolver.Resolve(context.Background(), task)

	assert.Equal(t, "z12345", result.ShikimoriID)
	assert.Equal(t, "uuid-catalog", result.AnimeID)
	assert.Equal(t, domain.ResolutionCached, result.Method)
}

func TestMALResolver_Resolve_ExactJapaneseMatch(t *testing.T) {
	db := setupTestDB(t)
	mappingRepo := repo.NewMappingRepository(db)
	log := logger.Default()

	// Create mock Shikimori server
	shikimoriServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/anime" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Return an anime with matching Japanese title
			w.Write([]byte(`[{"id":"z54321","name":"Shingeki no Kyojin","japanese":"進撃の巨人"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer shikimoriServer.Close()

	// Create mock catalog server that returns not found
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer catalogServer.Close()

	cfg := &config.JobsConfig{
		CatalogServiceURL: catalogServer.URL,
		ShikimoriAPIURL:   shikimoriServer.URL,
		ShikimoriAppName:  "TestApp",
	}

	resolver := NewMALResolver(mappingRepo, cfg, log)

	task := &domain.AnimeLoadTask{
		MALID:            12345,
		MALTitle:         "Shingeki no Kyojin",
		MALTitleJapanese: "進撃の巨人",
	}

	result := resolver.Resolve(context.Background(), task)

	assert.Equal(t, "z54321", result.ShikimoriID)
	assert.Equal(t, domain.ResolutionExactJapanese, result.Method)
	assert.Equal(t, 1.0, result.Confidence)
}

func TestMALResolver_Resolve_ExactRomanizedMatch(t *testing.T) {
	db := setupTestDB(t)
	mappingRepo := repo.NewMappingRepository(db)
	log := logger.Default()

	// Create mock Shikimori server that handles the search query
	// The URL constructed is: baseURL + /api/anime?search=...
	// So we need the server to handle the full path
	shikimoriServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Shikimori received request: %s %s", r.Method, r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"z54321","name":"Shingeki no Kyojin","japanese":"進撃の巨人"}]`))
	}))
	defer shikimoriServer.Close()

	// Create mock catalog server that returns not found
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Catalog received request: %s %s", r.Method, r.URL.String())
		w.WriteHeader(http.StatusNotFound)
	}))
	defer catalogServer.Close()

	// Note: The ShikimoriAPIURL should NOT include /api since the resolver appends /api/anime
	cfg := &config.JobsConfig{
		CatalogServiceURL: catalogServer.URL,
		ShikimoriAPIURL:   shikimoriServer.URL, // Server URL WITHOUT /api
		ShikimoriAppName:  "TestApp",
	}

	resolver := NewMALResolver(mappingRepo, cfg, log)

	task := &domain.AnimeLoadTask{
		MALID:    12345,
		MALTitle: "Shingeki no Kyojin",
		// No Japanese title - should fall back to romanized match
	}

	result := resolver.Resolve(context.Background(), task)

	assert.Equal(t, "z54321", result.ShikimoriID)
	assert.Equal(t, domain.ResolutionExactRomanized, result.Method)
	assert.Equal(t, 1.0, result.Confidence)
}

func TestMALResolver_Resolve_NoMatch(t *testing.T) {
	db := setupTestDB(t)
	mappingRepo := repo.NewMappingRepository(db)
	log := logger.Default()

	// Create mock Shikimori server that returns different anime
	shikimoriServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/anime" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"z99999","name":"Different Anime","japanese":"違うアニメ"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer shikimoriServer.Close()

	// Create mock catalog server that returns not found
	catalogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer catalogServer.Close()

	cfg := &config.JobsConfig{
		CatalogServiceURL: catalogServer.URL,
		ShikimoriAPIURL:   shikimoriServer.URL,
		ShikimoriAppName:  "TestApp",
	}

	resolver := NewMALResolver(mappingRepo, cfg, log)

	task := &domain.AnimeLoadTask{
		MALID:            12345,
		MALTitle:         "Unique Anime Title",
		MALTitleJapanese: "ユニークなアニメ",
	}

	result := resolver.Resolve(context.Background(), task)

	assert.Equal(t, domain.ResolutionNotFound, result.Method)
	assert.Empty(t, result.ShikimoriID)
}
