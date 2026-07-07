package capability

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// LibrarySource reports the aggregated per-title self-hosted ("ae") audio
// facts (Phase C source-panel truth). aeFamily derives BOTH presence
// (AeInfo.Present) and the real dub/original variant from this single call.
// HasLibraryTitle — a separate has-any-episode-only lookup — was folded into
// AeTitleInfo and removed once this became the only caller (2026-07-07).
type LibrarySource interface {
	AeTitleInfo(ctx context.Context, animeID string) (service.AeInfo, error)
}

// aeLangFromISO maps AeInfo's normalized ISO 639-2 audio_lang ("eng"/"rus"/
// "jpn") to the FE's ProviderCap.Lang code. Returns "" for anything that
// isn't a real localized-dub language (including "jpn" — there is no
// Japanese dub; original JP audio surfaces as the "sub"/raw category, not a
// dub with a lang override).
func aeLangFromISO(iso string) string {
	switch iso {
	case "eng":
		return "en"
	case "rus":
		return "ru"
	default:
		return ""
	}
}

// aeVariantsFromInfo builds the ae family's real variant + audios + lang from
// the title's self-hosted audio facts. ok=false means the info can't be
// mapped to a known audience (not present, or a dub whose language didn't
// normalize to eng/rus) — the caller falls back to variantsFromTraits.
func aeVariantsFromInfo(info service.AeInfo, row domain.ScraperProvider) (variants []domain.Variant, audios []string, lang string, ok bool) {
	if !info.Present {
		return nil, nil, "", false
	}
	var category, subDelivery string
	switch info.Track {
	case "dub":
		lang = aeLangFromISO(info.AudioLang)
		if lang == "" {
			return nil, nil, "", false // unrecognized dub language — trait fallback
		}
		category, subDelivery, audios = "dub", "none", []string{"dub"}
	case "raw":
		category, subDelivery, audios = "sub", row.SubDelivery, []string{"sub"}
	default:
		return nil, nil, "", false // unexpected/empty track — trait fallback
	}
	quality := info.Quality
	if quality == "" {
		quality = row.QualityCeiling
	}
	var qualities []string
	if quality != "" {
		qualities = []string{quality}
	}
	variant := domain.Variant{
		Category: category, SubDelivery: subDelivery, Qualities: qualities,
		QualitySource: "probed", Source: "discovered",
	}
	return []domain.Variant{variant}, audios, lang, true
}

// aeFamily builds the first-party "ae" family. The provider is ALWAYS emitted
// (so the user sees it), but is `no_content` (tinted, not selectable) until the
// title is encoded into the library. When encoded, its variants/audios/lang
// reflect the REAL per-title audio facts (AeInfo) instead of the provider's
// generic sub/dub traits — a self-hosted English dub surfaces as
// Audios:["dub"] + Lang:"en", not the old fabricated "sub". A library lookup
// failure, or content present but not classifiable (no usable dub language),
// falls back to the trait-only variants rather than dropping the family.
// Omitted only when the DB row is absent or disabled.
func (s *Service) aeFamily(ctx context.Context, animeID string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, "ae")
	if !ok || !row.IsRegistered() {
		return domain.SourceFamily{}, false
	}
	var info service.AeInfo
	if s.library != nil {
		if i, err := s.library.AeTitleInfo(ctx, animeID); err == nil {
			info = i
		} else if s.log != nil {
			s.log.Warnw("ae title info lookup failed; tinting", "anime_id", animeID, "error", err)
		}
	}
	pc := domain.ProviderCap{Provider: "ae", DisplayName: "AnimeEnigma"}
	variants, audios, lang, usable := aeVariantsFromInfo(info, row)
	if !usable {
		variants = variantsFromTraits(row)
	}
	pc.Variants = variants
	applyFeedFields(ctx, &pc, row, info.Present) // row verified registered above; ok is always true
	if usable {
		pc.Audios = audios
		pc.Lang = lang
	}
	// Late-only library flag: a present ae library that doesn't hold episode 1
	// must not win the fresh-open smart default (the FE would otherwise open its
	// lone late episode instead of ep 1). Complete libraries (covers ep 1) omit
	// this and stay the preferred default.
	pc.PartialLibrary = info.Present && !info.CoversFirstEpisode
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
	pc := domain.ProviderCap{Provider: providerName, DisplayName: displayName, Variants: variantsFromTraits(row)}
	if !applyFeedFields(ctx, &pc, row, true) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: family, Providers: []domain.ProviderCap{pc}}, true
}
