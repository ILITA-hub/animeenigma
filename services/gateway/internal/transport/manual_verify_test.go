package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/golang-jwt/jwt/v5"
)

// TestManual_DumpAPIKeyMintedJWT is a verification-only helper that prints
// the full minted JWT and its decoded claims to stderr (via t.Log) so we
// can eyeball the "sid" claim end-to-end. Acceptance criterion for WV3-T1.
//
// Marked as a regular test (not skipped) so it runs in CI alongside the
// rest and provides a permanent demonstration record.
func TestManual_DumpAPIKeyMintedJWT(t *testing.T) {
	rawAPIKey := "ak_manualverify_token_0123456789abcdef"
	userID := "manual-verify-user"
	fas := newFakeAuthService(t, userID, "verify-user", authz.RoleUser)
	defer fas.Close()

	var mintedAuth string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mintedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	handler := JWTValidationMiddleware(gatewayTestJWTConfig(), fas.URL())(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	minted := strings.TrimPrefix(mintedAuth, "Bearer ")
	t.Logf("MINTED JWT: %s", minted)

	// Decode without signature verification — same as a downstream service
	// would do for inspection only.
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	parsed, _, err := parser.ParseUnverified(minted, &authz.Claims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	out, _ := json.MarshalIndent(parsed.Claims, "", "  ")
	t.Logf("DECODED CLAIMS:\n%s", out)

	c := parsed.Claims.(*authz.Claims)
	if c.SessionID == "" {
		t.Fatal("DECODED CLAIMS lacked sid — WV3-T1 fix regressed")
	}
	t.Logf("sid present and non-empty: %q", c.SessionID)
}
