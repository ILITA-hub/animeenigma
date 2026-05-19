package allanime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// pickDomain returns the current active domain, probing the configured list
// when needed. First-success caching: once a domain returns any HTTP response
// (alive, even if it 5xxs the specific query), it's pinned for the process
// lifetime until a sustained failure plus cooldown invalidates it.
//
// The probe is intentionally lenient — a 500 from the GraphQL endpoint still
// means the host is up. We let the actual query path decide whether to mark
// the domain failed via markDomainFailed when subsequent requests fail.
func (c *Client) pickDomain(ctx context.Context) (string, error) {
	c.mu.RLock()
	active := c.activeDomain
	failed := c.failedAt
	c.mu.RUnlock()

	// Within cooldown of a recent failure, fall through to a re-probe.
	if active != "" && (failed.IsZero() || time.Since(failed) > c.domainCooldown) {
		return active, nil
	}

	// Probe each domain in order; first to respond at all wins. Reachability
	// is enough — content-level errors (4xx/5xx) are handled by the query.
	for _, d := range c.cfg.Domains {
		if c.probeDomain(ctx, d) {
			c.mu.Lock()
			c.activeDomain = d
			c.failedAt = time.Time{}
			c.mu.Unlock()
			return d, nil
		}
	}

	return "", errors.New("allanime: all domains unreachable")
}

// markDomainFailed records that the active domain just failed; subsequent
// pickDomain calls will re-probe after the cooldown.
func (c *Client) markDomainFailed() {
	c.mu.Lock()
	c.failedAt = time.Now()
	c.mu.Unlock()
}

// probeDomain checks whether https://api.{d}/api is reachable at the TCP
// layer. Any HTTP response — including 4xx and 5xx — counts as "alive"; the
// goal is to avoid sitting on a dead domain when the configured list has a
// healthy alternative. Content-level failures (stale persisted-query SHA,
// transient 500s) are surfaced separately by the query path.
func (c *Client) probeDomain(ctx context.Context, d string) bool {
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	u := fmt.Sprintf("https://api.%s/api", d)
	req, err := http.NewRequestWithContext(pctx, http.MethodGet, u, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Referer", c.cfg.Referer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// endpointURL builds the GraphQL endpoint for a domain. AllAnime serves both
// the static site and the API from the same `api.` host with a `/api` path.
// Query, variables, and persisted-query extensions go in the query string
// for GET requests (Apollo's GET protocol + APQ auto-registration). Sending
// `query` alongside `extensions` makes Apollo register the operation under
// our SHA on cache miss, so we never need to chase server-side SHA rotations.
func (c *Client) endpointURL(d, gqlQuery, variables, extensions string) string {
	v := url.Values{}
	if gqlQuery != "" {
		v.Set("query", gqlQuery)
	}
	v.Set("variables", variables)
	v.Set("extensions", extensions)
	return fmt.Sprintf("https://api.%s/api?%s", d, v.Encode())
}

// joinErrs builds a multi-line error string from a list of per-domain failures.
func joinErrs(errs []string) string {
	return strings.Join(errs, "; ")
}
