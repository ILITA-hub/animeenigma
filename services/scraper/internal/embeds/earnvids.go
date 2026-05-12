// earnvids.go — EarnvidsExtractor for otakuvid.online wrapper pages.
//
// SCRAPER-9ANI-03 / Plan 18-03. Earnvids and StreamHG are structurally
// identical: both wrap their playable HLS URL inside a Dean-Edwards-packed
// IIFE that, when unpacked, contains a `{"hls2":"https://...m3u8?..."}` JSON
// literal. The two providers differ only by:
//
//   - host allowlist:   ["otakuvid.online"] (vs ["otakuhg.site"])
//   - upstream Referer: "https://otakuvid.online/" (vs "https://otakuhg.site/")
//   - CDN host:         dramiyos-cdn.com (vs premilkyway.com — visible only
//                       in the returned m3u8 URL; not validated here)
//   - failure selectors: namespaced "earnvids_*" instead of "streamhg_*"
//
// All extraction machinery (fetch, packer locate, goja unpack with watchdog,
// hls2 regex) is delegated to the shared *packedExtractor base in
// packed_common.go.
package embeds

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// earnvidsHosts is the case-insensitive allowlist for Earnvids.
var earnvidsHosts = []string{"otakuvid.online"}

// earnvidsReferer is the upstream Referer header the CDN requires for
// segment fetches.
const earnvidsReferer = "https://otakuvid.online/"

// EarnvidsExtractor composes *packedExtractor configured for otakuvid.online.
type EarnvidsExtractor struct {
	*packedExtractor
}

// NewEarnvidsExtractor constructs the extractor with HTTP=15s, goja=5s —
// matches StreamHG and the Kwik extractor (HTTP wider than the JS budget so
// slow networks don't starve the goja runtime).
func NewEarnvidsExtractor() *EarnvidsExtractor {
	base := &packedExtractor{
		name:               "earnvids",
		hosts:              earnvidsHosts,
		referer:            earnvidsReferer,
		selectorPackerFail: "earnvids_packer_balance",
		selectorGojaFail:   "earnvids_goja",
		selectorRegexFail:  "earnvids_hls2_regex",
		selectorBodyFail:   "earnvids_body_read",
		http:               &http.Client{Timeout: defaultPackedHTTPTimeout},
		timeout:            defaultPackedGojaTimeout,
	}
	return &EarnvidsExtractor{packedExtractor: base}
}

// Name overrides the embedded Name() for explicit clarity at the call site.
func (e *EarnvidsExtractor) Name() string { return "earnvids" }

// Compile-time assertion: EarnvidsExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*EarnvidsExtractor)(nil)
