package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
)

// fakeIdP is a minimal OIDC provider: discovery + JWKS + token endpoint.
// It signs id_tokens with its own RSA key and echoes the configured claims.
type fakeIdP struct {
	srv       *httptest.Server
	key       *rsa.PrivateKey
	clientID  string
	sub       string
	name      string
	username  string
	signWrong bool // sign the id_token with a key NOT in the JWKS
}

func newFakeIdP(t *testing.T, clientID string) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &fakeIdP{key: key, clientID: clientID, sub: "777", name: "Tester", username: "tneymik"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/auth",
			"token_endpoint":                        f.srv.URL + "/token",
			"jwks_uri":                              f.srv.URL + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := f.key.Public().(*rsa.PublicKey)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "test",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		// PKCE: the exchange must carry the code_verifier.
		require.NotEmpty(t, r.Form.Get("code_verifier"), "token request missing PKCE code_verifier")
		signKey := f.key
		if f.signWrong {
			wrong, err := rsa.GenerateKey(rand.Reader, 2048)
			require.NoError(t, err)
			signKey = wrong
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": f.srv.URL, "aud": f.clientID, "sub": f.sub,
			"name": f.name, "preferred_username": f.username,
			"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		})
		tok.Header["kid"] = "test"
		signed, err := tok.SignedString(signKey)
		require.NoError(t, err)
		// golang.org/x/oauth2 only parses the token response as JSON when
		// Content-Type says so; without it, Go's ResponseWriter sniffs a
		// JSON body starting with '{' as text/plain and the client falls
		// back to (empty) form decoding.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": signed,
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newTestOIDC(t *testing.T, idp *fakeIdP) *TelegramOIDC {
	t.Helper()
	return NewTelegramOIDC(config.TelegramOIDCConfig{
		ClientID:     idp.clientID,
		ClientSecret: "secret",
		RedirectURL:  "https://animeenigma.org/api/auth/telegram/oidc/callback",
		IssuerURL:    idp.srv.URL,
	}, newFakeCache(), logger.Default())
}

// beginAndExtractState runs Begin and pulls the state param out of the auth URL.
func beginAndExtractState(t *testing.T, o *TelegramOIDC, returnPath string) (authURL *url.URL, state string) {
	t.Helper()
	raw, err := o.Begin(context.Background(), returnPath)
	require.NoError(t, err)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u, u.Query().Get("state")
}

func TestTelegramOIDC_BeginBuildsAuthURL(t *testing.T) {
	idp := newFakeIdP(t, "12345")
	o := newTestOIDC(t, idp)

	u, state := beginAndExtractState(t, o, "/anime/xyz")

	require.Equal(t, idp.srv.URL+"/auth", u.Scheme+"://"+u.Host+u.Path)
	q := u.Query()
	require.Equal(t, "12345", q.Get("client_id"))
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(t, q.Get("code_challenge"))
	require.NotEmpty(t, state)
	require.Contains(t, q.Get("scope"), "openid")
	require.Contains(t, q.Get("scope"), "profile")
	require.Contains(t, q.Get("scope"), "telegram:bot_access")
	require.NotContains(t, q.Get("scope"), "phone")
	require.Equal(t, "https://animeenigma.org/api/auth/telegram/oidc/callback", q.Get("redirect_uri"))
}

func TestTelegramOIDC_CompleteHappyPath(t *testing.T) {
	idp := newFakeIdP(t, "12345")
	o := newTestOIDC(t, idp)
	_, state := beginAndExtractState(t, o, "/anime/xyz")

	tgUser, returnPath, err := o.Complete(context.Background(), state, "any-code")
	require.NoError(t, err)
	require.Equal(t, int64(777), tgUser.ID)
	require.Equal(t, "Tester", tgUser.FirstName)
	require.Equal(t, "tneymik", tgUser.Username)
	require.Equal(t, "/anime/xyz", returnPath)
}

func TestTelegramOIDC_CompleteUnknownState(t *testing.T) {
	idp := newFakeIdP(t, "12345")
	o := newTestOIDC(t, idp)

	_, _, err := o.Complete(context.Background(), "nope", "code")
	require.ErrorIs(t, err, ErrOIDCStateExpired)
}

func TestTelegramOIDC_CompleteStateReplay(t *testing.T) {
	idp := newFakeIdP(t, "12345")
	o := newTestOIDC(t, idp)
	_, state := beginAndExtractState(t, o, "/")

	_, _, err := o.Complete(context.Background(), state, "code")
	require.NoError(t, err)
	_, _, err = o.Complete(context.Background(), state, "code")
	require.ErrorIs(t, err, ErrOIDCStateExpired, "state must be single-use")
}

func TestTelegramOIDC_CompleteBadSignature(t *testing.T) {
	idp := newFakeIdP(t, "12345")
	idp.signWrong = true
	o := newTestOIDC(t, idp)
	_, state := beginAndExtractState(t, o, "/")

	_, _, err := o.Complete(context.Background(), state, "code")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrOIDCStateExpired))
	require.Contains(t, err.Error(), "verify id_token")
}

func TestTelegramOIDC_CompleteNonNumericSub(t *testing.T) {
	idp := newFakeIdP(t, "12345")
	idp.sub = "not-a-number"
	o := newTestOIDC(t, idp)
	_, state := beginAndExtractState(t, o, "/")

	_, _, err := o.Complete(context.Background(), state, "code")
	require.Error(t, err)
	require.Contains(t, err.Error(), "sub claim")
}

func TestTelegramOIDC_BeginNotConfigured(t *testing.T) {
	o := NewTelegramOIDC(config.TelegramOIDCConfig{}, newFakeCache(), logger.Default())
	_, err := o.Begin(context.Background(), "/")
	require.Error(t, err)
}

// Interface guard so strings stay in sync with the fake IdP.
var _ = strings.Contains
