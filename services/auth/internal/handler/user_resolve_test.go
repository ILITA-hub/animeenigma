package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"go.uber.org/zap"
)

// fakeUserResolveRepo is a handwritten in-memory fake implementing the four
// getters userResolveRepo needs — no testify/mock, matches project convention.
// failQ, when non-empty, makes every getter return a non-notfound error for
// that query (simulating a real backend failure, e.g. DB outage).
type fakeUserResolveRepo struct {
	users []domain.User
	failQ string
}

func newFakeUserResolveRepo() *fakeUserResolveRepo {
	tgID := int64(100200300)
	return &fakeUserResolveRepo{
		users: []domain.User{
			{
				ID:         "11111111-1111-1111-1111-111111111111",
				Username:   "oronemu",
				PublicID:   "orovanity",
				TelegramID: &tgID,
			},
		},
	}
}

func (f *fakeUserResolveRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if f.failQ != "" && id == f.failQ {
		return nil, fmt.Errorf("db down")
	}
	for i := range f.users {
		if f.users[i].ID == id {
			return &f.users[i], nil
		}
	}
	return nil, liberrors.NotFound("user")
}

func (f *fakeUserResolveRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	if f.failQ != "" && username == f.failQ {
		return nil, fmt.Errorf("db down")
	}
	for i := range f.users {
		if f.users[i].Username == username {
			return &f.users[i], nil
		}
	}
	return nil, liberrors.NotFound("user")
}

func (f *fakeUserResolveRepo) GetByPublicID(ctx context.Context, publicID string) (*domain.User, error) {
	if f.failQ != "" && publicID == f.failQ {
		return nil, fmt.Errorf("db down")
	}
	for i := range f.users {
		if f.users[i].PublicID == publicID {
			return &f.users[i], nil
		}
	}
	return nil, liberrors.NotFound("user")
}

func (f *fakeUserResolveRepo) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	if f.failQ != "" && strconv.FormatInt(telegramID, 10) == f.failQ {
		return nil, fmt.Errorf("db down")
	}
	for i := range f.users {
		if f.users[i].TelegramID != nil && *f.users[i].TelegramID == telegramID {
			return &f.users[i], nil
		}
	}
	// Mirrors repo.UserRepository.GetByTelegramID: not-found is nil,nil.
	return nil, nil
}

// testLogger returns a no-op *logger.Logger so tests don't pollute output.
// Pattern mirrors services/catalog/internal/handler/spotlight_test.go.
func testLogger() *logger.Logger {
	return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name       string
		q          string
		wantID     string
		wantStatus int
		repoFailQ  string // when set, the fake repo returns a non-notfound error for this q
	}{
		{"by uuid", "11111111-1111-1111-1111-111111111111", "11111111-1111-1111-1111-111111111111", 200, ""},
		{"by username", "oronemu", "11111111-1111-1111-1111-111111111111", 200, ""},
		{"by public_id", "orovanity", "11111111-1111-1111-1111-111111111111", 200, ""},
		{"by telegram_id", "100200300", "11111111-1111-1111-1111-111111111111", 200, ""},
		{"not found", "ghost", "", 404, ""},
		{"empty q", "", "", 400, ""},
		{"whitespace q", "   ", "", 400, ""},
		// A real repo failure (e.g. DB outage) must surface as 500, never as
		// a misleading 404 — see the IMPORTANT finding this covers.
		{"repo error surfaces as 500 not 404", "dbdown", "", 500, "dbdown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newFakeUserResolveRepo()
			repo.failQ = c.repoFailQ
			h := NewUserResolveHandler(repo, testLogger())
			req := httptest.NewRequest(http.MethodGet, "/api/admin/users/resolve?q="+url.QueryEscape(c.q), nil)
			rec := httptest.NewRecorder()
			h.Resolve(rec, req)
			if rec.Code != c.wantStatus {
				t.Fatalf("status=%d want %d", rec.Code, c.wantStatus)
			}
			if c.wantStatus == 200 {
				var env struct {
					Success bool `json:"success"`
					Data    struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
					t.Fatalf("decode body: %v (body=%s)", err, rec.Body.String())
				}
				if !env.Success {
					t.Fatalf("success=false, want true")
				}
				if env.Data.ID != c.wantID {
					t.Fatalf("id=%q want %q", env.Data.ID, c.wantID)
				}
			}
		})
	}
}
