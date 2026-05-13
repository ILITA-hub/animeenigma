package handler

// Wave-0 scaffold for SOCIAL-04a (happy path), SOCIAL-04b (empty body),
// SOCIAL-04c (non-owner PATCH). Mapped to `01-VALIDATION.md` rows
// 01-Comment-01 / 01-Comment-02 / 01-Comment-03. Plan 03 fills bodies.
//
// `withCommentClaims` mirrors the claims-into-context injection used by
// mal_import_test.go:140 and sync_test.go (`authz.ContextWithClaims`). It's
// declared but unused today; plan 03 will use it to drive the handler tests.

import (
	"net/http"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// withCommentClaims attaches a *authz.Claims to the request context. Local to
// this file so plan 03 has a single place to extend (e.g. add Role for the
// admin-override test).
func withCommentClaims(r *http.Request, userID, username string, role authz.Role) *http.Request {
	claims := &authz.Claims{UserID: userID, Username: username, Role: role}
	return r.WithContext(authz.ContextWithClaims(r.Context(), claims))
}

// TestCommentHandler_CreateComment_HappyPath validates SOCIAL-04a: a logged-in
// POST with a valid body returns 201 + the created comment JSON.
func TestCommentHandler_CreateComment_HappyPath(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
	_ = withCommentClaims
}

// TestCommentHandler_CreateComment_EmptyBody validates SOCIAL-04b: empty or
// whitespace-only bodies return 400.
func TestCommentHandler_CreateComment_EmptyBody(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
	_ = withCommentClaims
}

// TestCommentHandler_UpdateComment_NotOwner validates SOCIAL-04c: PATCH from a
// non-owner non-admin user returns 403; admin override still succeeds.
func TestCommentHandler_UpdateComment_NotOwner(t *testing.T) {
	t.Skip("Wave 0 scaffold — implementation in plan 03")
	_ = withCommentClaims
}
