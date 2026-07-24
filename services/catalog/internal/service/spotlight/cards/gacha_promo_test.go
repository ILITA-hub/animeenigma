package cards

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

func TestGachaPromoResolver_Type(t *testing.T) {
	if got := NewGachaPromoResolver().Type(); got != "gacha_promo" {
		t.Fatalf("Type() = %q, want gacha_promo", got)
	}
}

// The promo card is static: always eligible, pinned priority, locked
// economy constants in the payload.
func TestGachaPromoResolver_Resolve(t *testing.T) {
	card, err := NewGachaPromoResolver().Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("Resolve returned nil card — promo must always be eligible")
	}
	if card.Type != "gacha_promo" {
		t.Errorf("card.Type = %q, want gacha_promo", card.Type)
	}
	// >= 2.0 is the frontend's PINNED_PRIORITY_MIN — the always-first contract.
	if card.Priority < 2.0 {
		t.Errorf("card.Priority = %v, want >= 2.0 (pinned)", card.Priority)
	}

	data, ok := card.Data.(spotlight.GachaPromoData)
	if !ok {
		t.Fatalf("card.Data is %T, want spotlight.GachaPromoData", card.Data)
	}
	if data.PullCostSingle != 100 || data.PullCostTen != 900 {
		t.Errorf("pull costs = %d/%d, want 100/900", data.PullCostSingle, data.PullCostTen)
	}
	if data.PitySSRAt != 90 {
		t.Errorf("PitySSRAt = %d, want 90", data.PitySSRAt)
	}
}

// A logged-in user gets the identical card — the promo is global.
func TestGachaPromoResolver_Resolve_UserIndependent(t *testing.T) {
	uid := "some-user"
	anon, _ := NewGachaPromoResolver().Resolve(context.Background(), nil)
	authed, _ := NewGachaPromoResolver().Resolve(context.Background(), &uid)
	if anon == nil || authed == nil {
		t.Fatal("both variants must resolve")
	}
	if *anon != *authed {
		t.Errorf("card differs by user: anon=%+v authed=%+v", *anon, *authed)
	}
}
