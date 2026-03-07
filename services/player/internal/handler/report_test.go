package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveReportToDisk_ValidPlayerType(t *testing.T) {
	tmpDir := t.TempDir()
	log := logger.Default()
	h := NewReportHandler(log, "", "", tmpDir)

	claims := &authz.Claims{UserID: "user-1", Username: "testuser"}
	report := &domain.ErrorReport{
		PlayerType:  "hianime",
		AnimeID:     "anime-123",
		AnimeName:   "Test Anime",
		ConsoleLogs: "[]",
		NetworkLogs: "[]",
	}

	filename := h.saveReportToDisk(claims, report)

	require.NotEmpty(t, filename, "saveReportToDisk should return a non-empty filename for valid player type")
	assert.FileExists(t, filename)

	// Verify the file contains valid JSON
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"player_type": "hianime"`)
}

func TestSaveReportToDisk_InvalidPlayerType(t *testing.T) {
	tmpDir := t.TempDir()
	log := logger.Default()
	h := NewReportHandler(log, "", "", tmpDir)

	claims := &authz.Claims{UserID: "user-1", Username: "testuser"}

	tests := []struct {
		name       string
		playerType string
	}{
		{"path traversal", "../../../etc/passwd"},
		{"malicious path", "malicious/path"},
		{"empty string", ""},
		{"dot dot", ".."},
		{"absolute path", "/etc/shadow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &domain.ErrorReport{
				PlayerType: tt.playerType,
				AnimeID:    "anime-123",
			}

			filename := h.saveReportToDisk(claims, report)
			assert.Empty(t, filename, "saveReportToDisk should return empty string for invalid player type %q", tt.playerType)
		})
	}

	// Verify no files were created outside the reports dir
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no files should be created for invalid player types")
}

func TestSaveReportToDisk_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	log := logger.Default()
	h := NewReportHandler(log, "", "", tmpDir)

	claims := &authz.Claims{UserID: "user-1", Username: "testuser"}
	report := &domain.ErrorReport{
		PlayerType:  "consumet",
		AnimeID:     "anime-456",
		ConsoleLogs: "[]",
		NetworkLogs: "[]",
	}

	filename := h.saveReportToDisk(claims, report)
	require.NotEmpty(t, filename)

	info, err := os.Stat(filename)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "report file should have 0600 permissions")
}

func TestSaveReportToDisk_UsernameSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	log := logger.Default()
	h := NewReportHandler(log, "", "", tmpDir)

	tests := []struct {
		name     string
		username string
	}{
		{"path traversal in username", "user@evil/../../"},
		{"special characters", "user<script>alert(1)</script>"},
		{"slashes", "../../etc/passwd"},
		{"spaces and dots", "user name.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &authz.Claims{UserID: "user-1", Username: tt.username}
			report := &domain.ErrorReport{
				PlayerType:  "kodik",
				AnimeID:     "anime-789",
				ConsoleLogs: "[]",
				NetworkLogs: "[]",
			}

			filename := h.saveReportToDisk(claims, report)
			require.NotEmpty(t, filename, "should still save report for sanitized username")

			// Verify the file was created inside the reports directory (no path traversal)
			absReports, _ := filepath.Abs(tmpDir)
			absFile, _ := filepath.Abs(filename)
			assert.True(t, strings.HasPrefix(absFile, absReports),
				"report file %q should be inside reports dir %q", absFile, absReports)

			// Verify the filename does not contain path separators or special chars
			base := filepath.Base(filename)
			assert.NotContains(t, base, "/")
			assert.NotContains(t, base, "@")
			assert.NotContains(t, base, "<")
			assert.NotContains(t, base, ">")
		})
	}
}

func TestSaveReportToDisk_EmptyReportsDir(t *testing.T) {
	log := logger.Default()
	h := NewReportHandler(log, "", "", "")

	claims := &authz.Claims{UserID: "user-1", Username: "testuser"}
	report := &domain.ErrorReport{
		PlayerType:  "hianime",
		AnimeID:     "anime-123",
		ConsoleLogs: "[]",
		NetworkLogs: "[]",
	}

	filename := h.saveReportToDisk(claims, report)
	assert.Empty(t, filename, "should return empty string when reportsDir is empty")
}
