package animejoy

import (
	"context"
	"fmt"
)

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
