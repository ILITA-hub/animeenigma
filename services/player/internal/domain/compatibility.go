package domain

// UserListEntry is the minimal projection used by the compatibility blend.
type UserListEntry struct {
	AnimeID  string
	Score    int      // 0 = unrated
	GenreIDs []string
}

// CompatibilityResult is returned by GET /users/{userId}/compatibility.
type CompatibilityResult struct {
	Percent      int      `json:"percent"`
	SharedCount  int      `json:"shared_count"`
	SharedSample []string `json:"shared_sample"` // up to 8 shared anime IDs
}
