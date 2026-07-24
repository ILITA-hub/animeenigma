package cache

import "testing"

// TestKeyClass asserts the classifier maps raw cache keys to a BOUNDED, stable
// class set: variable IDs are stripped, related/similar stay distinct (D-07),
// and unknown prefixes collapse into a single "other" bucket so the class set
// (which keys the aggregator map) can never grow with variable IDs (T-03-10).
func TestKeyClass(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		// anime sub-namespaces — each a distinct class.
		{"anime detail", "anime:42", "anime:detail"},
		{"anime detail uuid", "anime:550e8400-e29b-41d4-a716-446655440000", "anime:detail"},
		{"anime list", "anime:list:genre=action:p:1:l:20", "anime:list"},
		{"anime top", "anime:top:trending", "anime:top"},
		// related vs similar MUST stay separate (D-07).
		{"anime related", "anime:related:42", "anime:related"},
		{"anime similar", "anime:similar:42", "anime:similar"},
		// other prefixed sub-namespaces.
		{"user profile", "user:profile:99", "user:profile"},
		{"search", "search:naruto:1", "search"},
		{"progress", "progress:u1:a2", "progress"},
		{"video manifest", "video:manifest:42", "video:manifest"},
		{"extid", "extid:mal:42", "extid"},
		// prefix-only classes (collapse to the bare prefix-level class).
		{"episode", "episode:a1:3", "episode"},
		{"session", "session:abc123", "session"},
		{"genre with raw id", "genre:5", "genre"},
		{"studio", "studio:7", "studio"},
		{"ratelimit", "ratelimit:login:1.2.3.4", "ratelimit"},
		{"room", "room:xyz", "room"},
		{"tgauth", "tgauth:token123", "tgauth"},
		// unknown prefix → single "other" bucket.
		{"unknown prefix", "weirdkey:foo", "other"},
		{"another unknown", "totallynew:1:2:3", "other"},
		{"no colon at all", "bareword", "other"},
		{"empty", "", "other"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := KeyClass(tc.raw); got != tc.want {
				t.Fatalf("KeyClass(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestKeyClassBounded asserts the class set never grows with variable IDs: a
// thousand distinct anime ids all collapse to the same "anime:detail" class.
func TestKeyClassBounded(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		seen[KeyClass("anime:"+itoa(i))] = struct{}{}
	}
	if len(seen) != 1 {
		t.Fatalf("expected 1 class for 1000 distinct anime ids, got %d: %v", len(seen), seen)
	}
	if _, ok := seen["anime:detail"]; !ok {
		t.Fatalf("expected the single class to be anime:detail, got %v", seen)
	}
}

// itoa is a tiny dependency-free int->string to keep the test stdlib-light.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestKeyCertLogin(t *testing.T) {
	if got := KeyCertLogin("abc"); got != "certlogin:abc" {
		t.Fatalf("KeyCertLogin = %q", got)
	}
}

func TestKeyWebAuthnCeremony(t *testing.T) {
	if got := KeyWebAuthnCeremony("abc"); got != "webauthn:abc" {
		t.Fatalf("KeyWebAuthnCeremony = %q", got)
	}
}
