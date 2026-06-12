package domain

import (
	"testing"

	"github.com/lib/pq"
)

func TestValidActivityVisibility(t *testing.T) {
	for _, v := range []string{ActivityVisibilityAll, ActivityVisibilityNonHentai, ActivityVisibilityNone} {
		if !ValidActivityVisibility(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range []string{"", "hide_all", "hentai", "ALL", "none "} {
		if ValidActivityVisibility(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// ToPublic must blank public_statuses when activity is fully hidden, and must
// never expose the activity_visibility value itself (PublicUser has no such
// field — compile-time guarantee; here we check the statuses behaviour).
func TestToPublicActivityVisibility(t *testing.T) {
	u := &User{
		ID:                 "u1",
		Username:           "alice",
		PublicStatuses:     pq.StringArray{"watching", "completed"},
		ActivityVisibility: ActivityVisibilityNone,
	}
	if got := u.ToPublic().PublicStatuses; len(got) != 0 {
		t.Errorf("expected empty public_statuses for visibility=none, got %v", got)
	}

	u.ActivityVisibility = ActivityVisibilityNonHentai
	if got := u.ToPublic().PublicStatuses; len(got) != 2 {
		t.Errorf("expected statuses preserved for visibility=non_hentai, got %v", got)
	}

	u.ActivityVisibility = ActivityVisibilityAll
	if got := u.ToPublic().PublicStatuses; len(got) != 2 {
		t.Errorf("expected statuses preserved for visibility=all, got %v", got)
	}
}
