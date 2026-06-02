package transport

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// TestReportsValidation locks in the fix that footer "Обратная связь" feedback
// (player_type=feedback, no anime context) is accepted, while per-player error
// reports still require an anime_name.
func TestReportsValidation(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantSubmit bool
	}{
		{
			name:       "feedback without anime_name is accepted",
			body:       `{"player_type":"feedback","category":"bug","description":"Не рабит EN"}`,
			wantStatus: http.StatusOK,
			wantSubmit: true,
		},
		{
			name:       "player error without anime_name is rejected",
			body:       `{"player_type":"ourenglish","description":"black screen"}`,
			wantStatus: http.StatusBadRequest,
			wantSubmit: false,
		},
		{
			name:       "player error with anime_name is accepted",
			body:       `{"player_type":"ourenglish","anime_name":"Naruto","error_message":"hls fatal"}`,
			wantStatus: http.StatusOK,
			wantSubmit: true,
		},
		{
			name:       "empty player_type is rejected",
			body:       `{"description":"hi"}`,
			wantStatus: http.StatusBadRequest,
			wantSubmit: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got *domain.ReportRequest
			handler := reportsHandler(func(r domain.ReportRequest) {
				got = &r
			})

			req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantSubmit && got == nil {
				t.Fatalf("submitReport was not called")
			}
			if !tc.wantSubmit && got != nil {
				t.Fatalf("submitReport was called but report should have been rejected")
			}
		})
	}
}

// TestReportsCategoryDecoded ensures the category field added to ReportRequest is
// actually decoded from the wire (it was previously dropped on the maintenance side).
func TestReportsCategoryDecoded(t *testing.T) {
	var got *domain.ReportRequest
	handler := reportsHandler(func(r domain.ReportRequest) {
		got = &r
	})

	body := `{"player_type":"feedback","category":"feature","description":"add dark mode"}`
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("submitReport was not called")
	}
	if got.Category != "feature" {
		t.Fatalf("Category = %q, want %q", got.Category, "feature")
	}
}
