package service

import "strings"

// SanitizeOldURL constrains a caller-supplied return path to a safe SAME-ORIGIN
// relative path, defeating open-redirect. It must start with a single '/', must
// not start with '//' or '/\' (protocol-relative), must contain no scheme and no
// ASCII control chars. Anything else collapses to "/".
func SanitizeOldURL(raw string) string {
	if raw == "" || raw[0] != '/' {
		return "/"
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return "/"
	}
	if strings.Contains(raw, "://") {
		return "/"
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	return raw
}
