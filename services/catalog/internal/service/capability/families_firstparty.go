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
	state, selectable, hackerOnly := deriveProviderView(row, has)
	pc.State, pc.Selectable, pc.HackerOnly = state, selectable, hackerOnly
	pc.Order = row.PreferenceWeight
	pc.Group = wireGroup(row.Group)
	pc.Audios = audiosFromTraits(row)
	pc.Reason = row.Reason
	return domain.SourceFamily{Family: "ae", Providers: []domain.ProviderCap{pc}}, true
}

// rawFamily builds the "raw" (JP original-audio) family from its DB row. Phase 1
// hasContent=true (always shown when the row is enabled — parity with today's
// registry, where non-scraper providers always rendered selectable). Omitted
// when the row is absent or disabled.
func (s *Service) rawFamily(ctx context.Context, _ string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, "raw")
	if !ok {
		return domain.SourceFamily{}, false
	}
	pc := domain.ProviderCap{Provider: "raw", DisplayName: "Raw", Enabled: true, Health: "up", Variants: variantsFromTraits(row)}
	if !applyFeedFields(&pc, row) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: "raw", Providers: []domain.ProviderCap{pc}}, true
}

// adult18animeFamily builds the "adult" (18anime) family from its DB row. Phase 1
// hasContent=true (parity with today's registry). Omitted when the row is absent
// or disabled.
func (s *Service) adult18animeFamily(ctx context.Context, _ string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, "18anime")
	if !ok {
		return domain.SourceFamily{}, false
	}
	pc := domain.ProviderCap{Provider: "18anime", DisplayName: "18anime", Enabled: true, Health: "up", Variants: variantsFromTraits(row)}
	if !applyFeedFields(&pc, row) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: "adult", Providers: []domain.ProviderCap{pc}}, true
}
