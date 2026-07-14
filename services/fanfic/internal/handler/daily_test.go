package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/groq"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/service"
)

type fakeDaily struct {
	pick    *domain.Fanfic
	pickErr error

	ensureRes service.EnsureResult
	ensureErr error
}

func (f *fakeDaily) DailyPick(context.Context) (*domain.Fanfic, error) { return f.pick, f.pickErr }
func (f *fakeDaily) EnsureDaily(context.Context) (service.EnsureResult, error) {
	return f.ensureRes, f.ensureErr
}

func TestDailyInternal_ReturnsCompactDTO(t *testing.T) {
	pick := &domain.Fanfic{ID: "f1", Title: "T", Rating: "teen", Content: "Body paragraph."}
	h := NewDailyHandler(&fakeDaily{pick: pick})
	rec := httptest.NewRecorder()
	h.Internal(rec, httptest.NewRequest(http.MethodGet, "/internal/fanfic/daily", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Data service.DailyDTO `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.ID != "f1" || resp.Data.Excerpt == "" {
		t.Errorf("got %+v, want id=f1 with a non-empty excerpt", resp.Data)
	}
}

func TestDailyInternal_404WhenNil(t *testing.T) {
	h := NewDailyHandler(&fakeDaily{pick: nil})
	rec := httptest.NewRecorder()
	h.Internal(rec, httptest.NewRequest(http.MethodGet, "/internal/fanfic/daily", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDailyPublic_NonExplicit_FullContent(t *testing.T) {
	pick := &domain.Fanfic{ID: "f1", Title: "T", Rating: "teen", Content: "Full body here."}
	h := NewDailyHandler(&fakeDaily{pick: pick})
	rec := httptest.NewRecorder()
	h.Public(rec, httptest.NewRequest(http.MethodGet, "/api/fanfic/daily", nil))

	var resp struct {
		Data publicDaily `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Content != "Full body here." {
		t.Errorf("content = %q, want full body", resp.Data.Content)
	}
	if resp.Data.Gated {
		t.Error("non-explicit pick must not be gated")
	}
}

func TestDailyPublic_Explicit_AnonGatesWithLogin(t *testing.T) {
	pick := &domain.Fanfic{ID: "f1", Title: "T", Rating: "explicit", Content: "Explicit body."}
	h := NewDailyHandler(&fakeDaily{pick: pick})
	rec := httptest.NewRecorder()
	h.Public(rec, httptest.NewRequest(http.MethodGet, "/api/fanfic/daily", nil)) // no claims on context: anon

	var resp struct {
		Data publicDaily `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Content != "" {
		t.Errorf("content leaked for explicit pick: %q", resp.Data.Content)
	}
	if !resp.Data.Gated || resp.Data.GateReason != "login" {
		t.Errorf("gated=%v reason=%q, want true/login for anon", resp.Data.Gated, resp.Data.GateReason)
	}
}

func TestDailyPublic_Explicit_LoggedInGatesWithAdultSetting(t *testing.T) {
	pick := &domain.Fanfic{ID: "f1", Title: "T", Rating: "explicit", Content: "Explicit body."}
	h := NewDailyHandler(&fakeDaily{pick: pick})
	req := httptest.NewRequest(http.MethodGet, "/api/fanfic/daily", nil)
	req = req.WithContext(authz.ContextWithClaims(req.Context(), &authz.Claims{UserID: "u-1"}))
	rec := httptest.NewRecorder()
	h.Public(rec, req)

	var resp struct {
		Data publicDaily `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Content != "" {
		t.Errorf("content leaked for explicit pick: %q", resp.Data.Content)
	}
	if !resp.Data.Gated || resp.Data.GateReason != "adult_setting" {
		t.Errorf("gated=%v reason=%q, want true/adult_setting for logged-in user", resp.Data.Gated, resp.Data.GateReason)
	}
}

func TestDailyPublic_404WhenNil(t *testing.T) {
	h := NewDailyHandler(&fakeDaily{pick: nil})
	rec := httptest.NewRecorder()
	h.Public(rec, httptest.NewRequest(http.MethodGet, "/api/fanfic/daily", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestEnsure_Generated(t *testing.T) {
	h := NewDailyHandler(&fakeDaily{ensureRes: service.EnsureResult{Generated: true, Reason: "generated", FanficID: "f1"}})
	rec := httptest.NewRecorder()
	h.Ensure(rec, httptest.NewRequest(http.MethodPost, "/internal/fanfic/ensure-daily", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Data ensureResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Data.Generated || resp.Data.FanficID != "f1" {
		t.Errorf("resp = %+v, want generated=true fanfic_id=f1", resp.Data)
	}
}

func TestEnsure_NoOp(t *testing.T) {
	h := NewDailyHandler(&fakeDaily{ensureRes: service.EnsureResult{Generated: false, Reason: "user_exists"}})
	rec := httptest.NewRecorder()
	h.Ensure(rec, httptest.NewRequest(http.MethodPost, "/internal/fanfic/ensure-daily", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Data ensureResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Generated || resp.Data.Reason != "user_exists" {
		t.Errorf("resp = %+v, want generated=false reason=user_exists", resp.Data)
	}
}

// TestEnsure_GroqAuthFailure_Returns200 verifies the groq-auth classification:
// EnsureDaily already fired the Telegram alert (that's the operator signal),
// so the handler must respond 200 with error:"groq_auth" — NOT 500 — so the
// scheduler records a normal "ran successfully" tick instead of a
// retry-worthy fault.
func TestEnsure_GroqAuthFailure_Returns200(t *testing.T) {
	wrapped := fmt.Errorf("ensure-daily: groq: %w", &groq.StatusError{Code: http.StatusUnauthorized, Body: "bad key"})
	h := NewDailyHandler(&fakeDaily{ensureErr: wrapped})
	rec := httptest.NewRecorder()
	h.Ensure(rec, httptest.NewRequest(http.MethodPost, "/internal/fanfic/ensure-daily", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (groq auth failure already alerted upstream)", rec.Code)
	}
	var resp struct {
		Data ensureResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.Generated || resp.Data.Error != "groq_auth" {
		t.Errorf("resp = %+v, want generated=false error=groq_auth", resp.Data)
	}
}

func TestEnsure_Forbidden_Returns200(t *testing.T) {
	wrapped := fmt.Errorf("ensure-daily: groq: %w", &groq.StatusError{Code: http.StatusForbidden, Body: "forbidden"})
	h := NewDailyHandler(&fakeDaily{ensureErr: wrapped})
	rec := httptest.NewRecorder()
	h.Ensure(rec, httptest.NewRequest(http.MethodPost, "/internal/fanfic/ensure-daily", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestEnsure_TransientError_Returns500(t *testing.T) {
	h := NewDailyHandler(&fakeDaily{ensureErr: errors.New("db write failed")})
	rec := httptest.NewRecorder()
	h.Ensure(rec, httptest.NewRequest(http.MethodPost, "/internal/fanfic/ensure-daily", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a non-groq-auth error", rec.Code)
	}
}
