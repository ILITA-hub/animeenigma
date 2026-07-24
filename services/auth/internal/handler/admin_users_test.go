package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/go-chi/chi/v5"
)

type fakeAdminUsersRepo struct {
	users   []domain.User
	listErr error
}

func (f *fakeAdminUsersRepo) ListUsers(_ context.Context, query, role string, limit, offset int) ([]domain.User, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	var out []domain.User
	for _, u := range f.users {
		if role != "" && string(u.Role) != role {
			continue
		}
		if query != "" && !strings.Contains(u.Username, query) {
			continue
		}
		out = append(out, u)
	}
	total := int64(len(out))
	if offset > len(out) {
		offset = len(out)
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], total, nil
}

func (f *fakeAdminUsersRepo) UpdateRole(_ context.Context, id, role string) error {
	for i := range f.users {
		if f.users[i].ID == id {
			f.users[i].Role = authz.Role(role)
			return nil
		}
	}
	return liberrors.NotFound("user")
}

func (f *fakeAdminUsersRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	for i := range f.users {
		if f.users[i].ID == id {
			u := f.users[i]
			return &u, nil
		}
	}
	return nil, liberrors.NotFound("user")
}

func seedAdminUsers() *fakeAdminUsersRepo {
	return &fakeAdminUsersRepo{users: []domain.User{
		{ID: "11111111-1111-1111-1111-111111111111", Username: "alice", PublicID: "pub-alice", Role: authz.RoleUser},
		{ID: "22222222-2222-2222-2222-222222222222", Username: "bob", PublicID: "pub-bob", Role: authz.RoleAdmin},
	}}
}

func TestAdminUsers_List(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?page=1&page_size=25", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Items    []map[string]any `json:"items"`
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Success || env.Data.Total != 2 || len(env.Data.Items) != 2 || env.Data.Page != 1 || env.Data.PageSize != 25 {
		t.Fatalf("unexpected envelope: %+v", env.Data)
	}
}

func TestAdminUsers_List_InvalidRole(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?role=wizard", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func patchRoleReq(id, body, callerID string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+id+"/role", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = authz.ContextWithClaims(ctx, &authz.Claims{UserID: callerID, Role: authz.RoleAdmin})
	return req.WithContext(ctx)
}

func TestAdminUsers_UpdateRole_Success(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := patchRoleReq("11111111-1111-1111-1111-111111111111", `{"role":"admin"}`, "22222222-2222-2222-2222-222222222222")
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.Role != "admin" {
		t.Fatalf("role=%q want admin", env.Data.Role)
	}
}

func TestAdminUsers_UpdateRole_SelfLockout(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	id := "22222222-2222-2222-2222-222222222222"
	req := patchRoleReq(id, `{"role":"user"}`, id)
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}

func TestAdminUsers_UpdateRole_InvalidRole(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := patchRoleReq("11111111-1111-1111-1111-111111111111", `{"role":"guest"}`, "22222222-2222-2222-2222-222222222222")
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestAdminUsers_UpdateRole_NotFound(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := patchRoleReq("99999999-9999-9999-9999-999999999999", `{"role":"admin"}`, "22222222-2222-2222-2222-222222222222")
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}
