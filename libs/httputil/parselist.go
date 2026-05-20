package httputil

import "strings"

// ParseCommaList parses a comma-separated string into a slice of trimmed,
// non-empty elements. It is intended for env-var configuration values such
// as CORS_ORIGINS, ALLOWED_WS_ORIGINS, or PROXY_ALLOWED_DOMAINS where bare
// strings.Split misbehaves on empty input (returning [""] — a single empty
// element — instead of an empty slice).
//
// Behavior:
//   - empty / whitespace-only input returns nil
//   - leading/trailing whitespace on each element is trimmed
//   - empty elements (from leading/trailing/internal commas) are dropped
func ParseCommaList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
