package job

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// checkedTTL bounds notif:checked growth and lets a title that stops being
// scanned age back to "never checked" (fail-safe re-check). Comfortably above
// any tier floor.
const checkedTTL = 48 * time.Hour

func checkedKey(animeID string) string { return "notif:checked:" + animeID }

// CheckedStore records the last time each anime's combos were parser-checked,
// so the detector can tier its per-run candidate set and still guarantee a
// delivery floor. All reads fail open (error → empty map = "check them").
type CheckedStore struct {
	rdb *redis.Client
	now func() time.Time
}

// NewCheckedStore constructs the store over the service's Redis client.
func NewCheckedStore(rdb *redis.Client) *CheckedStore {
	return &CheckedStore{rdb: rdb, now: time.Now}
}

// LastChecked returns the last-checked time per anime id that has one. Ids
// absent from the result were never checked (or expired). Fail-open: any
// Redis error yields an empty map, so the caller checks everything.
func (s *CheckedStore) LastChecked(ctx context.Context, animeIDs []string) map[string]time.Time {
	out := map[string]time.Time{}
	if len(animeIDs) == 0 {
		return out
	}
	keys := make([]string, len(animeIDs))
	for i, id := range animeIDs {
		keys[i] = checkedKey(id)
	}
	vals, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		str, ok := v.(string)
		if !ok {
			continue
		}
		sec, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			continue
		}
		out[animeIDs[i]] = time.Unix(sec, 0)
	}
	return out
}

// MarkChecked stamps each anime id as checked now (TTL checkedTTL). Best-effort.
func (s *CheckedStore) MarkChecked(ctx context.Context, animeIDs []string) {
	if len(animeIDs) == 0 {
		return
	}
	now := strconv.FormatInt(s.now().Unix(), 10)
	pipe := s.rdb.Pipeline()
	for _, id := range animeIDs {
		pipe.Set(ctx, checkedKey(id), now, checkedTTL)
	}
	_, _ = pipe.Exec(ctx)
}
