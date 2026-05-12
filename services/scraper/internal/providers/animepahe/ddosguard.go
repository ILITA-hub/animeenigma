package animepahe

import (
	"context"
	"net/url"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// ddosCookieName is the cookie DDoS-Guard sets on the target host. The
// real value is locked into the cache jar by ensureDDoSCookie.
const ddosCookieName = "__ddg2_"

// ensureDDoSCookie performs DDoS-Guard's two-step handshake. RED stub.
func ensureDDoSCookie(ctx context.Context, hc *domain.BaseHTTPClient, target *url.URL) error {
	panic("not implemented (RED)")
}
