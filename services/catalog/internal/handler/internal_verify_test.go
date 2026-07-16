package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// stubMembership fakes verifyMembershipSource. It records the limits it was
// called with so tests can assert on clamping, and returns whatever
// ongoing/top/err are configured on it.
type stubMembership struct {
	ongoing []repo.VerifyMembershipRow
	top     []repo.VerifyMembershipRow
	err     error

	gotOngoingLimit int
	gotTopLimit     int
}

func (s *stubMembership) ListVerifyMembership(_ context.Context, ongoingLimit, topLimit int) ([]repo.VerifyMembershipRow, []repo.VerifyMembershipRow, error) {
	s.gotOngoingLimit = ongoingLimit
	s.gotTopLimit = topLimit
	return s.ongoing, s.top, s.err
}

func TestVerifyMembership(t *testing.T) {
	s := &stubMembership{
		ongoing: []repo.VerifyMembershipRow{{ID: "o1", Name: "Frieren", EpisodesAired: 28}},
		top:     []repo.VerifyMembershipRow{{ID: "t1", Name: "NANA", EpisodesAired: 47}},
	}
	h := NewInternalVerifyHandler(s, nil)
	rec := httptest.NewRecorder()
	h.Membership(rec, httptest.NewRequest("GET", "/internal/verify/membership", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var env struct {
		Data struct {
			Ongoing []repo.VerifyMembershipRow `json:"ongoing"`
			Top     []repo.VerifyMembershipRow `json:"top"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Ongoing) != 1 || env.Data.Ongoing[0].EpisodesAired != 28 || len(env.Data.Top) != 1 {
		t.Fatalf("body: %s", rec.Body.String())
	}
}

// TestVerifyMembership_LimitClamping asserts queryInt clamps out-of-range
// values to the nearer bound instead of discarding them to the default —
// ongoing_limit=5000 (max 2000) must reach the repo as 2000, and
// top_limit=999 (max 500) must reach it as 500.
func TestVerifyMembership_LimitClamping(t *testing.T) {
	s := &stubMembership{}
	h := NewInternalVerifyHandler(s, nil)
	rec := httptest.NewRecorder()
	h.Membership(rec, httptest.NewRequest("GET", "/internal/verify/membership?ongoing_limit=5000&top_limit=999", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if s.gotOngoingLimit != 2000 {
		t.Fatalf("ongoing_limit not clamped: got %d, want 2000", s.gotOngoingLimit)
	}
	if s.gotTopLimit != 500 {
		t.Fatalf("top_limit not clamped: got %d, want 500", s.gotTopLimit)
	}
}

// TestVerifyMembership_RepoError asserts a repo failure surfaces as 500.
func TestVerifyMembership_RepoError(t *testing.T) {
	s := &stubMembership{err: errors.New("db exploded")}
	h := NewInternalVerifyHandler(s, nil)
	rec := httptest.NewRecorder()
	h.Membership(rec, httptest.NewRequest("GET", "/internal/verify/membership", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// TestVerifyMembership_NilSlicesNormalized asserts nil ongoing/top slices
// from the repo are normalized to JSON [] rather than null.
func TestVerifyMembership_NilSlicesNormalized(t *testing.T) {
	s := &stubMembership{} // ongoing, top left nil
	h := NewInternalVerifyHandler(s, nil)
	rec := httptest.NewRecorder()
	h.Membership(rec, httptest.NewRequest("GET", "/internal/verify/membership", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ongoing":[]`) || !strings.Contains(body, `"top":[]`) {
		t.Fatalf("expected empty arrays, not null, in body: %s", body)
	}
}
