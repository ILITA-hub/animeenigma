package capability

import (
	"context"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// CatalogSource is the subset of *service.CatalogService the RU/Hanime family
// adapters use. *service.CatalogService satisfies it; tests pass a fake.
// May be nil on the Service — then only the EN family is assembled.
type CatalogSource interface {
	GetKodikTranslations(ctx context.Context, animeID string) ([]domain.KodikTranslation, error)
	GetAnimeLibTranslations(ctx context.Context, animeID string, episodeID int) ([]domain.AnimeLibTranslation, error)
	GetHanimeEpisodes(ctx context.Context, animeID string) ([]domain.HanimeEpisode, error)
	GetHanimeStream(ctx context.Context, animeID string, slug string) (*domain.HanimeStream, error)
}

// categoryFromTranslationType maps Kodik/AniLib's normalized translation type
// ("voice" = audio dub, "subtitles" = subbed) to a capability category.
func categoryFromTranslationType(t string) string {
	if strings.EqualFold(strings.TrimSpace(t), "subtitles") {
		return "sub"
	}
	return "dub"
}

// subDeliveryFor derives the subtitle-delivery for a RU variant. A dub is audio
// (no subs → "none"); a sub carries soft subs only when external files exist,
// otherwise the subs are baked/iframe-rendered ("hard", not separately rendered).
func subDeliveryFor(category string, hasExternalSubs bool) string {
	if category != "sub" {
		return "none"
	}
	if hasExternalSubs {
		return "soft"
	}
	return "hard"
}

// formatQuality normalizes a bare height ("720") to a label ("720p"); passes
// through anything that isn't a plain number unchanged.
func formatQuality(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if _, err := strconv.Atoi(h); err == nil {
		return h + "p"
	}
	return h
}

// providerRow loads one stream_providers row by name. ok=false when absent.
func (s *Service) providerRow(ctx context.Context, name string) (domain.ScraperProvider, bool) {
	if s.db == nil {
		return domain.ScraperProvider{}, false
	}
	var row domain.ScraperProvider
	if err := s.db.WithContext(ctx).Where("name = ?", name).First(&row).Error; err != nil {
		return domain.ScraperProvider{}, false
	}
	return row, true
}

// applyFeedFields fills the feed presentation on a built provider cap from its
// DB row. Returns ok=false when the row is disabled (caller omits the family).
// RU/adult families that reach here already have catalog content, so
// deriveProviderView is called with hasContent=true.
func applyFeedFields(cap *domain.ProviderCap, row domain.ScraperProvider) bool {
	if !row.IsRegistered() { // disabled → omit
		return false
	}
	state, selectable, hackerOnly := deriveProviderView(row, true)
	cap.State, cap.Selectable, cap.HackerOnly = state, selectable, hackerOnly
	cap.Order = row.PreferenceWeight
	cap.Group = wireGroup(row.Group)
	cap.Audios = audiosFromTraits(row)
	cap.Reason = row.Reason
	return true
}

// kodikFamily builds the "kodik" family: one provider whose variants are the
// real translation teams (Kodik exposes team names; iframe hides quality). Best
// effort — returns ok=false on error, when the anime isn't on Kodik, or when the
// `kodik-noads` DB row is disabled (the served no-ads variant gates the family).
func (s *Service) kodikFamily(ctx context.Context, animeID string) (domain.SourceFamily, bool) {
	trs, err := s.catalog.GetKodikTranslations(ctx, animeID)
	if err != nil {
		if s.log != nil {
			s.log.Warnw("capability kodik family skipped", "anime_id", animeID, "error", err)
		}
		return domain.SourceFamily{}, false
	}
	if len(trs) == 0 {
		return domain.SourceFamily{}, false
	}
	variants := make([]domain.Variant, 0, len(trs))
	for _, tr := range trs {
		cat := categoryFromTranslationType(tr.Type)
		variants = append(variants, domain.Variant{
			Category:      cat,
			Team:          &domain.Team{ID: strconv.Itoa(tr.ID), Name: tr.Title},
			SubDelivery:   subDeliveryFor(cat, false), // iframe — no external soft subs
			QualitySource: "unknown",                  // iframe hides quality ladder
			Source:        "discovered",
		})
	}
	row, ok := s.providerRow(ctx, "kodik-noads")
	if !ok {
		return domain.SourceFamily{}, false
	}
	cap := domain.ProviderCap{Provider: "kodik", DisplayName: "Kodik", Enabled: true, Health: "unknown", Variants: variants}
	if !applyFeedFields(&cap, row) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: "kodik", Providers: []domain.ProviderCap{cap}}, true
}

// animelibFamily builds the "animelib" family from translation teams of the
// first episode: real team names, soft/hard subs from HasSubtitles. Best effort —
// also omitted when the `animelib` DB row is disabled.
func (s *Service) animelibFamily(ctx context.Context, animeID string) (domain.SourceFamily, bool) {
	const firstEpisode = 1
	trs, err := s.catalog.GetAnimeLibTranslations(ctx, animeID, firstEpisode)
	if err != nil {
		if s.log != nil {
			s.log.Warnw("capability animelib family skipped", "anime_id", animeID, "error", err)
		}
		return domain.SourceFamily{}, false
	}
	if len(trs) == 0 {
		return domain.SourceFamily{}, false
	}
	variants := make([]domain.Variant, 0, len(trs))
	for _, tr := range trs {
		cat := categoryFromTranslationType(tr.Type)
		variants = append(variants, domain.Variant{
			Category:      cat,
			Team:          &domain.Team{ID: strconv.Itoa(tr.ID), Name: tr.TeamName},
			SubDelivery:   subDeliveryFor(cat, tr.HasSubtitles),
			QualitySource: "unknown", // quality ladder needs a per-team stream call; omitted
			Source:        "discovered",
		})
	}
	row, ok := s.providerRow(ctx, "animelib")
	if !ok {
		return domain.SourceFamily{}, false
	}
	cap := domain.ProviderCap{Provider: "animelib", DisplayName: "AniLib", Enabled: true, Health: "unknown", Variants: variants}
	if !applyFeedFields(&cap, row) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: "animelib", Providers: []domain.ProviderCap{cap}}, true
}

// hanimeFamily builds the "hanime" family: a single raw variant with the quality
// ladder of the first episode's stream. Best effort — omitted when the anime
// isn't on Hanime or the `hanime` DB row is disabled; quality is dropped (not the
// whole family) if the stream call fails after episodes resolve.
func (s *Service) hanimeFamily(ctx context.Context, animeID string) (domain.SourceFamily, bool) {
	eps, err := s.catalog.GetHanimeEpisodes(ctx, animeID)
	if err != nil {
		if s.log != nil {
			s.log.Warnw("capability hanime family skipped", "anime_id", animeID, "error", err)
		}
		return domain.SourceFamily{}, false
	}
	if len(eps) == 0 {
		return domain.SourceFamily{}, false
	}

	qualities := []string{}
	qualitySource := "unknown"
	if stream, err := s.catalog.GetHanimeStream(ctx, animeID, eps[0].Slug); err == nil && stream != nil {
		seen := map[string]bool{}
		for _, src := range stream.Sources {
			q := formatQuality(src.Height)
			if q == "" || seen[q] {
				continue
			}
			seen[q] = true
			qualities = append(qualities, q)
		}
		if len(qualities) > 0 {
			qualitySource = "discrete"
		}
	} else if err != nil && s.log != nil {
		s.log.Warnw("capability hanime stream quality unavailable", "anime_id", animeID, "error", err)
	}

	variant := domain.Variant{
		Category:      "raw",
		SubDelivery:   "none",
		QualitySource: qualitySource,
		Source:        "discovered",
	}
	if len(qualities) > 0 {
		variant.Qualities = qualities
	}
	row, ok := s.providerRow(ctx, "hanime")
	if !ok {
		return domain.SourceFamily{}, false
	}
	cap := domain.ProviderCap{Provider: "hanime", DisplayName: "Hanime", Enabled: true, Health: "unknown", Variants: []domain.Variant{variant}}
	if !applyFeedFields(&cap, row) {
		return domain.SourceFamily{}, false
	}
	return domain.SourceFamily{Family: "hanime", Providers: []domain.ProviderCap{cap}}, true
}
