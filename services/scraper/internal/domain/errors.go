// Package domain defines the core types and contracts shared by every scraper
// provider, the orchestrator, and the HTTP handler layer.
//
// These three sentinel errors drive orchestrator failover semantics. Providers
// MUST distinguish "real-empty" (which is not an error at all — return a normal
// empty slice with nil error) from each of the three failure modes below.
// SCRAPER-FOUND-02 / REQ catalog.
package domain

import (
	"errors"
	"fmt"
)

// ErrNotFound — the upstream provider answered, but the anime / episode /
// server identified by the caller does not exist there. Orchestrator: try the
// next provider in the chain.
var ErrNotFound = errors.New("scraper: not found")

// ErrProviderDown — the upstream provider could not be contacted at all (timeout,
// 5xx, DNS, conn-refused, anti-bot block). Orchestrator: try the next provider
// and surface the upstream incident to observability.
var ErrProviderDown = errors.New("scraper: provider down")

// ErrExtractFailed — the upstream provider responded, but the response could
// not be parsed / decrypted / shape-matched into the expected DTO. Orchestrator:
// log + alert (this signals a regression in our parsing code or an upstream
// HTML restructure), then try the next provider.
var ErrExtractFailed = errors.New("scraper: extract failed")

// WrapNotFound wraps an underlying cause with the ErrNotFound sentinel.
//
// The dual `%w` verbs (Go 1.20+) mean both `errors.Is(err, ErrNotFound)` and
// `errors.Is(err, cause)` return true on the returned error — orchestrator
// code can match on the high-level category AND log the original cause.
func WrapNotFound(cause error, msg string) error {
	return fmt.Errorf("%s: %w (cause: %w)", msg, ErrNotFound, cause)
}

// WrapProviderDown wraps a transport / availability failure with ErrProviderDown.
// See WrapNotFound for the multi-`%w` rationale.
func WrapProviderDown(cause error, msg string) error {
	return fmt.Errorf("%s: %w (cause: %w)", msg, ErrProviderDown, cause)
}

// WrapExtractFailed wraps a parse / decrypt / shape-mismatch failure with
// ErrExtractFailed. See WrapNotFound for the multi-`%w` rationale.
func WrapExtractFailed(cause error, msg string) error {
	return fmt.Errorf("%s: %w (cause: %w)", msg, ErrExtractFailed, cause)
}
