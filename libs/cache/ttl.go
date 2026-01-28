package cache

import "time"

// TTL strategies for different data types in anime streaming service
var (
	// Short-lived cache for frequently changing data
	TTLSession     = 24 * time.Hour
	TTLRateLimit   = 1 * time.Minute
	TTLUserOnline  = 5 * time.Minute

	// Medium-lived cache for moderately stable data
	TTLAnimeList      = 1 * time.Hour
	TTLSearchResults  = 15 * time.Minute
	TTLUserProfile    = 30 * time.Minute
	TTLWatchProgress  = 10 * time.Minute
	TTLEpisodeList    = 1 * time.Hour

	// Long-lived cache for stable data
	TTLAnimeDetails   = 6 * time.Hour
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
	PrefixExternalID   = "extid:"
	PrefixRateLimit    = "ratelimit:"
	PrefixRoom         = "room:"
)

// Key builders for consistent cache key formatting
func KeyAnime(id string) string {
	return PrefixAnime + id
}

func KeyAnimeList(page, limit int, filters string) string {
	return PrefixAnime + "list:" + filters + ":" + string(rune(page)) + ":" + string(rune(limit))
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
	return PrefixSearch + query + ":" + string(rune(page))
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
