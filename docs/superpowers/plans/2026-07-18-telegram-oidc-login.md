# Telegram OIDC Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the QR + deep-link Telegram login with Telegram's 2026 OIDC flow (auth-code + PKCE against `oauth.telegram.org`), per `docs/superpowers/specs/2026-07-17-telegram-oidc-login-design.md`.

**Architecture:** The auth service owns the whole protocol: a `start` endpoint 302s the browser to Telegram with PKCE + state (state stored in Redis), a `callback` endpoint exchanges the code server-side, verifies the RS256 id_token via JWKS, and feeds the claims into the existing `LoginWithTelegram` + cookie-session machinery. The gateway forwards both endpoints with its no-redirect proxy (magic-link precedent). The frontend is one styled button. The deep-link/webhook flow is deleted in the same change.

**Tech Stack:** Go (chi, `coreos/go-oidc/v3`, `golang.org/x/oauth2`), Redis state, Vue 3 SPA.

**Phase 0 status (verified live 2026-07-17/18):** discovery doc at `https://oauth.telegram.org/.well-known/openid-configuration` confirms: authorize `https://oauth.telegram.org/auth`, token `https://oauth.telegram.org/token`, JWKS `https://oauth.telegram.org/.well-known/jwks.json`, `code_challenge_methods_supported: [plain, S256]`, `token_endpoint_auth_methods_supported: [client_secret_basic, client_secret_post]`, scopes `openid phone profile telegram:bot_access`, claims `sub name preferred_username picture phone_number`. Client credentials: BotFather → **Bot Settings → Web Login** → register Allowed URLs (site origin + exact redirect URI) → BotFather displays **Client ID (the bot's numeric ID) + Client Secret**. Token endpoint accepts Basic auth `base64(client_id:client_secret)` (what `golang.org/x/oauth2` sends by default).

## Global Constraints

- **SESSION-PRESERVATION INVARIANT (owner-mandated): existing logged-in users must NOT be logged out.** Never touch: `libs/authz/` (JWT), `services/auth/internal/handler/sessions.go`, `services/auth/internal/repo/session*.go`, the `refreshTokenCookieName`/`accessTokenCookieName` constants, `setRefreshTokenCookie`/`setAccessTokenCookie`/`clearRefreshTokenCookie` bodies, `RefreshToken`/`Logout` handlers, `JWT_SECRET`/cookie env vars, or `domain.User`/`domain.UserSession` structs. Task 7 verifies this by diff.
- **GOLDEN RULE:** all work in the worktree `/data/animeenigma/.claude/worktrees/tg-oidc-login` (branch `feat/tg-oidc-login`), never the base tree. Commit with pathspecs (never bare `git commit -a`), never amend. Every commit ends with the three co-author trailers used by this repo (`Claude Code <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`, `NANDIorg <super.egor.mamonov@yandex.ru>`).
- Scopes are exactly `openid profile telegram:bot_access` — **never add `phone`** (privacy stance).
- Keep `domain.TelegramWebhookUser` type name (used by kept `LoginWithTelegram`); only its doc comment changes. Do not rename it.
- No `go work sync` — ever. Add deps with `go get` + `go mod tidy` inside `services/auth/` only.
- Frontend: bun/bunx only (never npm/npx); all new i18n keys land in **en, ru, and ja**; new UI must pass the DS lint (reuse the existing lint-green Telegram-blue button classes verbatim).
- Deploy is gated on owner-minted BotFather credentials — build/test everything, but do NOT run `/animeenigma-after-update` until the owner confirms `TELEGRAM_OIDC_CLIENT_ID`/`SECRET` are in `docker/.env` (Task 7).

---

### Task 1: `TelegramOIDC` service (state + exchange + verify) with fake-IdP tests

**Files:**
- Modify: `services/auth/internal/config/config.go` (add `TelegramOIDCConfig`; do NOT remove old fields yet — Task 4 does)
- Create: `services/auth/internal/service/telegram_oidc.go`
- Create: `services/auth/internal/service/telegram_oidc_test.go`
- Modify: `services/auth/go.mod` (via `go get`)

**Interfaces:**
- Consumes: `cache.Cache` (`libs/cache`), `domain.TelegramWebhookUser` (exists), `newFakeCache()` from `magiclink_test.go` (same package).
- Produces (Task 2 relies on these exact signatures):
  - `service.NewTelegramOIDC(cfg config.TelegramOIDCConfig, c cache.Cache, log *logger.Logger) *TelegramOIDC`
  - `(*TelegramOIDC).Begin(ctx context.Context, returnPath string) (string, error)` — returns the authorization URL
  - `(*TelegramOIDC).Complete(ctx context.Context, state, code string) (*domain.TelegramWebhookUser, string, error)` — returns claims-mapped user + stored return path
  - `service.ErrOIDCStateExpired` (std `errors.New` sentinel, matched with `errors.Is`)
  - `config.TelegramOIDCConfig{ClientID, ClientSecret, RedirectURL, IssuerURL string}` on `Config.TelegramOIDC`

- [ ] **Step 1: Add config struct**

In `services/auth/internal/config/config.go`, below `TelegramConfig`:

```go
// TelegramOIDCConfig configures the OIDC login against oauth.telegram.org.
// ClientID/ClientSecret come from BotFather (Bot Settings > Web Login).
// IssuerURL is overridable only so tests can point at a fake IdP.
type TelegramOIDCConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	IssuerURL    string
}
```

Add field `TelegramOIDC TelegramOIDCConfig` to `Config` (next to `Telegram`), and in `Load()` next to the `Telegram:` entry:

```go
		TelegramOIDC: TelegramOIDCConfig{
			ClientID:     getEnv("TELEGRAM_OIDC_CLIENT_ID", ""),
			ClientSecret: getEnv("TELEGRAM_OIDC_CLIENT_SECRET", ""),
			RedirectURL:  getEnv("TELEGRAM_OIDC_REDIRECT_URL", "https://animeenigma.org/api/auth/telegram/oidc/callback"),
			IssuerURL:    getEnv("TELEGRAM_OIDC_ISSUER", "https://oauth.telegram.org"),
		},
```

- [ ] **Step 2: Add dependency**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/services/auth
go get github.com/coreos/go-oidc/v3@latest && go mod tidy
```
Expected: `go.mod` gains `github.com/coreos/go-oidc/v3` (+ `go-jose` indirect); `golang.org/x/oauth2` appears. Build still green: `go build ./...`.

- [ ] **Step 3: Write the failing tests**

Create `services/auth/internal/service/telegram_oidc_test.go` (package `service` — reuses `newFakeCache()` from `magiclink_test.go`). Full content:

```go
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
```

Note: `github.com/golang-jwt/jwt/v5` is already an indirect dep (via `libs/authz`) — `go mod tidy` in Step 5 promotes it to direct.

- [ ] **Step 4: Run tests to verify they fail**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/services/auth
go test ./internal/service/ -run TestTelegramOIDC -v 2>&1 | head -20
```
Expected: compile FAIL — `undefined: NewTelegramOIDC`, `undefined: ErrOIDCStateExpired`.

- [ ] **Step 5: Write the implementation**

Create `services/auth/internal/service/telegram_oidc.go`:

```go
package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/config"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

const (
	// oidcStateKeyPrefix prefixes the Redis key holding one login attempt's
	// PKCE verifier + post-login return path. The TTL bounds the window
	// between the redirect to Telegram and the callback.
	oidcStateKeyPrefix = "auth:tgoidc:"
	oidcStateTTL       = 5 * time.Minute
)

// ErrOIDCStateExpired distinguishes an expired/replayed/unknown state (user
// retries) from infrastructure failures (logged as errors).
var ErrOIDCStateExpired = stderrors.New("oidc state expired or already used")

// oidcState is the Redis-stored payload for one in-flight login attempt.
type oidcState struct {
	Verifier   string `json:"verifier"`
	ReturnPath string `json:"return_path"`
}

// TelegramOIDC drives the OAuth2 authorization-code + PKCE login against
// Telegram's OIDC provider (oauth.telegram.org). It is pure protocol — no DB
// access; the handler composes Complete's claims with
// AuthService.LoginWithTelegram, which owns find-or-create + sessions.
type TelegramOIDC struct {
	cfg   config.TelegramOIDCConfig
	cache cache.Cache
	log   *logger.Logger

	// Discovery is lazy so an unreachable oauth.telegram.org delays logins,
	// not service boot. mu guards first initialization; after that the
	// fields are read-only.
	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func NewTelegramOIDC(cfg config.TelegramOIDCConfig, c cache.Cache, log *logger.Logger) *TelegramOIDC {
	return &TelegramOIDC{cfg: cfg, cache: c, log: log}
}

func (t *TelegramOIDC) ensureProvider(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.provider != nil {
		return nil
	}
	p, err := oidc.NewProvider(ctx, t.cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	t.provider = p
	t.oauth = &oauth2.Config{
		ClientID:     t.cfg.ClientID,
		ClientSecret: t.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  t.cfg.RedirectURL,
		// telegram:bot_access lets the auth bot message the user later
		// (notifications). phone is deliberately never requested.
		Scopes: []string{oidc.ScopeOpenID, "profile", "telegram:bot_access"},
	}
	t.verifier = p.Verifier(&oidc.Config{ClientID: t.cfg.ClientID})
	return nil
}

// Begin creates the state + PKCE verifier for one login attempt and returns
// the authorization URL to redirect the browser to.
func (t *TelegramOIDC) Begin(ctx context.Context, returnPath string) (string, error) {
	if t.cfg.ClientID == "" || t.cfg.ClientSecret == "" {
		return "", fmt.Errorf("telegram oidc not configured (TELEGRAM_OIDC_CLIENT_ID/SECRET)")
	}
	if err := t.ensureProvider(ctx); err != nil {
		return "", err
	}
	state := uuid.New().String()
	verifier := oauth2.GenerateVerifier()
	if err := t.cache.Set(ctx, oidcStateKeyPrefix+state, &oidcState{Verifier: verifier, ReturnPath: returnPath}, oidcStateTTL); err != nil {
		return "", fmt.Errorf("store oidc state: %w", err)
	}
	return t.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
}

// Complete consumes the state (single-use), exchanges the code, verifies the
// id_token, and maps its claims onto the TelegramWebhookUser shape consumed
// by AuthService.LoginWithTelegram. Also returns the stored return path.
func (t *TelegramOIDC) Complete(ctx context.Context, state, code string) (*domain.TelegramWebhookUser, string, error) {
	var st oidcState
	if err := t.cache.Get(ctx, oidcStateKeyPrefix+state, &st); err != nil {
		return nil, "", ErrOIDCStateExpired
	}
	// Single-use: delete before the exchange so a replayed callback cannot
	// race a second exchange with the same verifier.
	_ = t.cache.Delete(ctx, oidcStateKeyPrefix+state)

	if err := t.ensureProvider(ctx); err != nil {
		return nil, "", err
	}
	tok, err := t.oauth.Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return nil, "", fmt.Errorf("code exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return nil, "", fmt.Errorf("token response missing id_token")
	}
	idt, err := t.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, "", fmt.Errorf("verify id_token: %w", err)
	}
	var claims struct {
		Sub               string `json:"sub"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idt.Claims(&claims); err != nil {
		return nil, "", fmt.Errorf("parse claims: %w", err)
	}
	tgID, err := strconv.ParseInt(claims.Sub, 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("non-numeric sub claim %q", claims.Sub)
	}
	return &domain.TelegramWebhookUser{
		ID:        tgID,
		FirstName: claims.Name,
		Username:  claims.PreferredUsername,
	}, st.ReturnPath, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/services/auth
go test ./internal/service/ -run TestTelegramOIDC -v
```
Expected: all 7 tests PASS. Then the full package: `go test ./...` → PASS (integration-tagged tests skipped by default).

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login
git add services/auth/internal/service/telegram_oidc.go services/auth/internal/service/telegram_oidc_test.go services/auth/internal/config/config.go services/auth/go.mod services/auth/go.sum
git commit services/auth -m "feat(auth): TelegramOIDC service — PKCE state, code exchange, id_token verify"
```
(append the three co-author trailers)

---

### Task 2: Auth-service handler + routes + wiring

**Files:**
- Create: `services/auth/internal/handler/telegram_oidc.go`
- Create: `services/auth/internal/handler/telegram_oidc_test.go`
- Modify: `services/auth/internal/transport/router.go` (add param + 2 routes)
- Modify: `services/auth/cmd/auth-api/main.go` (construct + pass)

**Interfaces:**
- Consumes: `service.NewTelegramOIDC`/`Begin`/`Complete`/`ErrOIDCStateExpired` (Task 1), `service.SanitizeOldURL(raw string) string` (exists, `magiclink.go:18` — returns "/" unless the input is a safe same-origin relative path), `cookieSetter` interface (exists, `magiclink.go` — satisfied by `*AuthHandler`), `sessionContextFromReq` (exists, `auth.go`), `service.AuthService.LoginWithTelegram(ctx, *domain.TelegramWebhookUser, SessionContext) (*domain.AuthResponse, error)` (exists).
- Produces: `handler.NewTelegramOIDCHandler(o *service.TelegramOIDC, a *service.AuthService, cookie cookieSetter, log *logger.Logger) *TelegramOIDCHandler` with `Start`/`Callback` `http.HandlerFunc` methods; routes `GET /api/auth/telegram/oidc/start` and `GET /api/auth/telegram/oidc/callback` (Task 3 and Task 5 rely on these exact paths).

- [ ] **Step 1: Write the failing handler test**

Create `services/auth/internal/handler/telegram_oidc_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Callback with a provider ?error= must bounce to /auth?error=denied without
// touching any service (nil deps prove no dereference happens on this path).
func TestTelegramOIDCCallback_ProviderError(t *testing.T) {
	h := NewTelegramOIDCHandler(nil, nil, nil, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/telegram/oidc/callback?error=access_denied", nil)
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth?error=denied", rec.Header().Get("Location"))
}

// Callback without state/code is a malformed hit — generic telegram error.
func TestTelegramOIDCCallback_MissingParams(t *testing.T) {
	h := NewTelegramOIDCHandler(nil, nil, nil, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/telegram/oidc/callback", nil)
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth?error=telegram", rec.Header().Get("Location"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/services/auth
go test ./internal/handler/ -run TestTelegramOIDCCallback -v 2>&1 | head -5
```
Expected: compile FAIL — `undefined: NewTelegramOIDCHandler`.

- [ ] **Step 3: Write the handler**

Create `services/auth/internal/handler/telegram_oidc.go`:

```go
package handler

import (
	stderrors "errors"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// TelegramOIDCHandler serves the browser-facing Telegram OIDC login
// endpoints. Both answer with 302s (never JSON): the browser is mid full-page
// navigation, and the gateway forwards these routes with its no-redirect
// proxy so every Location header reaches the browser verbatim.
type TelegramOIDCHandler struct {
	oidc        *service.TelegramOIDC
	authService *service.AuthService
	cookie      cookieSetter
	log         *logger.Logger
}

// NewTelegramOIDCHandler constructs the handler. Pass the *AuthHandler as
// cookieSetter (same package — unexported methods satisfy the interface).
func NewTelegramOIDCHandler(o *service.TelegramOIDC, a *service.AuthService, cookie cookieSetter, log *logger.Logger) *TelegramOIDCHandler {
	return &TelegramOIDCHandler{oidc: o, authService: a, cookie: cookie, log: log}
}

// Start begins a login attempt: 302 to Telegram's authorization endpoint.
// ?return= is the SPA path to land on after login; it travels server-side in
// the OIDC state, sanitized exactly like the magic-link oldurl.
func (h *TelegramOIDCHandler) Start(w http.ResponseWriter, r *http.Request) {
	returnPath := service.SanitizeOldURL(r.URL.Query().Get("return"))
	authURL, err := h.oidc.Begin(r.Context(), returnPath)
	if err != nil {
		h.log.Errorw("telegram oidc begin failed", "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "begin_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback finishes the login: exchanges the code, mints the session via the
// existing LoginWithTelegram, sets the standard auth cookies, and lands the
// user on their return path. Every failure lands on /auth?error=… (retryable).
func (h *TelegramOIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		// User cancelled on Telegram's consent screen (or provider error).
		h.log.Infow("telegram oidc denied", "provider_error", e)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "denied").Inc()
		http.Redirect(w, r, "/auth?error=denied", http.StatusFound)
		return
	}
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}

	tgUser, returnPath, err := h.oidc.Complete(r.Context(), state, code)
	if err != nil {
		if stderrors.Is(err, service.ErrOIDCStateExpired) {
			metrics.AuthEventsTotal.WithLabelValues("telegram_login", "state_expired").Inc()
			http.Redirect(w, r, "/auth?error=expired", http.StatusFound)
			return
		}
		h.log.Errorw("telegram oidc complete failed", "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "exchange_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}

	resp, err := h.authService.LoginWithTelegram(r.Context(), tgUser, sessionContextFromReq(r))
	if err != nil {
		h.log.Errorw("telegram oidc login failed", "telegram_id", tgUser.ID, "error", err)
		metrics.AuthEventsTotal.WithLabelValues("telegram_login", "login_error").Inc()
		http.Redirect(w, r, "/auth?error=telegram", http.StatusFound)
		return
	}

	metrics.AuthEventsTotal.WithLabelValues("telegram_login", "success").Inc()
	h.cookie.setRefreshTokenCookie(w, resp.RefreshToken)
	h.cookie.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)
	http.Redirect(w, r, returnPath, http.StatusFound)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/handler/ -run TestTelegramOIDCCallback -v
```
Expected: 2 PASS.

- [ ] **Step 5: Wire router + main**

`services/auth/internal/transport/router.go` — add `telegramOIDCHandler *handler.TelegramOIDCHandler` as a new parameter (directly after `telegramBotHandler`), and in the public `/auth` route group add below the existing telegram routes:

```go
			// Telegram OIDC login (2026 flow). Browser-facing 302 endpoints —
			// the gateway forwards them redirect-transparently.
			r.Get("/telegram/oidc/start", telegramOIDCHandler.Start)
			r.Get("/telegram/oidc/callback", telegramOIDCHandler.Callback)
```

`services/auth/cmd/auth-api/main.go` — after `authHandler := ...`:

```go
	telegramOIDC := service.NewTelegramOIDC(cfg.TelegramOIDC, redisCache, log)
	telegramOIDCHandler := handler.NewTelegramOIDCHandler(telegramOIDC, authService, authHandler, log)
```
and pass `telegramOIDCHandler` in the `transport.NewRouter(...)` call in the same position as the router signature.

- [ ] **Step 6: Build + full test**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/services/auth && go build ./... && go test ./...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add services/auth/internal/handler/telegram_oidc.go services/auth/internal/handler/telegram_oidc_test.go services/auth/internal/transport/router.go services/auth/cmd/auth-api/main.go
git commit services/auth -m "feat(auth): Telegram OIDC start/callback endpoints wired into router"
```

---

### Task 3: Gateway — forward OIDC routes without chasing redirects

**Files:**
- Modify: `services/gateway/internal/transport/router.go` (2 routes in the `/api` group, above the `/auth/*` wildcard at line ~379)

**Interfaces:**
- Consumes: `proxyHandler.ProxyToAuthNoRedirect` (exists — magic-link precedent, `handler/proxy.go`).
- Produces: browser-reachable `GET /api/auth/telegram/oidc/{start,callback}` whose 302s reach the browser.

- [ ] **Step 1: Add routes**

In `services/gateway/internal/transport/router.go`, inside the `/api` route group directly ABOVE `r.HandleFunc("/auth/*", proxyHandler.ProxyToAuth)`:

```go
		// Telegram OIDC login — the auth service answers with 302s (to
		// oauth.telegram.org and back into the SPA). Like the magic-link
		// bridge, these must be proxied WITHOUT following redirects so the
		// Location + Set-Cookie headers reach the browser verbatim. chi
		// prefers these static routes over the /auth/* wildcard below.
		r.Get("/auth/telegram/oidc/start", proxyHandler.ProxyToAuthNoRedirect)
		r.Get("/auth/telegram/oidc/callback", proxyHandler.ProxyToAuthNoRedirect)
```

- [ ] **Step 2: Build + test**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/services/gateway && go build ./... && go test ./...
```
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add services/gateway/internal/transport/router.go
git commit services/gateway -m "feat(gateway): no-redirect proxy for telegram oidc start/callback"
```

---

### Task 4: Delete the deep-link/webhook flow (backend)

**Files:**
- Delete: `services/auth/internal/handler/telegram_bot.go`
- Modify: `services/auth/internal/transport/router.go` (drop `telegramBotHandler` param + 3 routes)
- Modify: `services/auth/internal/handler/auth.go` (drop `DeepLink`, `CheckDeepLink`, `telegramConfig` field/param)
- Modify: `services/auth/internal/service/auth.go` (drop 4 deep-link funcs + `telegramBotToken` field/param)
- Modify: `services/auth/internal/domain/user.go` (drop `TelegramAuthSession`, `DeepLinkResponse`, `DeepLinkCheckResponse`; re-comment `TelegramWebhookUser`)
- Modify: `services/auth/internal/config/config.go` (drop `BotName`, `WebhookSecret`, `WebhookURL`)
- Modify: `services/auth/cmd/auth-api/main.go` (drop bot handler + `SetWebhook`; add `deleteTelegramWebhook`)
- Modify: `libs/cache/ttl.go` (drop `KeyTelegramAuth` + `TTLTelegramAuth` — auth was the only consumer, verified 2026-07-18)

**Interfaces:**
- Consumes: nothing new.
- Produces: `LoginWithTelegram` and `TelegramWebhookUser` REMAIN (Task 2 uses them). Routes `/telegram/deeplink`, `/telegram/check`, `/telegram/webhook` are GONE (Task 5's frontend must not call them).

- [ ] **Step 1: Remove routes + handler wiring**

`router.go`: delete the `telegramBotHandler *handler.TelegramBotHandler` parameter and these three lines:
```go
			r.Post("/telegram/deeplink", authHandler.DeepLink)
			r.Get("/telegram/check", authHandler.CheckDeepLink)
			r.Post("/telegram/webhook", telegramBotHandler.HandleWebhook)
```

- [ ] **Step 2: Delete the bot webhook handler**

```bash
git rm services/auth/internal/handler/telegram_bot.go
```

- [ ] **Step 3: Trim handler/auth.go**

Delete the `DeepLink` and `CheckDeepLink` methods entirely. Remove the `telegramConfig config.TelegramConfig` struct field and the `telegramConfig config.TelegramConfig` parameter from `NewAuthHandler` (signature becomes `NewAuthHandler(authService *service.AuthService, cookieConfig config.CookieConfig, log *logger.Logger)`). Drop the now-unused `config` import only if nothing else in the file uses it (CookieConfig does — keep).

- [ ] **Step 4: Trim service/auth.go**

Delete `CreateDeepLinkToken`, `CheckDeepLinkToken`, `HandleTelegramStart`, `HandleTelegramCallback` (lines ~318–430). Remove the `telegramBotToken` field and the `telegramBotToken string` parameter from `NewAuthService`. Fix the `cache` import if now unused (it is still used — `s.cache` remains for magic-link/throttle/OIDC state).

- [ ] **Step 5: Trim domain/user.go**

Delete `TelegramAuthSession`, `DeepLinkResponse`, `DeepLinkCheckResponse`. Replace the `TelegramWebhookUser` doc comment with:
```go
// TelegramWebhookUser carries the user identity fields extracted from a
// Telegram login (today: OIDC id_token claims — sub/name/preferred_username).
// The name is historical; it predates the OIDC migration.
```

- [ ] **Step 6: Trim config**

In `config.go`: `TelegramConfig` keeps only `BotToken`; delete `BotName`, `WebhookSecret`, `WebhookURL` fields and their `getEnv` lines in `Load()`.

- [ ] **Step 7: main.go — replace SetWebhook with deleteWebhook**

Delete the `telegramBotHandler := handler.NewTelegramBotHandler(...)` line, its use in `transport.NewRouter(...)`, and the whole `// Register Telegram webhook` block. Update the `NewAuthHandler` call to the 3-arg signature. Add in its place:

```go
	// One-time webhook teardown: the deep-link login flow is gone, so tell
	// Telegram to stop POSTing /start updates at the removed route.
	// Idempotent; warn-only — login no longer depends on the bot.
	if cfg.Telegram.BotToken != "" {
		go deleteTelegramWebhook(cfg.Telegram.BotToken, log)
	}
```

and at file bottom:

```go
// deleteTelegramWebhook tears down the legacy bot-webhook registration.
func deleteTelegramWebhook(botToken string, log *logger.Logger) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("https://api.telegram.org/bot"+botToken+"/deleteWebhook", "application/json", nil)
	if err != nil {
		log.Warnw("telegram deleteWebhook failed", "error", err)
		return
	}
	defer resp.Body.Close()
	log.Infow("telegram webhook deleted", "status", resp.StatusCode)
}
```

- [ ] **Step 8: Trim libs/cache**

In `libs/cache/ttl.go` delete the `TTLTelegramAuth` constant and the `KeyTelegramAuth` function.

- [ ] **Step 9: Verify no leftovers + build + test**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login
grep -rn "DeepLink\|deeplink\|TelegramAuthSession\|HandleTelegramStart\|TelegramBotHandler\|WebhookSecret\|WebhookURL\|BotName\|KeyTelegramAuth\|TTLTelegramAuth" services/ libs/ --include="*.go"
```
Expected: NO matches. Then:
```bash
cd services/auth && go build ./... && go test ./... && cd ../gateway && go build ./...
```
Expected: PASS. (If other services vendor `libs/cache`, also run `go build ./...` in any service that fails CI-style: `grep -rln "libs/cache" services/*/go.mod` and build each.)

- [ ] **Step 10: Commit**

```bash
git add -A services/auth libs/cache
git commit services/auth libs/cache -m "feat(auth)!: delete QR deep-link + bot-webhook login flow (replaced by OIDC)"
```

---

### Task 5: Frontend — single-button Auth.vue, store + i18n + dependency cleanup

**Files:**
- Modify: `frontend/web/src/views/Auth.vue` (full rewrite below)
- Modify: `frontend/web/src/stores/auth.ts` (delete `requestDeepLink` ~line 250 and `checkDeepLink` ~line 262 + their exports ~404-405)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (auth section)
- Modify: `frontend/web/package.json` (drop `qrcode`, `@types/qrcode`) + `bun install`

**Interfaces:**
- Consumes: `GET /api/auth/telegram/oidc/start?return=<path>` (Task 2/3); backend error bounces `/auth?error=expired|denied|telegram`.
- Produces: nothing downstream.

- [ ] **Step 1: Rewrite Auth.vue**

Replace the entire file with:

```vue
<template>
  <div class="min-h-[calc(100vh-var(--header-offset))] flex items-center justify-center px-4 py-12">
    <!-- Background -->
    <div class="fixed inset-0 -z-10">
      <div class="absolute inset-0 bg-gradient-to-br from-base via-surface to-base" />
      <div class="absolute top-1/4 left-1/4 w-96 h-96 bg-cyan-500/10 rounded-full blur-3xl" />
      <div class="absolute bottom-1/4 right-1/4 w-96 h-96 bg-pink-500/10 rounded-full blur-3xl" />
    </div>

    <div class="w-full max-w-md">
      <!-- Logo -->
      <div class="text-center mb-8">
        <router-link to="/" class="inline-flex items-center gap-2 text-2xl font-semibold">
          <span class="text-cyan-400">Anime</span>
          <span class="text-white">Enigma</span>
        </router-link>
      </div>

      <!-- Auth Card -->
      <div class="glass-card p-6 md:p-8">
        <h1 class="sr-only">{{ $t('auth.heading') }}</h1>
        <h2 class="text-center text-white text-lg font-medium mb-6">{{ $t('auth.telegramLogin') }}</h2>

        <!-- Error from the OIDC callback bounce (?error=) -->
        <div
          v-if="errorKey"
          class="mb-4 p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm"
          role="alert"
        >
          {{ $t(errorKey) }}
        </div>

        <!-- The OIDC round-trip is a full-page navigation: the backend 302s
             to Telegram and back, sets the session cookies, then lands on
             `return`. A plain <a> (not XHR) keeps the redirect chain in the
             browser. -->
        <a
          :href="startUrl"
          class="inline-flex items-center gap-2 px-6 py-3 bg-[#54a9eb] hover:bg-[#4a96d2] text-white font-medium rounded-lg transition-colors w-full justify-center"
          @click="onLoginClick"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
          </svg>
          {{ $t('auth.continueWithTelegram') }}
        </a>

        <p class="mt-4 text-center text-white/40 text-xs">{{ $t('auth.oidcHint') }}</p>
      </div>

      <!-- Back to home -->
      <p class="text-center mt-6 text-white/60 text-sm">
        <router-link to="/" class="hover:text-white transition-colors">
          {{ '← ' + $t('auth.backHome') }}
        </router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'

useI18n()

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

// return path travels server-side inside the OIDC state; sessionStorage is
// only the SPA-internal handoff from the router guard to this page.
const startUrl = computed(() => {
  const returnUrl = sessionStorage.getItem('returnUrl') || '/'
  return `/api/auth/telegram/oidc/start?return=${encodeURIComponent(returnUrl)}`
})

// ?error= set by the backend callback on any failure path.
const errorKey = computed(() => {
  switch (route.query.error) {
    case 'expired':
      return 'auth.errorExpired'
    case 'denied':
      return 'auth.errorDenied'
    case 'telegram':
      return 'auth.errorTelegram'
    default:
      return ''
  }
})

function onLoginClick() {
  // The href has already captured the return path; clear the handoff so a
  // later manual /auth visit doesn't bounce to a stale destination.
  sessionStorage.removeItem('returnUrl')
}

onMounted(() => {
  // UA-027: if user is already authed, bounce them out rather than show the button.
  if (authStore.isAuthenticated) {
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.replace(returnUrl || '/')
  }
})
</script>
```

- [ ] **Step 2: Trim stores/auth.ts**

Delete the `requestDeepLink` and `checkDeepLink` functions and their two entries in the returned object. Grep first to confirm no other callers:
```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login/frontend/web
grep -rn "requestDeepLink\|checkDeepLink" src/ | grep -v stores/auth.ts
```
Expected: no matches (Auth.vue was rewritten in Step 1).

- [ ] **Step 3: i18n — en/ru/ja**

In each locale's `auth` object: **keep** `heading`, `telegramLogin`, `backHome`, `loginError`, `registerError`, `loading` (grep `auth.loading` first — remove only if unused). **Delete** `qrAlt`, `telegramLoginError`, `openInTelegram`, `expiresIn`, `sessionExpired`, `tryAgain`, `troubleshootingHint`, `openInBrowser`, `tgWebToggle`, `tgWebInstructions`, `tgWebCopy`, `tgWebCopied` (grep each key name across `src/` before deleting; all usages lived in the old Auth.vue/auth.ts). **Add**:

en:
```json
"continueWithTelegram": "Continue with Telegram",
"oidcHint": "You'll be redirected to Telegram to confirm — one tap, then straight back.",
"errorTelegram": "Telegram login failed. Please try again.",
"errorExpired": "That login attempt expired — try again.",
"errorDenied": "Login was cancelled in Telegram."
```
ru:
```json
"continueWithTelegram": "Продолжить через Telegram",
"oidcHint": "Вас перенаправит в Telegram для подтверждения — один тап, и сразу обратно.",
"errorTelegram": "Не удалось войти через Telegram. Попробуйте ещё раз.",
"errorExpired": "Попытка входа истекла — попробуйте ещё раз.",
"errorDenied": "Вход отменён в Telegram."
```
ja:
```json
"continueWithTelegram": "Telegramで続行",
"oidcHint": "確認のためTelegramにリダイレクトされます。ワンタップで戻ります。",
"errorTelegram": "Telegramログインに失敗しました。もう一度お試しください。",
"errorExpired": "ログイン試行の有効期限が切れました。もう一度お試しください。",
"errorDenied": "Telegramでログインがキャンセルされました。"
```

- [ ] **Step 4: Drop qrcode dependency**

```bash
grep -rn "qrcode" src/  # expected: no matches after Step 1
```
Remove `"qrcode"` and `"@types/qrcode"` lines from `package.json`, then `bun install`.

- [ ] **Step 5: Verify**

Run `/frontend-verify` (DS-lint + i18n parity + `bun run build` + trap checks). Expected: all green. (The DS PostToolUse hook will also have linted each edit.)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/views/Auth.vue frontend/web/src/stores/auth.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/package.json frontend/web/bun.lock
git commit frontend/web -m "feat(web): single-button Telegram OIDC login page; drop QR/deeplink flow + qrcode dep"
```

---

### Task 6: Environment plumbing + docs

**Files:**
- Modify: `docker/docker-compose.yml` (auth service env, lines ~690-693)
- Modify: k8s auth env (discover exact file: `grep -rln "TELEGRAM\|JWT_SECRET" deploy/kustomize/` — follow the secrets model in `docs/k8s-deploy.md`; base stays secretless)
- Modify: `docs/environment-variables.md` (auth section)
- Modify: `docs/HOW-IT-WORKS.md` (auth/login description, if it describes the QR flow: `grep -n -i "qr\|deep.link\|telegram" docs/HOW-IT-WORKS.md`)

**Interfaces:**
- Consumes: env var names from Task 1 (`TELEGRAM_OIDC_CLIENT_ID`, `TELEGRAM_OIDC_CLIENT_SECRET`; `TELEGRAM_OIDC_REDIRECT_URL`/`TELEGRAM_OIDC_ISSUER` have correct prod defaults in code — only document them).
- Produces: deployable config; owner instructions.

- [ ] **Step 1: docker-compose auth env**

Replace lines 690-693 (`TELEGRAM_BOT_TOKEN`/`BOT_NAME`/`WEBHOOK_SECRET`/`WEBHOOK_URL`) with:

```yaml
      TELEGRAM_BOT_TOKEN: ${TELEGRAM_BOT_TOKEN:-}
      TELEGRAM_OIDC_CLIENT_ID: ${TELEGRAM_OIDC_CLIENT_ID:-}
      TELEGRAM_OIDC_CLIENT_SECRET: ${TELEGRAM_OIDC_CLIENT_SECRET:-}
```

- [ ] **Step 2: k8s overlay**

Run the discovery grep; add the same two new keys wherever the auth deployment's secret env lives (per `docs/k8s-deploy.md` secrets model), and remove `TELEGRAM_BOT_NAME`/`TELEGRAM_WEBHOOK_SECRET`/`TELEGRAM_WEBHOOK_URL` if present. If the k8s manifests carry no TELEGRAM vars at all, add the two OIDC keys to the auth secret template only.

- [ ] **Step 3: docs**

`docs/environment-variables.md` — in the auth section add:

```markdown
| `TELEGRAM_OIDC_CLIENT_ID` | — | OIDC Client ID from BotFather (Bot Settings → Web Login). Empty ⇒ Telegram login disabled. |
| `TELEGRAM_OIDC_CLIENT_SECRET` | — | OIDC Client Secret from BotFather. |
| `TELEGRAM_OIDC_REDIRECT_URL` | `https://animeenigma.org/api/auth/telegram/oidc/callback` | Must exactly match a BotFather Allowed URL. |
| `TELEGRAM_OIDC_ISSUER` | `https://oauth.telegram.org` | Test override only. |
```
Remove any rows for `TELEGRAM_BOT_NAME`/`TELEGRAM_WEBHOOK_*` if present. Update `docs/HOW-IT-WORKS.md` login description to one line: login = "Continue with Telegram" → OIDC redirect → session cookies.

- [ ] **Step 4: Commit**

```bash
git add docker/docker-compose.yml deploy/kustomize docs/environment-variables.md docs/HOW-IT-WORKS.md
git commit docker deploy docs -m "chore(auth): env + docs plumbing for Telegram OIDC login"
```

---

### Task 7: Verification, session-invariant audit, land — deploy gated on owner creds

**Files:** none new.

- [ ] **Step 1: Full build + test sweep**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login
(cd services/auth && go build ./... && go test ./...) && (cd services/gateway && go build ./... && go test ./...)
cd frontend/web && bunx tsc --noEmit && bun run build
```
Expected: all green.

- [ ] **Step 2: Session-preservation audit (the owner's hard requirement)**

```bash
cd /data/animeenigma/.claude/worktrees/tg-oidc-login
git diff --name-only origin/main | grep -E "libs/authz|sessions|repo/session" ; echo "exit=$? (want 1 = no matches)"
git diff origin/main -- services/auth/internal/handler/auth.go | grep -E "^[+-].*(refreshTokenCookieName|accessTokenCookieName|setRefreshTokenCookie|setAccessTokenCookie|MaxAge|Path:)" 
```
Expected: first grep matches nothing; second shows NO modified cookie-helper lines (only the deleted `DeepLink`/`CheckDeepLink` method bodies). Also confirm `domain.User`, `UserSession`, `RefreshToken`, `Logout` untouched: `git diff origin/main -- services/auth/internal/domain/user.go` shows only the three struct deletions + comment change.

- [ ] **Step 3: Rebase + push the branch's commits to main**

```bash
for i in 1 2 3; do git pull --rebase -q origin main && git push -q origin HEAD:main && break; echo "push race — retry $i"; sleep 2; done
```

- [ ] **Step 4: STOP — owner gate**

Report to the owner: code is landed but **inert until credentials exist** (`Begin` refuses when unconfigured → `/auth?error=telegram`). Owner steps (exact):
1. Open @BotFather → select the auth bot → **Bot Settings → Web Login**.
2. Add Allowed URLs: `https://animeenigma.org` and `https://animeenigma.org/api/auth/telegram/oidc/callback`.
3. Copy the displayed **Client ID** and **Client Secret** into `/data/animeenigma/docker/.env` as `TELEGRAM_OIDC_CLIENT_ID=` / `TELEGRAM_OIDC_CLIENT_SECRET=` (base tree `.env` edit is the allowed exception to the golden rule).

- [ ] **Step 5: After owner confirms — deploy + after-update**

Deploy order matters: `make redeploy-gateway && make redeploy-auth` BEFORE `make redeploy-web` (new FE requires new routes). Env vars are NEW ⇒ compose recreates containers with them (redeploy = build+up). Then run `/animeenigma-after-update` from the worktree (simplify pass, health checks, Trump-mode changelog, final commit/push), then verify live:
```bash
curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" "https://animeenigma.org/api/auth/telegram/oidc/start?return=/"
```
Expected: `302 https://oauth.telegram.org/auth?...` (and a real end-to-end login test by the owner). Existing-session spot-check: an already-logged-in browser must still be logged in after the deploy.

---

## Self-review notes (done at write time)

- Spec coverage: config ✓ endpoints ✓ gateway ✓ deletion ✓ FE ✓ env/docs ✓ tests ✓ rollout+owner gate ✓; Phase 0 was executed live before planning (results in header).
- Type consistency: `TelegramWebhookUser` deliberately NOT renamed; `Begin/Complete/ErrOIDCStateExpired` signatures identical in Tasks 1 and 2; route paths identical in Tasks 2, 3, 5.
- Session invariant: no task touches authz/sessions/cookie internals; Task 7 Step 2 proves it by diff.
