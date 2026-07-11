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
	// nekostream.site — gogoanime's megaplay wrapper resolves some anime/
	// episode combos to this host, which serves a valid-HTTP-200 1x1 PNG
	// (dressed up with a video/mp2t Content-Type and megabytes of trailing
	// padding) instead of real video. Confirmed independently 3x across
	// 2026-07-06 / 2026-07-09 / 2026-07-11 (see
	// docs/issues/provider-recovery-log.md) on multiple anime/episodes each
	// time — a stable dead mirror, not a transient blip. Probe never decodes
	// segment bytes (a fake CDN answering 200 is otherwise indistinguishable
	// from a real one to this cheap primitive), so blocklisting the host is
	// the only lever available here.
	"nekostream.site",
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
