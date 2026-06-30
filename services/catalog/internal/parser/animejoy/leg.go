package animejoy

import (
	"context"
	"fmt"
)

// LegEmbedURL returns the embed URL the given episode carries for leg: ep.Sibnet
// for "sibnet", ep.AllVideo for "allvideo", "" for any other leg (or when the
// episode lacks that player). PURE — the single place that maps a leg name onto
// the Episode field it lives in, so every leg-vs-field decision (pickLegEmbed's
// selector, the leg-info builder) routes through here.
func LegEmbedURL(ep Episode, leg string) string {
	switch leg {
	case "sibnet":
		return ep.Sibnet
	case "allvideo":
		return ep.AllVideo
	default:
		return ""
	}
}

// ResolveLeg dispatches a (leg, embedURL) pair to the matching final-leg
// resolver: "sibnet" → ResolveSibnet, "allvideo" → ResolveAllVideo. It is the
// single enumeration of the supported legs, so callers (the catalog stream
// resolver) no longer switch on leg names themselves. An unknown leg returns an
// error rather than a zero ResolvedLeg.
func (c *Client) ResolveLeg(ctx context.Context, leg, embedURL string) (ResolvedLeg, error) {
	switch leg {
	case "sibnet":
		return c.ResolveSibnet(ctx, embedURL)
	case "allvideo":
		return c.ResolveAllVideo(ctx, embedURL)
	default:
		return ResolvedLeg{}, fmt.Errorf("animejoy: unknown leg %q", leg)
	}
}
