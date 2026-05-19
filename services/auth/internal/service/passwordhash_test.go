package service

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPasswordUsesPolicyCost(t *testing.T) {
	h, err := HashPassword("hunter2hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(h))
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	if cost != PasswordHashCost {
		t.Fatalf("got cost %d, want %d", cost, PasswordHashCost)
	}
}

func TestNeedsRehash(t *testing.T) {
	weak, _ := bcrypt.GenerateFromPassword([]byte("p"), 10)
	strong, _ := bcrypt.GenerateFromPassword([]byte("p"), PasswordHashCost)

	if !NeedsRehash(string(weak)) {
		t.Fatal("cost=10 should need rehash")
	}
	if NeedsRehash(string(strong)) {
		t.Fatal("policy-cost hash must not need rehash")
	}
	if !NeedsRehash("not-a-bcrypt-hash") {
		t.Fatal("invalid hash should be treated as needing rehash")
	}
}
