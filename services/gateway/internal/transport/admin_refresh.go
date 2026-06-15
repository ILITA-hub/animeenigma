// admin_refresh.go — transparent access-token refresh for browser-driven admin
// tools (Grafana, Prometheus) served under /admin/*.
//
// Why this exists:
//
// The /admin group is gated by JWTValidationMiddleware, which authenticates
// browser navigation via the short-lived `access_token` cookie (~1h JWT;
// see libs/httputil.BearerToken). Those admin tools are served by their own
// containers (Grafana etc.) via full-page navigation, so the Vue SPA — and its
// axios /api/auth/refresh interceptor — is NOT running. Once the access_token
// cookie expires, every admin sub-request 401s until the user bounces through
// the SPA or re-logs in. (Root cause of "auth randomly falls to UNAUTHORIZED".)
//
// This middleware closes that gap server-side: when no valid access token is
// present but a `refresh_token` cookie is, it calls the auth service's
// /api/auth/refresh once, relays the resulting Set-Cookie headers back to the
// browser, and rewrites the request's Authorization header with the fresh
// access token so the downstream (unchanged) JWTValidationMiddleware validates
// it. It never authorizes by itself — JWTValidationMiddleware + AdminRoleMiddleware
// remain the gate; a request with neither a valid access token nor a usable
// refresh token still 401s exactly as before.
package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// refreshCookieName is the cookie the auth service issues for refresh tokens.
// Mirrors services/auth/internal/handler.refreshTokenCookieName (different
// package); kept as a local const to avoid a cross-service import.
const refreshCookieName = "refresh_token"

// refreshResult is the shared outcome of one /api/auth/refresh round-trip.
type refreshResult struct {
	accessToken string
	setCookies  []string
	ok          bool
}

// refreshCall is an in-flight (or just-completed) refresh shared by all
// goroutines that requested the same refresh-token value.
type refreshCall struct {
	done chan struct{}
	res  refreshResult
}

// adminRefresher performs the auth-service refresh call and de-duplicates
// concurrent refreshes of the same token (single-flight). A Grafana dashboard
// load fires dozens of parallel sub-requests; without de-dup each would spawn
// its own /api/auth/refresh POST. Auth's 20-min grace window makes concurrent
// rotations safe regardless, but single-flight removes the stampede.
type adminRefresher struct {
	authRefreshURL string
	jwt            *authz.JWTManager
	client         *http.Client
	log            *logger.Logger

	mu       sync.Mutex
	inflight map[string]*refreshCall
}

func newAdminRefresher(jwtConfig authz.JWTConfig, authServiceURL string, log *logger.Logger) *adminRefresher {
	return &adminRefresher{
		authRefreshURL: strings.TrimRight(authServiceURL, "/") + "/api/auth/refresh",
		jwt:            authz.NewJWTManager(jwtConfig),
		// Reuse the shared, JAR-LESS client (no cookie jar — required so the
		// auth response's Set-Cookie headers stay in resp.Header for relay; a
		// jar would consume them). 5s timeout bounds the hang on auth slowness.
		client:   getApiKeyHTTPClient(),
		log:      log,
		inflight: make(map[string]*refreshCall),
	}
}

// refresh returns the (single-flighted) result of refreshing refreshToken.
func (a *adminRefresher) refresh(refreshToken string) refreshResult {
	key := hashRefreshToken(refreshToken)

	a.mu.Lock()
	if call, ok := a.inflight[key]; ok {
		a.mu.Unlock()
		<-call.done
		return call.res
	}
	call := &refreshCall{done: make(chan struct{})}
	a.inflight[key] = call
	a.mu.Unlock()

	call.res = a.doRefresh(refreshToken)
	close(call.done)

	a.mu.Lock()
	delete(a.inflight, key)
	a.mu.Unlock()

	return call.res
}

// doRefresh performs the actual POST /api/auth/refresh. The auth handler reads
// the refresh token from the Cookie header, so we send it that way.
func (a *adminRefresher) doRefresh(refreshToken string) refreshResult {
	req, err := http.NewRequest(http.MethodPost, a.authRefreshURL, nil)
	if err != nil {
		return refreshResult{}
	}
	req.Header.Set("Cookie", refreshCookieName+"="+refreshToken)

	resp, err := a.client.Do(req)
	if err != nil {
		a.log.Warnw("admin session refresh: auth call failed", "error", err)
		return refreshResult{}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// A non-200 here is a real refresh failure (dead/revoked session, auth
		// outage). Log it — silence made the original envelope-decode bug
		// invisible: admin sessions 401'd with no trace for weeks.
		a.log.Warnw("admin session refresh: auth returned non-200", "status", resp.StatusCode)
		return refreshResult{}
	}

	// The auth service wraps its payload in the httputil.OK envelope
	// ({success, data:{access_token,...}}). Decode the nested field; a flat
	// top-level access_token (legacy/defensive) is accepted as a fallback.
	var body struct {
		AccessToken string `json:"access_token"`
		Data        struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		a.log.Warnw("admin session refresh: decode auth response failed", "error", err)
		return refreshResult{}
	}
	accessToken := body.Data.AccessToken
	if accessToken == "" {
		accessToken = body.AccessToken
	}
	if accessToken == "" {
		a.log.Warnw("admin session refresh: auth response had no access_token")
		return refreshResult{}
	}

	return refreshResult{
		accessToken: accessToken,
		setCookies:  resp.Header.Values("Set-Cookie"),
		ok:          true,
	}
}

func hashRefreshToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// AdminSessionRefreshMiddleware tops up an expired/absent access token from the
// refresh_token cookie before JWTValidationMiddleware runs. Wire it FIRST in
// the /admin group, ahead of JWTValidationMiddleware.
func AdminSessionRefreshMiddleware(jwtConfig authz.JWTConfig, authServiceURL string, log *logger.Logger) func(http.Handler) http.Handler {
	ar := newAdminRefresher(jwtConfig, authServiceURL, log)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fast path: a valid access token is already present (header or
			// access_token cookie). Do nothing — no auth round-trip.
			if tok := httputil.BearerToken(r); tok != "" {
				if _, err := ar.jwt.ValidateAccessToken(tok); err == nil {
					next.ServeHTTP(w, r)
					return
				}
			}

			// No valid access token: attempt a transparent refresh.
			c, err := r.Cookie(refreshCookieName)
			if err != nil || c.Value == "" {
				// Nothing to refresh with — let JWTValidationMiddleware 401.
				next.ServeHTTP(w, r)
				return
			}

			res := ar.refresh(c.Value)
			if !res.ok {
				// Refresh failed — fall through; JWTValidationMiddleware 401s
				// exactly as it did before this middleware existed.
				next.ServeHTTP(w, r)
				return
			}

			// Relay the fresh cookies to the browser (new access_token, and the
			// re-set non-rotating refresh_token with its sliding max-age) and
			// hand the fresh access token to the downstream JWTValidationMiddleware via the
			// Authorization header (BearerToken prefers the header over the
			// still-expired access_token cookie).
			for _, sc := range res.setCookies {
				w.Header().Add("Set-Cookie", sc)
			}
			r.Header.Set("Authorization", "Bearer "+res.accessToken)
			next.ServeHTTP(w, r)
		})
	}
}
