package capability

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// LibrarySource reports whether AnimeEnigma has a title self-hosted (library/MinIO).
type LibrarySource interface {
	HasLibraryTitle(ctx context.Context, animeID string) (bool, error)
}

// aeFamily builds the first-party "ae" family. The provider is ALWAYS emitted
// (so the user sees it), but is `no_content` (tinted, not selectable) until the
// title is encoded into the library. A library lookup failure falls back to
// no_content rather than dropping the family. Omitted only when the DB row is
// absent or disabled.
func (s *Service) aeFamily(ctx context.Context, animeID string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, "ae")
	if !ok || !row.IsRegistered() {
		return domain.SourceFamily{}, false
	}
	has := false
	if s.library != nil {
		if h, err := s.library.HasLibraryTitle(ctx, animeID); err == nil {
			has = h
		} else if s.log != nil {
			s.log.Warnw("ae library presence lookup failed; tinting", "anime_id", animeID, "error", err)
		}
	}
	pc := domain.ProviderCap{
		Provider: "ae", DisplayName: "AnimeEnigma", Enabled: true, Health: "up",
		Variants: variantsFromTraits(row),
	}
	applyFeedFields(&pc, row, has) // row verified registered above; ok is always true
	return domain.SourceFamily{Family: "ae", Providers: []domain.ProviderCap{pc}}, true
}

// dbRowFamily builds a single-provider family straight from its stream_providers
// row — for the trait-only sources (raw JP original-audio, 18anime) that need no
// live catalog lookup. Phase 1 hasContent=true (always shown when the row is
// enabled — parity with the old registry, where non-scraper providers always
// rendered selectable). Omitted when the row is absent or disabled.
func (s *Service) dbRowFamily(ctx context.Context, providerName, displayName, family string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, providerName)
	if !ok {
		return domain.SourceFamily{}, false
	}
	pc := domain.ProviderCap{Provider: providerName, DisplayName: displayName, Enabled: true, Health: "up", Variants: variantsFromTraits(row)}
	if !applyFeedFields(&pc, row, true) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: family, Providers: []domain.ProviderCap{pc}}, true
}
