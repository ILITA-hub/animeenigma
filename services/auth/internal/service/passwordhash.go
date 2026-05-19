package service

import "golang.org/x/crypto/bcrypt"

// PasswordHashCost is the bcrypt cost factor used by all password
// hashing in this service. Bumped from DefaultCost (10) to 12 per
// audit Wave 1 (S2). Each increment doubles work; 12 is the OWASP
// 2023 baseline.
const PasswordHashCost = 12

// HashPassword returns a bcrypt hash at the current policy cost.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), PasswordHashCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NeedsRehash returns true when the stored hash was produced with a
// cost factor below the current policy, or is otherwise unparseable
// (in which case the next successful login will replace it).
func NeedsRehash(stored string) bool {
	c, err := bcrypt.Cost([]byte(stored))
	if err != nil {
		return true
	}
	return c < PasswordHashCost
}
