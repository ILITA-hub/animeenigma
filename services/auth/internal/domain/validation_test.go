package domain

import (
	"strings"
	"testing"
)

func TestValidateUsername(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"too short", "ab", false},
		{"too long", strings.Repeat("a", 33), false},
		{"embedded space", "bad name", false},
		{"injection chars", "a';drop", false},
		{"unicode", "пользователь", false},
		{"plain alnum", "user123", true},
		{"underscore (service accounts like ui_audit_bot)", "ui_audit_bot", true},
		{"hyphen", "co-watcher", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateUsername(c.in)
			if c.ok && err != nil {
				t.Fatalf("ValidateUsername(%q) = %v, want nil", c.in, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("ValidateUsername(%q) = nil, want error", c.in)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"too short", "short", false},
		{"min length 8", "12345678", true},
		{"exactly 72 bytes", strings.Repeat("a", 72), true},
		// 73 bytes previously produced an opaque wrapped bcrypt 500 instead of 400.
		{"73 bytes rejected", strings.Repeat("a", 73), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePassword(c.in)
			if c.ok && err != nil {
				t.Fatalf("ValidatePassword(len=%d) = %v, want nil", len(c.in), err)
			}
			if !c.ok && err == nil {
				t.Fatalf("ValidatePassword(len=%d) = nil, want error", len(c.in))
			}
		})
	}
}
