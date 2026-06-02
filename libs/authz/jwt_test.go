package authz

import (
	"testing"
	"time"
)

// TestGenerateGuestToken verifies the Watch Together guest token path: an
// access-only JWT carrying RoleGuest + the supplied uid/username, with the
// caller-supplied TTL and no session id.
func TestGenerateGuestToken(t *testing.T) {
	mgr := NewJWTManager(JWTConfig{
		Secret:         "guest-test-secret",
		Issuer:         "animeenigma-test",
		AccessTokenTTL: 15 * time.Minute,
	})

	tok, err := mgr.GenerateGuestToken("guest_abc", "Guest-1234", 6*time.Hour)
	if err != nil {
		t.Fatalf("GenerateGuestToken: %v", err)
	}
	if tok == "" {
		t.Fatal("expected non-empty guest token")
	}

	claims, err := mgr.ValidateAccessToken(tok)
	if err != nil {
		t.Fatalf("ValidateAccessToken on guest token: %v", err)
	}
	if claims.Role != RoleGuest {
		t.Errorf("Role = %q, want %q", claims.Role, RoleGuest)
	}
	if claims.UserID != "guest_abc" {
		t.Errorf("UserID = %q, want guest_abc", claims.UserID)
	}
	if claims.Username != "Guest-1234" {
		t.Errorf("Username = %q, want Guest-1234", claims.Username)
	}
	// Guest tokens carry no session id (no DB-backed session).
	if claims.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", claims.SessionID)
	}

	// TTL honored: exp ≈ now + 6h (allow a minute of slack for execution time).
	wantExp := time.Now().Add(6 * time.Hour)
	gotExp := claims.ExpiresAt.Time
	if d := gotExp.Sub(wantExp); d > time.Minute || d < -time.Minute {
		t.Errorf("exp off by %v (got %v, want ~%v)", d, gotExp, wantExp)
	}
}

// TestGuestTokenIsNotAdmin locks the security-critical invariant the gateway +
// admin gates rely on: a guest token validates (valid signature) but never
// reads as the admin role, and IsAdmin is false for it.
func TestGuestTokenIsNotAdmin(t *testing.T) {
	mgr := NewJWTManager(JWTConfig{Secret: "s", Issuer: "i", AccessTokenTTL: time.Minute})

	tok, err := mgr.GenerateGuestToken("guest_1", "Guest-1", time.Hour)
	if err != nil {
		t.Fatalf("GenerateGuestToken: %v", err)
	}
	claims, err := mgr.ValidateAccessToken(tok)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.Role == RoleAdmin {
		t.Error("guest token must not carry the admin role")
	}

	ctx := ContextWithClaims(t.Context(), claims)
	if IsAdmin(ctx) {
		t.Error("IsAdmin must be false for a guest token")
	}
	if RoleFromContext(ctx) != RoleGuest {
		t.Errorf("RoleFromContext = %q, want %q", RoleFromContext(ctx), RoleGuest)
	}
}

// TestGenerateGuestTokenExpiry confirms an expired guest TTL surfaces as
// ErrTokenExpired on validation (so the frontend re-mints rather than looping).
func TestGenerateGuestTokenExpiry(t *testing.T) {
	mgr := NewJWTManager(JWTConfig{Secret: "s", Issuer: "i", AccessTokenTTL: time.Minute})

	tok, err := mgr.GenerateGuestToken("guest_x", "Guest-9", -time.Second)
	if err != nil {
		t.Fatalf("GenerateGuestToken: %v", err)
	}
	if _, err := mgr.ValidateAccessToken(tok); err != ErrTokenExpired {
		t.Errorf("err = %v, want ErrTokenExpired", err)
	}
}
