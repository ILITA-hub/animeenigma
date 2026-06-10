// Package handler — my_reports.go: the user-facing "my feedback" listing.
//
// GET /api/users/reports — paginated list of the AUTHENTICATED user's own
// feedback / error reports (the same on-disk archive admin_reports.go
// triages), newest first. User-safe row shape: triage statuses are exposed
// (the frontend maps them to friendly labels) but the heavy diagnostics,
// other users' identities, and attachment internals are not.
//
// Mounted in the JWT-protected /api/users group (no admin gate) — see
// services/player/internal/transport/router.go.
package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// myReportRow is what a submitter sees about their own message. Description
// is returned in full (it is their own text, and it's capped at submit time).
type myReportRow struct {
	ID              string `json:"id"`
	Timestamp       string `json:"timestamp"`
	PlayerType      string `json:"player_type"`
	Category        string `json:"category"`
	AnimeName       string `json:"anime_name,omitempty"`
	EpisodeNumber   *int   `json:"episode_number,omitempty"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	StatusUpdatedAt string `json:"status_updated_at,omitempty"`
}

// myReportFile is the on-disk subset needed to build a row + match the owner.
type myReportFile struct {
	UserID        string `json:"user_id"`
	Timestamp     string `json:"timestamp"`
	PlayerType    string `json:"player_type"`
	Category      string `json:"category"`
	AnimeName     string `json:"anime_name"`
	EpisodeNumber *int   `json:"episode_number,omitempty"`
	Description   string `json:"description"`
}

// ListMine returns the current user's reports, newest first.
// Query params: page, page_size (same limits as the admin list).
func (h *AdminReportsHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}

	empty := map[string]interface{}{"items": []myReportRow{}, "total": 0, "page": page, "page_size": pageSize}
	if h.reportsDir == "" {
		httputil.OK(w, empty)
		return
	}

	entries, err := os.ReadDir(h.reportsDir)
	if err != nil {
		h.log.Warnw("failed to read reports dir", "path", h.reportsDir, "error", err)
		httputil.OK(w, empty)
		return
	}

	// Filenames begin with an ISO timestamp, so a reverse name sort == newest first.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "_") || !strings.HasSuffix(n, ".json") {
			continue
		}
		names = append(names, n)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	h.mu.Lock()
	statuses := h.loadStatuses()
	h.mu.Unlock()

	mine := make([]myReportRow, 0, 16)
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(h.reportsDir, n))
		if err != nil {
			continue
		}
		var f myReportFile
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		if f.UserID != claims.UserID {
			continue
		}
		id := strings.TrimSuffix(n, ".json")
		row := myReportRow{
			ID:            id,
			Timestamp:     f.Timestamp,
			PlayerType:    f.PlayerType,
			Category:      f.Category,
			AnimeName:     f.AnimeName,
			EpisodeNumber: f.EpisodeNumber,
			Description:   f.Description,
			Status:        "new",
		}
		if st, ok := statuses[id]; ok && st.Status != "" {
			row.Status = st.Status
			row.StatusUpdatedAt = st.UpdatedAt
		}
		mine = append(mine, row)
	}

	total := len(mine)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	httputil.OK(w, map[string]interface{}{
		"items":     mine[start:end],
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
