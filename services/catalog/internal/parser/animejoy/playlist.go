package animejoy

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// playlistEnvelope is the AJAX response shape:
// {"success":true,"response":"<html …escaped…>"}.
type playlistEnvelope struct {
	Success  bool   `json:"success"`
	Response string `json:"response"`
}

// labelLiRe matches a player-label list item in the playlists-lists block:
//
//	<li data-id="0_1">Sibnet</li>
//
// (no data-file attribute — that distinguishes labels from video items).
var labelLiRe = regexp.MustCompile(`(?is)<li\s+data-id="(\d+)_(\d+)"\s*>([^<]*)</li>`)

// videoLiRe matches a video list item in the playlists-videos block:
//
//	<li data-file="https://fsst.online/embed/726858/" data-id="0_1">1 серия</li>
//
// data-file comes first in the real markup; we require it so labels (which lack
// data-file) never match here.
var videoLiRe = regexp.MustCompile(`(?is)<li\s+data-file="([^"]*)"\s+data-id="(\d+)_(\d+)"\s*>([^<]*)</li>`)

// epNumRe extracts the leading integer from an episode label ("1 серия",
// "23 β серия" → 23, "1101 серия" → 1101 for absolute-numbered series).
var epNumRe = regexp.MustCompile(`(\d+)`)

// parsePlaylist parses the playlist AJAX JSON into teams carrying only the
// Sibnet + AllVideo legs. PURE. Steps:
//  1. JSON-decode the {"response":"<html>"} envelope (this also un-escapes the
//     \/ and \" sequences in the embedded HTML).
//  2. Read the player-label list (group_player → player name).
//  3. Walk the video items, keeping only those whose player resolves to Sibnet
//     or AllVideo, grouping by team (the group index) and episode number.
//
// Episodes that carry neither kept leg are dropped; the rest are returned sorted
// ascending by episode number per team.
func parsePlaylist(body []byte) ([]Team, error) {
	var env playlistEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("animejoy: decode playlist envelope: %w", err)
	}
	if strings.TrimSpace(env.Response) == "" {
		return nil, fmt.Errorf("animejoy: empty playlist response")
	}
	doc := env.Response

	// Player-label map: "<group>_<player>" → normalized leg ("sibnet"/"allvideo"/"").
	// Built from the playlists-lists block; labels are the <li> WITHOUT a
	// data-file, so we scan the whole doc but only labelLiRe (no data-file)
	// matches them. A label whose text isn't a known leg maps to "".
	legByKey := map[string]string{}
	for _, m := range labelLiRe.FindAllStringSubmatch(doc, -1) {
		group, player, name := m[1], m[2], strings.TrimSpace(m[3])
		legByKey[group+"_"+player] = classifyLeg(name)
	}

	// team group index → episode number → *Episode (accumulating legs).
	type teamAcc struct {
		order []int
		byNum map[int]*Episode
	}
	teams := map[string]*teamAcc{}
	teamOrder := []string{}

	for _, m := range videoLiRe.FindAllStringSubmatch(doc, -1) {
		file, group, player, label := m[1], m[2], m[3], m[4]
		leg := legByKey[group+"_"+player]
		if leg == "" {
			// Player isn't Sibnet/AllVideo (or had no label) — drop it.
			continue
		}
		num, ok := parseEpisodeNum(label)
		if !ok {
			// Placeholder rows (e.g. Kodik's "`") have no episode number.
			continue
		}
		fileURL := html.UnescapeString(strings.TrimSpace(file))

		acc, seen := teams[group]
		if !seen {
			acc = &teamAcc{byNum: map[int]*Episode{}}
			teams[group] = acc
			teamOrder = append(teamOrder, group)
		}
		ep, seenEp := acc.byNum[num]
		if !seenEp {
			ep = &Episode{Num: num}
			acc.byNum[num] = ep
			acc.order = append(acc.order, num)
		}
		switch leg {
		case "sibnet":
			if ep.Sibnet == "" {
				ep.Sibnet = fileURL
			}
		case "allvideo":
			if ep.AllVideo == "" {
				ep.AllVideo = fileURL
			}
		}
	}

	out := make([]Team, 0, len(teamOrder))
	for _, group := range teamOrder {
		acc := teams[group]
		eps := make([]Episode, 0, len(acc.order))
		for _, num := range acc.order {
			ep := acc.byNum[num]
			if ep.Sibnet == "" && ep.AllVideo == "" {
				continue
			}
			eps = append(eps, *ep)
		}
		if len(eps) == 0 {
			continue
		}
		sort.Slice(eps, func(i, j int) bool { return eps[i].Num < eps[j].Num })
		out = append(out, Team{ID: group, Episodes: eps})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("animejoy: no Sibnet/AllVideo episodes in playlist")
	}
	return out, nil
}

// classifyLeg maps a player-label name to the canonical leg we keep, or "" for
// players we drop (Kodik, Mail, Dzen, OK, CDA, VK, Alloha, …).
func classifyLeg(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sibnet":
		return "sibnet"
	case "allvideo":
		return "allvideo"
	default:
		return ""
	}
}

// parseEpisodeNum extracts the integer episode number from a label like
// "1 серия" / "23 β серия". Returns ok=false when the label has no leading
// digits (placeholder rows). Absolute numbering (One Piece "1101 серия") is
// preserved verbatim — reconciling absolute vs per-series numbers against the
// catalog is a later-phase concern noted as an open issue.
func parseEpisodeNum(label string) (int, bool) {
	label = html.UnescapeString(strings.TrimSpace(label))
	m := epNumRe.FindString(label)
	if m == "" {
		return 0, false
	}
	return atoiSafe(m), true
}

// FetchPlaylist is the thin HTTP wrapper: GET the playlists.php AJAX endpoint for
// a news_id and parse the response. Caching (3-6h per the spec) is layered on in
// a later phase.
func (c *Client) FetchPlaylist(ctx context.Context, newsID string) ([]Team, error) {
	if strings.TrimSpace(newsID) == "" {
		return nil, fmt.Errorf("animejoy: FetchPlaylist called with empty news_id")
	}
	u := fmt.Sprintf("%s/engine/ajax/playlists.php?news_id=%s&xfield=playlist", c.base(), newsID)
	body, err := c.getBody(ctx, u, map[string]string{"X-Requested-With": "XMLHttpRequest"})
	if err != nil {
		return nil, fmt.Errorf("animejoy: playlist request: %w", err)
	}
	return parsePlaylist(body)
}
