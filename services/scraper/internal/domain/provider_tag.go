package domain

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// providerKey is the private (unexported) context-key type for the scraper's
// stream-provider tag. Using a named string type instead of a bare string
// avoids collisions with any other package that might stash a value under a
// plain "provider" string key (Go's documented context-key idiom).
type providerKey string

const providerTagKey providerKey = "scraper.stream_provider"

// ProviderContext tags ctx with the stream-provider responsible for the
// outbound requests made under it (D-02/D-09). The recording transport reads
// this tag (via tracing.ProviderFromContext) so streaming egress rows pivot by
// `target = provider + host`, not just host (the CDN host hides the provider).
//
// It writes the tag in TWO places intentionally:
//
//  1. tracing's private provider ctx value — this is the value the recording
//     RoundTripper in libs/tracing reads at outbound time. This is what makes
//     the provider actually reach the effect row.
//  2. this package's own private key — so ProviderFromContext can read it back
//     within the scraper without depending on tracing's read path.
//
// Empty name is a no-op (general, non-streaming egress carries NO provider per
// D-01). The provider tag rides a PRIVATE ctx value and is never put on W3C
// wire baggage, so it cannot leak to 3rd-party hosts (T-02-PII).
func ProviderContext(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	ctx = tracing.WithProvider(ctx, name)
	return context.WithValue(ctx, providerTagKey, name)
}

// ProviderFromContext reads the stream-provider tag set by ProviderContext, or
// "" when unset (general egress). It reads the scraper-domain key; the
// recording transport independently reads tracing's mirror of the same value.
func ProviderFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(providerTagKey).(string); ok {
		return v
	}
	return ""
}
