package transport

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func loadedCache(flags map[string]audience, failSafe map[string]string) *rulesetCache {
	c := newRulesetCache(func(ctx context.Context) (rulesetSnapshot, error) {
		return rulesetSnapshot{Flags: flags, FailSafe: failSafe}, nil
	}, logger.Default())
	c.refresh(context.Background())
	return c
}

func TestCanAccess_order(t *testing.T) {
	adminFlag := audience{Roles: []string{"admin"}}
	if !canAccess(adminFlag, "u1", "admin") {
		t.Fatal("admin should access admin flag")
	}
	if canAccess(adminFlag, "u1", "user") {
		t.Fatal("user should NOT access admin-only flag")
	}
	if !canAccess(audience{Roles: []string{"admin"}, AllowUsers: []string{"u1"}}, "u1", "user") {
		t.Fatal("allow-list should grant a non-admin")
	}
	if canAccess(audience{Roles: []string{"admin"}, AllowUsers: []string{"u1"}, DenyUsers: []string{"u1"}}, "u1", "admin") {
		t.Fatal("deny beats allow")
	}
	if !canAccess(audience{Roles: []string{"everyone"}}, "", "") {
		t.Fatal("everyone should allow anonymous")
	}
	if canAccess(audience{Roles: []string{"everyone"}}, "g1", "guest") {
		t.Fatal("guest is never granted")
	}
}

func TestFeatureAllowed_coldStart_failsafe(t *testing.T) {
	empty := newRulesetCache(func(ctx context.Context) (rulesetSnapshot, error) {
		return rulesetSnapshot{}, context.DeadlineExceeded
	}, logger.Default())
	empty.refresh(context.Background()) // stays unloaded
	// cold start, unknown flag → fail-closed to admin-only
	if featureAllowed(empty, "fanfic", "u1", "user") {
		t.Fatal("cold start must fail-closed for a non-admin")
	}
	if !featureAllowed(empty, "fanfic", "a1", "admin") {
		t.Fatal("cold start must still allow admin")
	}
}

func TestFeatureAllowed_loaded(t *testing.T) {
	c := loadedCache(
		map[string]audience{"fanfic": {Roles: []string{"admin"}, AllowUsers: []string{"oronemu"}}},
		map[string]string{"fanfic": "admin"},
	)
	if !featureAllowed(c, "fanfic", "oronemu", "user") {
		t.Fatal("allow-listed user should pass")
	}
	if featureAllowed(c, "fanfic", "rando", "user") {
		t.Fatal("non-listed user should 403")
	}
	// unknown flag with failSafe everyone in snapshot → open
	c2 := loadedCache(map[string]audience{}, map[string]string{"x": "everyone"})
	if !featureAllowed(c2, "x", "", "") {
		t.Fatal("failSafe everyone should allow anonymous for an unlisted flag")
	}
}
