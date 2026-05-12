package animepahe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// ddosCookieNamePrefix is the prefix for the cookie DDoS-Guard sets on the
// target host after a successful bypass GET. In 2025/2026 the cookie name
// includes a versioned suffix (e.g. `__ddg2_BvHvjMmh`), so an exact-match
// check would always miss the real cookie and force the handshake to
// re-run on every request — or worse, mis-report the handshake as failed.
// See REVIEW.md CR-05.
const ddosCookieNamePrefix = "__ddg2_"

// ddosCheckURL is the path that DDoS-Guard's bootstrap JS lives at. The
// response body is shaped like  var x='/foo/bar/baz';  — a single-quoted path
// is the ONLY thing we extract.
const ddosCheckURL = "https://check.ddos-guard.net/check.js"

// ddosCheckMaxBody caps the check.js body at 64 KiB. Real bodies are ~1 KiB;
// this is a DoS / surprise-fix guard.
const ddosCheckMaxBody = 64 << 10

// ensureDDoSCookie performs DDoS-Guard's two-step handshake to populate the
// HTTP client's cookie jar with `__ddg2_` for the target host. If the jar
// already contains a non-empty `__ddg2_` cookie for the target, this function
// is a no-op (returns nil immediately).
//
// Protocol (per RESEARCH.md Pattern 3):
//
//  1. GET check.ddos-guard.net/check.js — returns var x='/secret-path';
//  2. Extract the single-quoted path.
//  3. GET <target.scheme>://<target.host>{path} — DDoS-Guard sets __ddg2_
//     cookie on the response.
//
// Security:
//
//   - check.js parser is strict (`strings.SplitN(body, "'", 3)`, requires
//     exactly 3 parts). Malformed bodies return a wrapped ErrExtractFailed
//     and the jar is left unchanged.
//   - The bypass URL is constructed from `target.Scheme` + `target.Host` —
//     the host from check.js body is NEVER used. This blocks check.ddos-guard.net
//     from steering us to attacker.example.com.
func ensureDDoSCookie(ctx context.Context, hc *domain.BaseHTTPClient, target *url.URL) error {
	if target == nil {
		return errors.New("ensureDDoSCookie: target URL is nil")
	}
	// Idempotency: do nothing if we already have a non-empty __ddg2_ for this
	// host. The jar is scoped to public-suffix etld+1, so Cookies(target)
	// returns animepahe.ru cookies for any subdomain too.
	jar := hc.Jar()
	if jar == nil {
		return errors.New("ensureDDoSCookie: client has no cookie jar")
	}
	for _, c := range jar.Cookies(target) {
		if strings.HasPrefix(c.Name, ddosCookieNamePrefix) && c.Value != "" {
			return nil
		}
	}

	// 1. Fetch check.js.
	resp, err := hc.Get(ctx, ddosCheckURL)
	if err != nil {
		return domain.WrapProviderDown(err, "ddos-guard check.js fetch failed")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return domain.WrapProviderDown(
			fmt.Errorf("status %d", resp.StatusCode),
			"ddos-guard check.js non-200",
		)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ddosCheckMaxBody))
	if err != nil {
		return domain.WrapProviderDown(err, "ddos-guard check.js read body")
	}

	// 2. Extract the single-quoted path. Real response body example:
	//      (function(){var x='/.well-known/ddos-guard/id/abc';...})()
	// Strict parse: require exactly three pieces after splitting on `'`.
	parts := strings.SplitN(string(body), "'", 3)
	if len(parts) != 3 {
		return domain.WrapExtractFailed(
			errors.New("expected single-quoted path"),
			"ddos-guard check.js shape changed",
		)
	}
	path := parts[1]
	if path == "" || !strings.HasPrefix(path, "/") {
		return domain.WrapExtractFailed(
			fmt.Errorf("invalid bypass path %q", path),
			"ddos-guard check.js shape changed",
		)
	}

	// 3. GET the bypass URL on the TARGET host (never check.ddos-guard.net's
	// host). The response Set-Cookie: __ddg2_=... lands in the jar.
	bypassURL := target.Scheme + "://" + target.Host + path
	bresp, err := hc.Get(ctx, bypassURL)
	if err != nil {
		return domain.WrapProviderDown(err, "ddos-guard bypass GET failed")
	}
	defer func() {
		_, _ = io.Copy(io.Discard, bresp.Body)
		_ = bresp.Body.Close()
	}()
	// Don't error on non-200 here — DDoS-Guard sometimes returns 200, sometimes
	// 403, but always SETS the cookie if the bypass URL is reachable.
	// Re-check the jar (matches on the `__ddg2_` prefix, not exact name — see
	// CR-05; real cookies carry a versioned suffix like `__ddg2_BvHvjMmh`):
	for _, c := range jar.Cookies(target) {
		if strings.HasPrefix(c.Name, ddosCookieNamePrefix) && c.Value != "" {
			return nil
		}
	}
	return domain.WrapExtractFailed(
		errors.New("__ddg2_* cookie not set after bypass GET"),
		"ddos-guard handshake produced no cookie",
	)
}
