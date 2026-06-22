package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// TestSendToMaintenance_SetsTokenHeader proves the player forwards the
// X-Maintenance-Token shared secret so the maintenance /api/reports gate
// (finding #39) accepts its reports.
func TestSendToMaintenance_SetsTokenHeader(t *testing.T) {
	var gotToken, gotPath, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Maintenance-Token")
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewReportHandler(logger.Default(), "", "", "", srv.URL, "tok-123", nil)
	claims := &authz.Claims{UserID: "u1", Username: "user"}
	report := &domain.ErrorReport{PlayerType: "ourenglish", AnimeName: "Naruto"}

	if err := h.sendToMaintenance(claims, report, ""); err != nil {
		t.Fatalf("sendToMaintenance returned error: %v", err)
	}
	if gotPath != "/api/reports" {
		t.Fatalf("path = %q, want /api/reports", gotPath)
	}
	if gotToken != "tok-123" {
		t.Fatalf("X-Maintenance-Token = %q, want tok-123", gotToken)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotCT)
	}
}

// TestSendToMaintenance_NoTokenOmitsHeader: with no token configured the header
// is absent, so open maintenance endpoints stay reachable.
func TestSendToMaintenance_NoTokenOmitsHeader(t *testing.T) {
	var hadHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadHeader = r.Header["X-Maintenance-Token"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := NewReportHandler(logger.Default(), "", "", "", srv.URL, "", nil)
	claims := &authz.Claims{UserID: "u1", Username: "user"}
	report := &domain.ErrorReport{PlayerType: "ourenglish", AnimeName: "Naruto"}

	if err := h.sendToMaintenance(claims, report, ""); err != nil {
		t.Fatalf("sendToMaintenance returned error: %v", err)
	}
	if hadHeader {
		t.Fatal("X-Maintenance-Token header present when no token configured")
	}
}
