package cache

import (
	"fmt"
	"time"
)

// TTL strategies for different data types in anime streaming service
var (
	// Short-lived cache for frequently changing data
	TTLSession     = 24 * time.Hour
	TTLRateLimit   = 1 * time.Minute
	TTLUserOnline    = 5 * time.Minute
	TTLTelegramAuth  = 5 * time.Minute
	// One-time cross-domain SSO handoff token. Deliberately tiny — the token
	// rides in a URL during a single redirect chain.
	TTLXDomainMagic = 60 * time.Second

	// Medium-lived cache for moderately stable data
	TTLAnimeList      = 1 * time.Hour
	TTLSearchResults  = 15 * time.Minute
	TTLUserProfile    = 30 * time.Minute
	TTLWatchProgress  = 10 * time.Minute
	TTLEpisodeList    = 1 * time.Hour

	// Long-lived cache for stable data
	TTLAnimeDetails   = 6 * time.Hour
	// Ongoing anime change often (episodes_aired / next_episode_at advance as
	// episodes air), so cache their detail row briefly — airing data self-heals
	// within minutes even if an invalidation is ever missed.
	TTLOngoingAnimeDetails = 15 * time.Minute
	TTLTopAnime       = 24 * time.Hour
	TTLGenreList      = 24 * time.Hour
	TTLStudioList     = 24 * time.Hour
	TTLVideoManifest  = 12 * time.Hour
	TTLExternalIDs    = 7 * 24 * time.Hour  // MAL/Shikimori ID mappings

	// Very long cache for rarely changing data
	TTLStaticContent = 30 * 24 * time.Hour
)

// Key prefixes for cache organization
const (
	PrefixAnime        = "anime:"
	PrefixEpisode      = "episode:"
	PrefixUser         = "user:"
	PrefixSession      = "session:"
	PrefixSearch       = "search:"
	PrefixProgress     = "progress:"
	PrefixVideo        = "video:"
	PrefixGenre        = "genre:"
	PrefixStudio       = "studio:"
	PrefixCharacter    = "character:"
	PrefixExternalID   = "extid:"
	PrefixRateLimit    = "ratelimit:"
	PrefixRoom         = "room:"
	PrefixTelegramAuth = "tgauth:"
	PrefixXDomainMagic = "xdomain:"
)

// Key builders for consistent cache key formatting
func KeyAnime(id string) string {
	return PrefixAnime + id
}

func KeyAnimeList(page, limit int, filters string) string {
	return fmt.Sprintf("%slist:%s:%d:%d", PrefixAnime, filters, page, limit)
}

func KeyEpisode(animeID, episodeNum string) string {
	return PrefixEpisode + animeID + ":" + episodeNum
}

func KeyUserProfile(userID string) string {
	return PrefixUser + "profile:" + userID
}

func KeyUserSession(sessionID string) string {
	return PrefixSession + sessionID
}

func KeySearchResults(query string, page int) string {
	return fmt.Sprintf("%s%s:%d", PrefixSearch, query, page)
}

func KeyWatchProgress(userID, animeID string) string {
	return PrefixProgress + userID + ":" + animeID
}

func KeyVideoManifest(videoID string) string {
	return PrefixVideo + "manifest:" + videoID
}

func KeyExternalID(source, externalID string) string {
	return PrefixExternalID + source + ":" + externalID
}

func KeyRateLimit(identifier, action string) string {
	return PrefixRateLimit + action + ":" + identifier
}

func KeyRoom(roomID string) string {
	return PrefixRoom + roomID
}

func KeyTelegramAuth(token string) string {
	return PrefixTelegramAuth + token
}

func KeyTopAnime() string {
	return PrefixAnime + "top:trending"
}

func KeyRelatedAnime(shikimoriID string) string {
	return PrefixAnime + "related:" + shikimoriID
}

// KeySimilarAnime is the cache key for Shikimori /similar lookups.
// Phase 13 (REC-SIG-06) — sibling of KeyRelatedAnime. TTL = TTLAnimeDetails (6h).
func KeySimilarAnime(shikimoriID string) string {
	return PrefixAnime + "similar:" + shikimoriID
}

// KeyAnimeCharacters is the cache key for an anime's character list.
// TTL = TTLAnimeDetails (6h). Mirrors KeyRelatedAnime / KeySimilarAnime.
func KeyAnimeCharacters(animeID string) string {
	return PrefixAnime + "characters:" + animeID
}

// KeyCharacter is the cache key for a single character's detail row,
// keyed by Shikimori character id. TTL = TTLAnimeDetails (6h).
func KeyCharacter(shikimoriID string) string {
	return PrefixCharacter + shikimoriID
}
