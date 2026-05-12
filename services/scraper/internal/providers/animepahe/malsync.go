package animepahe

import (
	"context"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// Cache TTLs locked by the test layer; real implementation reads these
// constants. Defined in RED so the tests compile.
const (
	malSyncCacheTTL = 24 * time.Hour
	malSyncMissTTL  = 24 * time.Hour
)

// MalSyncClient resolves a MAL ID to a provider-specific identifier via
// the api.malsync.moe service. Results are cached for 24h.
//
// RED stub — see malsync_green.go after Task 1 GREEN lands.
type MalSyncClient struct {
	http    *http.Client
	cache   cache.Cache
	baseURL string
}

// MalSyncOption configures a MalSyncClient.
type MalSyncOption func(*MalSyncClient)

// WithMalSyncHTTPClient overrides the http.Client used to call malsync.moe.
func WithMalSyncHTTPClient(c *http.Client) MalSyncOption {
	return func(m *MalSyncClient) { panic("not implemented (RED)") }
}

// WithMalSyncBaseURL overrides the malsync base URL.
func WithMalSyncBaseURL(u string) MalSyncOption {
	return func(m *MalSyncClient) { panic("not implemented (RED)") }
}

// NewMalSyncClient — RED stub.
func NewMalSyncClient(c cache.Cache, opts ...MalSyncOption) *MalSyncClient {
	panic("not implemented (RED)")
}

// Lookup resolves (malID, provider) → providerID. RED stub.
func (m *MalSyncClient) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	panic("not implemented (RED)")
}

// Suppress unused-import / unused-field warnings while stubbed.
var _ = (*MalSyncClient)(nil)
var _ time.Duration = 0
