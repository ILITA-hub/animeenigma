package sourceranking

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const srcfixTTL = 24 * time.Hour

// knownProviders is the allowlist a srcfix value must match (mirrors the frontend
// CURATED_TIER ids + EN scraper ids). Keeps garbage out of the public override key.
// SYNC: keep in step with frontend providerRegistry.ts CURATED_TIER (+ EN scraper ids).
var knownProviders = map[string]struct{}{
	"ae": {}, "allanime": {}, "gogoanime": {}, "miruro": {}, "animepahe": {},
	"animefever": {}, "nineanime": {}, "animekai": {}, "kodik": {}, "raw": {},
	"18anime": {}, "animelib": {}, "hanime": {},
}

type stringSetter interface {
	SetString(ctx context.Context, key, val string, ttl time.Duration) error
}

// Writer writes the same-day per-anime srcfix override key.
type Writer struct{ cache stringSetter }

func NewWriter(c stringSetter) *Writer { return &Writer{cache: c} }

// SetFix validates the provider against the allowlist and writes srcfix:{id} with
// a 24h TTL. An empty animeID, non-UUID animeID, or unknown provider returns an
// error and writes nothing. The UUID check caps the srcfix key namespace to
// plausible catalog PKs (gen_random_uuid()) so a public endpoint can't flood Redis.
func (w *Writer) SetFix(ctx context.Context, animeID, provider string) error {
	if animeID == "" {
		return errors.New("empty animeID")
	}
	if _, err := uuid.Parse(animeID); err != nil {
		return errors.New("invalid animeID")
	}
	if _, ok := knownProviders[provider]; !ok {
		return errors.New("unknown provider")
	}
	return w.cache.SetString(ctx, "srcfix:"+animeID, provider, srcfixTTL)
}
