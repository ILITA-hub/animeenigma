package service

// AnimeTosho subtitle provider — official simulcast subs (Erai-raws Multi-Sub
// = Crunchyroll rips, EN+RU+~8 more languages) extracted per-track by
// AnimeTosho and resolved here by AniDB series id. Added 2026-07-18 after the
// coverage probe found EN thin and RU near-zero for ongoing seasonals while
// AnimeTosho had official CR tracks for the same episodes within hours of
// airing.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animetosho"
)

const (
	// aniDBHitTTL / aniDBMissTTL cache the animeID → AniDB id resolution.
	// Hits are effectively permanent facts; misses retry sooner so a title
	// ARM maps later self-heals.
	aniDBHitTTL  = 7 * 24 * time.Hour
	aniDBMissTTL = 6 * time.Hour

	// maxToshoDetailFetches bounds how many candidate releases get a detail
	// (attachments) lookup per aggregate request. The preferred group's
	// release almost always wins on the first fetch.
	maxToshoDetailFetches = 3
)

// toshoPreferredGroup is checked first among matching releases: Erai-raws
// Multi-Sub releases carry the official CR tracks for every language CR
// produces, which is exactly the coverage this provider exists for.
const toshoPreferredGroup = "Erai-raws"

// toshoLangs maps AnimeTosho's ISO 639-2 track languages (both /B and /T
// variants) to the two-letter codes the aggregator serves. Untagged or
// unmapped tracks are skipped — a track we can't classify is worse than none.
var toshoLangs = map[string]string{
	"eng": "en", "rus": "ru", "jpn": "ja",
	"spa": "es", "por": "pt", "ara": "ar",
	"fre": "fr", "fra": "fr", "ger": "de", "deu": "de",
	"ita": "it", "chi": "zh", "zho": "zh", "kor": "ko",
	"vie": "vi", "tha": "th", "ind": "id", "may": "ms", "msa": "ms",
	"tur": "tr", "pol": "pl", "hin": "hi",
}

// toshoFormat maps mkv subtitle codec names to the FE parser's format ids.
// SRT rides in mkv as S_TEXT/UTF8, which AnimeTosho reports as "UTF-8".
func toshoFormat(codec string) string {
	switch strings.ToUpper(codec) {
	case "ASS", "SSA":
		return "ass"
	case "SRT", "SUBRIP", "UTF-8":
		return "srt"
	case "WEBVTT", "VTT":
		return "vtt"
	default:
		return ""
	}
}

// fetchAnimeTosho lists the official/fansub softsub tracks AnimeTosho
// extracted for this episode. Resolution chain: anime → AniDB id (ARM,
// Redis-cached) → per-series release list → episode-matched release
// (preferred group first) → subtitle attachments.
func (s *SubsAggregator) fetchAnimeTosho(ctx context.Context, anime *domain.Anime, episode int) ([]SubtitleTrack, error) {
	if s.tosho == nil || !s.tosho.IsConfigured() {
		return nil, errProviderUnconfigured
	}
	aniDBID, err := s.resolveAniDBID(ctx, anime)
	if err != nil {
		return nil, err
	}
	if aniDBID == 0 {
		return nil, nil // no AniDB mapping → nothing to search
	}

	releases, err := s.tosho.SearchByAniDB(ctx, aniDBID)
	if err != nil {
		return nil, err
	}

	target := episode
	isMovie := strings.EqualFold(anime.Kind, "movie")
	if isMovie {
		target = 1
	}

	// Candidates: releases whose title parses to the requested episode
	// (movies: releases with no episode marker at all). AniDB ids are
	// per-season, so series identity is already guaranteed. Preferred group
	// first, then feed order (newest first); one release per group so the
	// same subs don't repeat per resolution variant.
	var preferred, rest []animetosho.Release
	seenGroup := map[string]bool{}
	for _, r := range releases {
		ep, ok := animetosho.EpisodeFromTitle(r.Title)
		if isMovie {
			if ok {
				continue
			}
		} else if !ok || ep != target {
			continue
		}
		group := animetosho.ReleaseGroup(r.Title)
		key := strings.ToLower(group)
		if seenGroup[key] {
			continue
		}
		seenGroup[key] = true
		if strings.EqualFold(group, toshoPreferredGroup) {
			preferred = append(preferred, r)
		} else {
			rest = append(rest, r)
		}
	}
	candidates := append(preferred, rest...)

	// First candidate that actually carries subtitle attachments wins.
	fetches := 0
	for _, cand := range candidates {
		if fetches >= maxToshoDetailFetches {
			break
		}
		fetches++
		files, err := s.tosho.TorrentFiles(ctx, cand.ID)
		if err != nil {
			return nil, err
		}
		tracks := s.toshoTracksFromFiles(anime, files, target, isMovie)
		if len(tracks) > 0 {
			return tracks, nil
		}
	}
	return nil, nil
}

// toshoTracksFromFiles picks the release file for the target episode
// (single-file releases match trivially; batch files are matched by
// filename) and maps its subtitle attachments to tracks.
func (s *SubsAggregator) toshoTracksFromFiles(anime *domain.Anime, files []animetosho.TorrentFile, target int, isMovie bool) []SubtitleTrack {
	var file *animetosho.TorrentFile
	switch {
	case len(files) == 1:
		file = &files[0]
	case len(files) > 1 && !isMovie:
		for i := range files {
			if ep, ok := animetosho.EpisodeFromTitle(files[i].Filename); ok && ep == target {
				file = &files[i]
				break
			}
		}
	}
	if file == nil {
		return nil
	}

	group := animetosho.ReleaseGroup(file.Filename)
	var tracks []SubtitleTrack
	for _, a := range file.Attachments {
		if a.Type != "subtitle" {
			continue
		}
		lang, ok := toshoLangs[strings.ToLower(a.Info.Lang)]
		if !ok {
			continue
		}
		label := a.Info.Name
		if group != "" {
			if label != "" {
				label += " · "
			}
			label += group
		}
		if label == "" {
			label = "AnimeTosho"
		}
		tracks = append(tracks, SubtitleTrack{
			URL:      fmt.Sprintf("/api/anime/%s/subtitles/animetosho/file/%d", anime.ID, a.ID),
			Lang:     lang,
			Label:    label,
			Format:   toshoFormat(a.Info.Codec),
			Provider: "animetosho",
			Release:  file.Filename,
		})
	}
	return tracks
}

// resolveAniDBID maps an anime to its AniDB series id via ARM, cached in
// Redis (idmapping's AniList GraphQL fallback cannot supply AniDB, so a
// transient ARM outage is cached briefly as a miss and retried).
func (s *SubsAggregator) resolveAniDBID(ctx context.Context, anime *domain.Anime) (int, error) {
	cacheKey := "subs:animetosho:anidb:" + anime.ID
	var cached int
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	aniDBID := 0
	if s.idmap != nil && anime.ShikimoriID != "" {
		mapping, err := s.idmap.ResolveByShikimoriIDContext(ctx, anime.ShikimoriID)
		if err != nil {
			s.log.Debugw("subs aggregator: arm anidb lookup failed",
				"anime_id", anime.ID, "shikimori_id", anime.ShikimoriID, "error", err)
		} else if mapping != nil && mapping.AniDB != nil {
			aniDBID = *mapping.AniDB
		}
	}

	ttl := aniDBHitTTL
	if aniDBID == 0 {
		ttl = aniDBMissTTL
	}
	_ = s.cache.Set(ctx, cacheKey, aniDBID, ttl)
	return aniDBID, nil
}

// ResolveAnimeToshoFile turns an AnimeTosho attachment id into the subtitle
// bytes (xz-decompressed by the client), cached 24h.
func (s *SubsAggregator) ResolveAnimeToshoFile(ctx context.Context, attachID int) ([]byte, error) {
	if s.tosho == nil || !s.tosho.IsConfigured() {
		return nil, errProviderUnconfigured
	}
	cacheKey := fmt.Sprintf("subsfile:animetosho:%d", attachID)

	var hit cachedSubFile
	if err := s.cache.Get(ctx, cacheKey, &hit); err == nil && len(hit.Body) > 0 {
		return hit.Body, nil
	}

	body, err := s.tosho.DownloadAttachment(ctx, attachID)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, cachedSubFile{Body: body}, 24*time.Hour)
	return body, nil
}
