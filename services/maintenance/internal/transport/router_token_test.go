package transport

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

const tokenTestBody = `{"player_type":"feedback","category":"bug","description":"x"}`

// TestReportsTokenAuth locks the shared-secret gate on /api/reports: the player
// service (sole legitimate caller) must present X-Maintenance-Token. A wrong or
// missing token is rejected before submitReport runs.
func TestReportsTokenAuth(t *testing.T) {
	const secret = "s3cr3t-token"
	cases := []struct {
		name       string
		setHeader  bool
		token      string
		wantStatus int
		wantSubmit bool
	}{
		{"correct token accepted", true, secret, http.StatusOK, true},
		{"wrong token rejected", true, "nope", http.StatusUnauthorized, false},
		{"missing token rejected", false, "", http.StatusUnauthorized, false},
		{"empty token header rejected", true, "", http.StatusUnauthorized, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *domain.ReportRequest
			h := reportsTokenAuth(secret, reportsHandler(func(r domain.ReportRequest) { got = &r }))

			req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewBufferString(tokenTestBody))
			req.Header.Set("Content-Type", "application/json")
			if tc.setHeader {
				req.Header.Set("X-Maintenance-Token", tc.token)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantSubmit && got == nil {
				t.Fatal("submitReport not called for authorized request")
			}
			if !tc.wantSubmit && got != nil {
				t.Fatal("submitReport called for an unauthorized request")
			}
		})
	}
}

// post is a small helper that runs body through handler with an optional token.
func post(h http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewBufferString(tokenTestBody))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Maintenance-Token", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestBuildReportsHandler_Gated verifies that a configured token wraps the
// reports handler in the shared-secret gate.
func TestBuildReportsHandler_Gated(t *testing.T) {
	called := false
	h := buildReportsHandler(func(domain.ReportRequest) { called = true }, "tok")

	if rec := post(h, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("submitReport ran for a request with no token")
	}
	if rec := post(h, "tok"); rec.Code != http.StatusOK {
		t.Fatalf("good-token status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("submitReport did not run for an authorized request")
	}
}

// TestBuildReportsHandler_Open keeps the backward-compatible open posture: with
// no token configured the endpoint stays unauthenticated (the daemon logs a
// WARN at startup; the gate is opt-in via REPORTS_AUTH_TOKEN).
func TestBuildReportsHandler_Open(t *testing.T) {
	called := false
	h := buildReportsHandler(func(domain.ReportRequest) { called = true }, "")

	if rec := post(h, ""); rec.Code != http.StatusOK {
		t.Fatalf("open-mode status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("submitReport did not run in open mode")
	}
}

// TestNewRouter_ReportsTokenWiring proves the gate is mounted on the live chi
// router (one NewRouter call — promauto registers on the global registry, so a
// second call in-process would panic; branch coverage lives in the
// buildReportsHandler tests above).
func TestNewRouter_ReportsTokenWiring(t *testing.T) {
	called := false
	r := NewRouter(func(domain.ReportRequest) { called = true }, nil, "", "", "tok")

	if rec := post(r, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("router no-token status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("submitReport ran through the router with no token")
	}
	if rec := post(r, "tok"); rec.Code != http.StatusOK {
		t.Fatalf("router good-token status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("submitReport did not run through the router for an authorized request")
	}
}
