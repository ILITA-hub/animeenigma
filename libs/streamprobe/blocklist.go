package streamprobe

import "strings"

// TODO(spec §4.1.c-TODO, Redis-lift trigger):
//
//	When this slice grows past ~10 entries OR the maintenance bot needs
//	to extend it without a redeploy, lift into Redis at key
//	`scraper:streamprobe:blocklist` (sorted set or list of suffixes).
//	The Redis path takes precedence; this hardcoded slice becomes the
//	bootstrap default loaded once at scraper startup if Redis is empty.
//	Tracked: docs/plans/2026-05-13-scraper-self-healing-spec.md §4.1.c-TODO.
var adCDNHostSuffixes = []string{
	"ibyteimg.com",
	"p16-ad-sg",    // matches p16-ad-sg.* (TikTok ad CDN region tag)
	"ad-site-i18n", // matches *.ad-site-i18n.* (TikTok i18n ad CDN)
	"tiktokcdn.com",
}

// isAdCDNHost reports whether host matches any blocklisted suffix.
// Case-insensitive. Empty host returns false.
//
// Implementation note: uses strings.Contains rather than a pure suffix
// match because `p16-ad-sg` appears as a HOSTNAME PREFIX in production
// poison (`p16-ad-sg.ibyteimg.com`), not a suffix. Contains catches both
// the prefix-style and the suffix-style entries in the same loop.
func isAdCDNHost(host string) bool {
	if host == "" {
		return false
	}
	h := strings.ToLower(host)
	for _, suf := range adCDNHostSuffixes {
		s := strings.ToLower(suf)
		if h == s || strings.HasSuffix(h, "."+s) || strings.Contains(h, s) {
			return true
		}
	}
	return false
}
