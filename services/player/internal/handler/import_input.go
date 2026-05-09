package handler

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// ExtractMALUsername normalizes a user-supplied MAL identifier.
// Accepts either a bare username or a profile/animelist URL such as:
//   - https://myanimelist.net/profile/Username
//   - https://myanimelist.net/animelist/Username
//   - myanimelist.net/profile/Username
//
// Returns a tagged AppError (CodeInvalidInput) with a human-readable message
// when the input cannot be reduced to a plain username. The error's Details
// map carries machine-readable hints the frontend can use for i18n:
//
//	reason: "empty"|"url_wrong_host"|"url_no_username"|"url_unparseable"|"contains_separator"
//	field:  "mal_username"
//	host:   "..." (for URL-related reasons)
//	input:  truncated original input
func ExtractMALUsername(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", invalidInput("MAL username is required", map[string]string{
			"reason": "empty", "field": "mal_username",
		})
	}

	if !looksLikeURL(s, "myanimelist.") {
		if strings.ContainsAny(s, "/?#@ \t") {
			return "", invalidInput(
				fmt.Sprintf("invalid MAL username %q — paste only your username, not a URL or path", truncate(s, 64)),
				map[string]string{
					"reason": "contains_separator",
					"field":  "mal_username",
					"input":  truncate(s, 64),
				})
		}
		return s, nil
	}

	host, parts, err := parseProfileURL(s)
	if err != nil {
		return "", invalidInput(
			"that looks like a URL but couldn't be parsed — paste only your MAL username, e.g. JohnDoe",
			map[string]string{
				"reason": "url_unparseable",
				"field":  "mal_username",
				"input":  truncate(s, 128),
			})
	}

	if host != "myanimelist.net" {
		return "", invalidInput(
			fmt.Sprintf("that's a %s URL, not a MyAnimeList one — paste only your MAL username", host),
			map[string]string{
				"reason": "url_wrong_host",
				"field":  "mal_username",
				"host":   host,
				"input":  truncate(s, 128),
			})
	}

	// /profile/<USERNAME>, /animelist/<USERNAME>, /<USERNAME> (some legacy paths)
	if len(parts) >= 2 && (parts[0] == "profile" || parts[0] == "animelist") && parts[1] != "" {
		return parts[1], nil
	}
	if len(parts) == 1 && parts[0] != "" {
		// e.g. https://myanimelist.net/Username — accept it just in case
		return parts[0], nil
	}

	return "", invalidInput(
		"couldn't find a username in that URL — paste only your MAL username, e.g. JohnDoe",
		map[string]string{
			"reason": "url_no_username",
			"field":  "mal_username",
			"host":   host,
			"input":  truncate(s, 128),
		})
}

// ExtractShikimoriNickname normalizes a user-supplied Shikimori identifier.
// Accepts either a bare nickname or a profile URL such as:
//   - https://shikimori.one/Nickname
//   - https://shikimori.me/Nickname/list/anime
//   - shikimori.one/Nickname
func ExtractShikimoriNickname(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", invalidInput("Shikimori nickname is required", map[string]string{
			"reason": "empty", "field": "shikimori_nickname",
		})
	}

	if !looksLikeURL(s, "shikimori.") {
		if strings.ContainsAny(s, "/?#@ \t") {
			return "", invalidInput(
				fmt.Sprintf("invalid Shikimori nickname %q — paste only your nickname, not a URL or path", truncate(s, 64)),
				map[string]string{
					"reason": "contains_separator",
					"field":  "shikimori_nickname",
					"input":  truncate(s, 64),
				})
		}
		return s, nil
	}

	host, parts, err := parseProfileURL(s)
	if err != nil {
		return "", invalidInput(
			"that looks like a URL but couldn't be parsed — paste only your Shikimori nickname",
			map[string]string{
				"reason": "url_unparseable",
				"field":  "shikimori_nickname",
				"input":  truncate(s, 128),
			})
	}

	if !strings.HasPrefix(host, "shikimori.") {
		return "", invalidInput(
			fmt.Sprintf("that's a %s URL, not a Shikimori one — paste only your Shikimori nickname", host),
			map[string]string{
				"reason": "url_wrong_host",
				"field":  "shikimori_nickname",
				"host":   host,
				"input":  truncate(s, 128),
			})
	}

	if len(parts) >= 1 && parts[0] != "" {
		return parts[0], nil
	}

	return "", invalidInput(
		"couldn't find a nickname in that URL — paste only your Shikimori nickname",
		map[string]string{
			"reason": "url_no_username",
			"field":  "shikimori_nickname",
			"host":   host,
			"input":  truncate(s, 128),
		})
}

// invalidInput wraps errors.InvalidInput with a details map, honoring the
// existing WithDetail (singular) API of the libs/errors package.
func invalidInput(message string, details map[string]string) error {
	e := errors.InvalidInput(message)
	for k, v := range details {
		e = e.WithDetail(k, v)
	}
	return e
}

// looksLikeURL returns true when input has a scheme, is host-prefixed (e.g.
// "myanimelist.net/..."), or starts with the supplied bare-host prefix
// ("myanimelist.", "shikimori.").
func looksLikeURL(s, hostPrefix string) bool {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "://") {
		return true
	}
	if strings.HasPrefix(lower, "www.") {
		return true
	}
	if strings.HasPrefix(lower, hostPrefix) {
		return true
	}
	return false
}

// parseProfileURL adds a scheme if missing, parses, and returns lower-cased
// host (with leading "www." stripped) plus path segments.
func parseProfileURL(raw string) (string, []string, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, err
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	if host == "" {
		return "", nil, fmt.Errorf("missing host")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		parts = nil
	}
	return host, parts, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
