// streamhg.go — StreamHGExtractor for otakuhg.site wrapper pages.
//
// SCRAPER-9ANI-03 / Plan 18-03. StreamHG embeds the playable HLS URL inside
// a Dean-Edwards-packed `eval(function(p,a,c,k,e,d){...}(...))` IIFE that,
// when unpacked, contains a JSON-shape literal `{"hls2":"https://...m3u8?..."}`.
//
// All extraction machinery (HTTP fetch, body cap, packer locate, goja unpack
// with watchdog, hls2 regex) lives in the shared packedExtractor base in
// packed_common.go. This file just configures the StreamHG-specific knobs:
//
//   - allowlist:           ["otakuhg.site"]
//   - upstream Referer:    "https://otakuhg.site/"
//   - failure selectors:   "streamhg_packer_balance", "streamhg_hls2_regex",
//                          "streamhg_body_read"
//
// The split-file design (one composed file per provider, shared base in
// packed_common.go) makes adding the next packed-style provider a ~40-line
// change — see earnvids.go for the parallel implementation.
package embeds

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// streamhgHosts is the case-insensitive allowlist for StreamHG.
var streamhgHosts = []string{"otakuhg.site"}

// streamhgReferer is the upstream Referer header the CDN requires for
// segment fetches.
const streamhgReferer = "https://otakuhg.site/"

// StreamHGExtractor composes *packedExtractor configured for otakuhg.site.
// Name() returns "streamhg" so logs/metrics distinguish this extractor from
// the parallel earnvids one even though they share the unpack pipeline.
type StreamHGExtractor struct {
	*packedExtractor
}

// NewStreamHGExtractor constructs the extractor with HTTP=15s, goja=5s —
// matches the Kwik extractor's defaults (HTTP wider than the JS budget so
// slow networks don't starve the goja runtime).
func NewStreamHGExtractor() *StreamHGExtractor {
	base := &packedExtractor{
		name:               "streamhg",
		hosts:              streamhgHosts,
		referer:            streamhgReferer,
		selectorPackerFail: "streamhg_packer_balance",
		selectorGojaFail:   "streamhg_goja",
		selectorRegexFail:  "streamhg_hls2_regex",
		selectorBodyFail:   "streamhg_body_read",
		http:               &http.Client{Timeout: defaultPackedHTTPTimeout},
		timeout:            defaultPackedGojaTimeout,
	}
	return &StreamHGExtractor{packedExtractor: base}
}

// Name overrides the embedded Name() for clarity at the call site (even
// though the embedded packedExtractor.Name() would return the same string).
func (e *StreamHGExtractor) Name() string { return "streamhg" }

// Compile-time assertion: StreamHGExtractor satisfies domain.EmbedExtractor.
var _ domain.EmbedExtractor = (*StreamHGExtractor)(nil)
