package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type stubMembership struct{}

func (stubMembership) ListVerifyMembership(_ context.Context, _, _ int) ([]repo.VerifyMembershipRow, []repo.VerifyMembershipRow, error) {
	return []repo.VerifyMembershipRow{{ID: "o1", Name: "Frieren", EpisodesAired: 28}},
		[]repo.VerifyMembershipRow{{ID: "t1", Name: "NANA", EpisodesAired: 47}}, nil
}

func TestVerifyMembership(t *testing.T) {
	h := NewInternalVerifyHandler(stubMembership{}, nil)
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
