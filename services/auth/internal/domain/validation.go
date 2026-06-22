package domain

import (
	"regexp"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// usernameCharset is the registration username policy: ASCII letters, digits,
// underscore and hyphen. Underscore is allowed because existing service
// accounts use it (e.g. ui_audit_bot, animeenigma_maintenance_bot). This is
// the single source of truth — the struct `validate` tags are not run by
// httputil.Bind (see audit medium "Bind() not BindAndValidate()").
var usernameCharset = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateUsername enforces the registration username policy: 3–32 chars from
// usernameCharset. Returns an InvalidInput (400) error, never a 500.
func ValidateUsername(s string) error {
	if len(s) < 3 || len(s) > 32 {
		return errors.InvalidInput("username must be between 3 and 32 characters")
	}
	if !usernameCharset.MatchString(s) {
		return errors.InvalidInput("username may contain only letters, digits, underscore and hyphen")
	}
	return nil
}

// ValidatePassword enforces 8–72 bytes. 72 is bcrypt's hard input limit:
// longer inputs make bcrypt return an opaque error that surfaced as a generic
// 500 instead of a clean 400 (audit medium "Passwords >72 bytes ...").
func ValidatePassword(s string) error {
	if len(s) < 8 {
		return errors.InvalidInput("password must be at least 8 characters")
	}
	if len(s) > 72 {
		return errors.InvalidInput("password must be at most 72 bytes")
	}
	return nil
}
