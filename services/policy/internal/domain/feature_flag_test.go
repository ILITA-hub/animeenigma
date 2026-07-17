package domain

import "testing"

func flag(roles, allow, deny []string) FeatureFlag {
	return FeatureFlag{Key: "f", Roles: roles, AllowUsers: allow, DenyUsers: deny}
}

func TestCanAccess(t *testing.T) {
	cases := []struct {
		name         string
		f            FeatureFlag
		userID, role string
		want         bool
	}{
		{"admin flag, admin user", flag([]string{RoleAdmin}, nil, nil), "u1", RoleAdmin, true},
		{"admin flag, normal user", flag([]string{RoleAdmin}, nil, nil), "u1", RoleUser, false},
		{"admin flag, allow-listed user wins", flag([]string{RoleAdmin}, []string{"u1"}, nil), "u1", RoleUser, true},
		{"deny beats allow", flag([]string{RoleAdmin}, []string{"u1"}, []string{"u1"}), "u1", RoleAdmin, false},
		{"everyone flag, anonymous", flag([]string{RoleEveryone}, nil, nil), "", "", true},
		{"everyone flag, guest still denied", flag([]string{RoleEveryone}, nil, nil), "g1", RoleGuest, false},
		{"user flag, allow-listed guest still denied", flag([]string{RoleUser}, []string{"g1"}, nil), "g1", RoleGuest, false},
		{"empty audience denies", flag(nil, nil, nil), "u1", RoleUser, false},
		{"user flag, user role", flag([]string{RoleUser}, nil, nil), "u1", RoleUser, true},
		{"user flag, anonymous denied", flag([]string{RoleUser}, nil, nil), "", "", false},
		// Librarian normalizes to user for flag evaluation: keeps user-tier
		// features, gains no admin-tier ones.
		{"user flag, librarian treated as user", flag([]string{RoleUser}, nil, nil), "u1", RoleLibrarian, true},
		{"admin flag, librarian denied", flag([]string{RoleAdmin}, nil, nil), "u1", RoleLibrarian, false},
		{"deny-list beats librarian normalization", flag([]string{RoleUser}, nil, []string{"u1"}), "u1", RoleLibrarian, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.f.CanAccess(c.userID, c.role); got != c.want {
				t.Fatalf("CanAccess(%q,%q) = %v, want %v", c.userID, c.role, got, c.want)
			}
		})
	}
}
