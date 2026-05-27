package repo

// Relevance predicate fragments shared by the read path (List, UnreadCount)
// and the hourly RelevanceInvalidationJob, so the "is this notification still
// relevant?" rule lives in exactly ONE place.
//
// PORTABILITY: every construct here is valid on BOTH Postgres (prod) and
// SQLite (tests). Uses the `->>` JSON operator and standard-SQL CAST — never
// `::int`/`::text`. References the outer row via the literal table name
// `user_notifications` so the fragment works inside both a SELECT (List,
// Model(&UserNotification{})) and an UPDATE on the same table.
//
// A new_episode notification is RELEVANT iff:
//
//	(1) the user still has the anime as anime_list.status = 'watching', AND
//	(2) the user's max watched episode for the anime (ANY combo) is below the
//	    advertised latest_available_episode (fail-open when that field is NULL).
const relevantBodySQL = `
EXISTS (
	SELECT 1 FROM anime_list al
	WHERE al.user_id = user_notifications.user_id
	  AND CAST(al.anime_id AS TEXT) = (user_notifications.payload ->> 'anime_id')
	  AND al.status = 'watching'
)
AND (
	CAST((user_notifications.payload ->> 'latest_available_episode') AS INTEGER) IS NULL
	OR COALESCE((
		SELECT MAX(wh.episode_number) FROM watch_history wh
		WHERE wh.user_id = user_notifications.user_id
		  AND CAST(wh.anime_id AS TEXT) = (user_notifications.payload ->> 'anime_id')
	), -1) < CAST((user_notifications.payload ->> 'latest_available_episode') AS INTEGER)
)`

// relevanceReadClause is the WHERE fragment added to user-facing reads. Rows
// of a non-new_episode type always pass (future types are filtered by their
// own logic, not this one).
func relevanceReadClause() string {
	return `(user_notifications.type <> 'new_episode' OR (` + relevantBodySQL + `))`
}

// NotRelevantClause matches new_episode rows that are NO LONGER relevant —
// used by the invalidation job's UPDATE ... WHERE.
func NotRelevantClause() string {
	return `user_notifications.type = 'new_episode' AND NOT (` + relevantBodySQL + `)`
}
