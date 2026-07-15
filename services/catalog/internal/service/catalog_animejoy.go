package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animejoy"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/streamsign"
)

// mapAnimejoyTeams reduces the discovery playlist's per-episode legs to the
// per-team presence flags the capability feed needs. PURE: HasSibnet /
// HasAllVideo report whether ANY episode of a team carries that leg's embed URL.
// Order is preserved.
func mapAnimejoyTeams(teams []animejoy.Team) []domain.AnimejoyTeam {
	out := make([]domain.AnimejoyTeam, 0, len(teams))
	for _, t := range teams {
		dt := domain.AnimejoyTeam{ID: t.ID, Name: t.Name}
		for _, ep := range t.Episodes {
			if ep.Sibnet != "" {
				dt.HasSibnet = true
			}
			if ep.AllVideo != "" {
				dt.HasAllVideo = true
			}
		}
		out = append(out, dt)
	}
	return out
}

// animejoyTitlesFor gathers the lookup titles for AnimeJoy discovery, primary
// first: romaji (Name), then English / Russian / Japanese variants, deduped and
// blank-stripped. AnimeJoy's DLE search keys on the first title; the rest feed
// the fuzzy scorer.
func animejoyTitlesFor(anime *domain.Anime) []string {
	titles := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, t := range []string{anime.Name, anime.NameEN, anime.NameRU, anime.NameJP} {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		titles = append(titles, t)
	}
	return titles
}

// resolveAnimejoyPlaylist resolves AnimeJoy discovery ONCE for an anime and
// returns the FULL per-team playlist (each episode carrying its Sibnet/AllVideo
// embed URLs). This is the shared discovery base for both the capability feed
// (GetAnimejoyTeams reduces it to per-leg presence) and the stream resolver
// (GetAnimejoyStream picks a concrete embed). Best-effort and CACHED 3h, keyed
// by anime_id (animejoy:playlist:<animeID>) — a discovery miss or error
// NEGATIVE-caches an empty slice and returns it, never blocking the feed.
// GetAnimejoyTeams reduces this cached shape with a cheap pure map, so it adds
// no cache layer of its own.
func (s *CatalogService) resolveAnimejoyPlaylist(ctx context.Context, animeID string) ([]animejoy.Team, error) {
	if s.animejoyClient == nil {
		return []animejoy.Team{}, nil
	}

	cacheKey := fmt.Sprintf("animejoy:playlist:%s", animeID)
	var cached []animejoy.Team
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	titles := animejoyTitlesFor(anime)
	if len(titles) == 0 {
		_ = s.cache.Set(ctx, cacheKey, []animejoy.Team{}, 3*time.Hour)
		return []animejoy.Team{}, nil
	}

	q := animejoy.Query{
		Titles: titles,
		Year:   anime.Year,
		Season: animejoy.DetectSeason(anime.Name), // best-effort; MVP caveat (defaults to 1)
		Kind:   anime.Kind,
	}

	newsID, err := s.animejoyClient.ResolveNewsID(ctx, q)
	if err != nil {
		s.log.Warnw("animejoy news_id unresolved; returning no playlist",
			"anime_id", animeID, "name", anime.Name, "error", err)
		_ = s.cache.Set(ctx, cacheKey, []animejoy.Team{}, 3*time.Hour)
		return []animejoy.Team{}, nil
	}

	teams, err := s.animejoyClient.FetchPlaylist(ctx, newsID)
	if err != nil {
		s.log.Warnw("animejoy playlist fetch failed; returning no playlist",
			"anime_id", animeID, "news_id", newsID, "error", err)
		_ = s.cache.Set(ctx, cacheKey, []animejoy.Team{}, 3*time.Hour)
		return []animejoy.Team{}, nil
	}

	if teams == nil {
		teams = []animejoy.Team{}
	}
	_ = s.cache.Set(ctx, cacheKey, teams, 3*time.Hour)
	return teams, nil
}

// GetAnimejoyTeams resolves AnimeJoy discovery ONCE for an anime and returns the
// per-leg (Sibnet/AllVideo) team presence shared by both animejoy leg families.
// Best-effort and CACHED (3h, keyed by anime_id) — a discovery miss or error
// yields an empty list, never blocks the capability feed. RU-sub only. The cache
// lives in resolveAnimejoyPlaylist (the expensive network step); mapAnimejoyTeams
// is a cheap pure reduction, so no second cache layer is kept here.
func (s *CatalogService) GetAnimejoyTeams(ctx context.Context, animeID string) (_ []domain.AnimejoyTeam, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animejoy", "get_teams", start, &retErr)

	if s.animejoyClient == nil {
		return []domain.AnimejoyTeam{}, nil
	}

	teams, err := s.resolveAnimejoyPlaylist(ctx, animeID)
	if err != nil {
		return nil, err
	}
	return mapAnimejoyTeams(teams), nil
}

// buildAnimejoyLegInfo reduces the discovery playlist to the per-leg episode +
// team inventory the FE adapter's listEpisodes / listTeams consume. PURE (no
// network): a team "has" the leg when ANY of its episodes carries a non-empty
// leg embed (via animejoy.LegEmbedURL) — only such teams appear in Teams; an
// episode contributes its Num to Episodes only when it carries the leg.
// Episodes is the DISTINCT set across all teams, sorted ascending. Both slices
// are always non-nil (empty, never nil) so the JSON renders [] not null.
func buildAnimejoyLegInfo(teams []animejoy.Team, leg string) domain.AnimejoyLegInfo {
	info := domain.AnimejoyLegInfo{
		Episodes: []int{},
		Teams:    []domain.AnimejoyTeamMeta{},
	}
	seenEp := map[int]bool{}
	for _, t := range teams {
		hasLeg := false
		for _, ep := range t.Episodes {
			if animejoy.LegEmbedURL(ep, leg) == "" {
				continue
			}
			hasLeg = true
			if !seenEp[ep.Num] {
				seenEp[ep.Num] = true
				info.Episodes = append(info.Episodes, ep.Num)
			}
		}
		if hasLeg {
			info.Teams = append(info.Teams, domain.AnimejoyTeamMeta{ID: t.ID, Name: t.Name})
		}
	}
	sort.Ints(info.Episodes)
	return info
}

// GetAnimejoyLegInfo returns the per-leg (sibnet|allvideo) episode + team
// inventory for a title — the source the FE AnimeJoy adapter's listEpisodes /
// listTeams read (the player uses a per-provider episode list, one per leg).
// Best-effort: a nil client or a discovery miss/error yields an EMPTY info
// (non-nil slices), nil error — the FE then shows "no episodes" rather than an
// error. An unknown leg is the one hard error (errors.InvalidInput). Shares the
// cached discovery base with GetAnimejoyTeams / GetAnimejoyStream. RU-sub only.
func (s *CatalogService) GetAnimejoyLegInfo(ctx context.Context, animeID, leg string) (_ *domain.AnimejoyLegInfo, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animejoy", "get_leg_info", start, &retErr)

	empty := &domain.AnimejoyLegInfo{Episodes: []int{}, Teams: []domain.AnimejoyTeamMeta{}}
	if s.animejoyClient == nil {
		return empty, nil
	}
	if leg != "sibnet" && leg != "allvideo" {
		return nil, errors.InvalidInput(fmt.Sprintf("invalid animejoy leg %q", leg))
	}

	teams, err := s.resolveAnimejoyPlaylist(ctx, animeID)
	if err != nil {
		// Best-effort: surface no episodes rather than an error so the FE leg
		// chip degrades cleanly (mirrors resolveAnimejoyPlaylist's own
		// negative-cache-and-return-empty posture for discovery misses).
		return empty, nil
	}

	info := buildAnimejoyLegInfo(teams, leg)
	return &info, nil
}

// pickLegEmbed selects the concrete embed URL for a (leg, episode, teamID)
// request from the discovery playlist. PURE (no network):
//   - leg must be "sibnet" or "allvideo".
//   - if teamID is non-empty it MUST be present, and that team's episode MUST
//     carry the requested leg (no fallback to a different team when a team is
//     explicitly pinned — the user picked it).
//   - otherwise the first team (in playlist order) whose matching episode
//     carries the leg wins.
//
// Episode is matched by Num verbatim. CAVEAT: AnimeJoy numbers per-series, while
// the catalog may pass an absolute number for long-running shows (e.g. One Piece
// 1101+); reconciling absolute-vs-per-series numbering is deferred (MVP matches
// Num as-is). Returns an errors.NotFound AppError if no leg/team/episode matches.
func pickLegEmbed(teams []animejoy.Team, leg string, episode int, teamID string) (string, error) {
	if leg != "sibnet" && leg != "allvideo" {
		return "", errors.NotFound(fmt.Sprintf("animejoy leg %q", leg))
	}
	if len(teams) == 0 {
		return "", errors.NotFound("animejoy team")
	}

	legURL := func(ep animejoy.Episode) string {
		return animejoy.LegEmbedURL(ep, leg)
	}

	if teamID != "" {
		for i := range teams {
			if teams[i].ID != teamID {
				continue
			}
			for _, ep := range teams[i].Episodes {
				if ep.Num == episode {
					if u := legURL(ep); u != "" {
						return u, nil
					}
					return "", errors.NotFound(fmt.Sprintf("animejoy %s embed for team %s episode %d", leg, teamID, episode))
				}
			}
			return "", errors.NotFound(fmt.Sprintf("animejoy episode %d for team %s", episode, teamID))
		}
		return "", errors.NotFound(fmt.Sprintf("animejoy team %s", teamID))
	}

	for i := range teams {
		for _, ep := range teams[i].Episodes {
			if ep.Num == episode {
				if u := legURL(ep); u != "" {
					return u, nil
				}
			}
		}
	}
	return "", errors.NotFound(fmt.Sprintf("animejoy %s embed for episode %d", leg, episode))
}

// GetAnimejoyStream resolves a single playable AnimeJoy leg to a tokened MP4.
// leg is "sibnet" or "allvideo" (the two independent providers each resolve
// ONLY their own leg — no intra-provider failover; the other chip is the user's
// fallback). Flow: shared discovery playlist → pickLegEmbed → ResolveSibnet /
// ResolveAllVideo → streamsign.Sign → AnimejoyStream{Referer}. The Referer is
// the embed/shell origin the proxy MUST replay; the provenance (Exp/Sig)
// authorizes the un-allowlisted CDN host without a static allowlist entry.
func (s *CatalogService) GetAnimejoyStream(ctx context.Context, animeID, leg string, episode int, teamID string) (_ *domain.AnimejoyStream, retErr error) {
	start := time.Now()
	defer metrics.ObserveParser("animejoy", "get_stream", start, &retErr)
	metrics.EpisodeStreamRequestsTotal.WithLabelValues("animejoy-" + leg).Inc()

	if s.animejoyClient == nil {
		return nil, errors.NotFound("animejoy not available")
	}
	if leg != "sibnet" && leg != "allvideo" {
		return nil, errors.InvalidInput(fmt.Sprintf("invalid animejoy leg %q", leg))
	}

	teams, err := s.resolveAnimejoyPlaylist(ctx, animeID)
	if err != nil {
		return nil, err
	}

	embedURL, err := pickLegEmbed(teams, leg, episode, teamID)
	if err != nil {
		return nil, err
	}

	resolved, err := s.animejoyClient.ResolveLeg(ctx, leg, embedURL)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeUnavailable, fmt.Sprintf("animejoy %s leg resolution failed", leg))
	}

	exp, sig, masked := streamsign.Stamp(resolved.URL, resolved.Referer, "mp4")
	return &domain.AnimejoyStream{
		URL:       resolved.URL,
		Type:      "mp4",
		Quality:   resolved.Quality,
		Referer:   resolved.Referer,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Source:    "animejoy",
		Exp:       exp,
		Sig:       sig,
		MaskedURL: masked,
	}, nil
}
