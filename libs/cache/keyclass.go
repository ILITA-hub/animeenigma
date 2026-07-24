package cache

import "strings"

// KeyClass maps a raw cache key to a stable, BOUNDED class label derived from the
// ttl.go prefix + key-builder taxonomy (D-07). It strips variable IDs so the
// class set never grows with cardinality — this matters because the class keys
// the in-process aggregator map, and an unbounded class set is a memory-exhaustion
// vector (T-03-10). Any prefix not in the known taxonomy collapses to "other".
//
// The classifier keeps prefix + (where the builders define one) the first
// sub-namespace token, and drops everything from the first variable ID onward.
// anime:related and anime:similar are deliberately KEPT distinct — D-07 calls out
// their very different hit rates, so they must not collapse into one anime class.
func KeyClass(raw string) string {
	if raw == "" {
		return classOther
	}

	// Split off the prefix (first ":" segment). A key with no ":" has no known
	// prefix and is bucketed as "other".
	prefix, rest, hasColon := strings.Cut(raw, ":")
	if !hasColon {
		return classOther
	}

	switch prefix + ":" {
	case PrefixAnime:
		// anime keys carry a sub-namespace that selects the class; a bare
		// anime:<id> (the KeyAnime builder) is the detail class.
		sub, _, _ := strings.Cut(rest, ":")
		switch sub {
		case "list":
			return "anime:list"
		case "top":
			return "anime:top"
		case "related":
			return "anime:related"
		case "similar":
			return "anime:similar"
		default:
			// anime:<id> — the bare-id detail row.
			return "anime:detail"
		}
	case PrefixUser:
		// user:profile:<id> is the only user sub-namespace today; collapse any
		// other user:<sub> to a stable "user:<sub>"-free bucket via profile when
		// it is the profile builder, else the bare prefix class.
		sub, _, _ := strings.Cut(rest, ":")
		if sub == "profile" {
			return "user:profile"
		}
		return "user"
	case PrefixVideo:
		sub, _, _ := strings.Cut(rest, ":")
		if sub == "manifest" {
			return "video:manifest"
		}
		return "video"
	case PrefixEpisode:
		return "episode"
	case PrefixSession:
		return "session"
	case PrefixSearch:
		return "search"
	case PrefixProgress:
		return "progress"
	case PrefixGenre:
		return "genre"
	case PrefixStudio:
		return "studio"
	case PrefixExternalID:
		return "extid"
	case PrefixRateLimit:
		return "ratelimit"
	case PrefixRoom:
		return "room"
	case PrefixTelegramAuth:
		return "tgauth"
	case PrefixXDomainMagic:
		return "xdomain"
	default:
		return classOther
	}
}

// KeyXDomainMagic is the Redis key for a one-time cross-domain SSO handoff token.
func KeyXDomainMagic(token string) string {
	return PrefixXDomainMagic + token
}

// KeyCertLogin is the Redis key for a one-time TLS-cert login handoff token
// (minted by /cert/handshake-login, consumed by /api/auth/cert/consume).
func KeyCertLogin(token string) string {
	return "certlogin:" + token
}

// KeyWebAuthnCeremony is the Redis key for in-flight WebAuthn ceremony state
// (registration or login), keyed by a random ceremony id.
func KeyWebAuthnCeremony(id string) string {
	return "webauthn:" + id
}

// classOther is the single catch-all bucket for unknown prefixes. Keeping it a
// named constant makes the bounded-set intent explicit and greppable.
const classOther = "other"
