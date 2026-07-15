# Stream proxy security

This is the current trust contract for external video and subtitle URLs. It
replaces the completed 2026-07-14 allowlist/blocklist retirement plan.

## Proxy admission

The HLS proxy admits a request only when one of these conditions holds:

1. a masked, authenticated stream token authorizes it;
2. the host belongs to the configured first-party set (currently internal
   services such as `stealth-scraper` and `minio`); or
3. the URL carries a valid provenance signature minted by AnimeEnigma.

There is no external CDN allowlist. Every catalog or scraper path that emits an
external stream/subtitle URL must stamp it at the source with `streamsign` /
`videoutils.SignStreamURL` and return the resulting `exp`/`sig` or masked URL
through its API shape. Frontend adapters must preserve that authorization when
building proxy URLs.

The proxy rewrites playlists relative to the final post-redirect URL and mints
provenance tokens for redirect targets and child playlists/segments. Redirects
therefore do not require a new host exception.

The separate legacy `ProxyConfig.AllowedDomains` setting and poster/image proxy
rules are different trust boundaries and are not part of this HLS contract.

## Poison detection

`libs/streamprobe` validates media bytes instead of trusting a hostname or
Content-Type. It recognizes MPEG-TS, fMP4, EBML/WebM, and ID3-prefixed TS.
Image or HTML magic bytes are classified as `ReasonAdDecoy`; unknown magic
fails open so unusual but valid media is not bricked.

The scraper caches confirmed poison verdicts in Redis for 24 hours by stream
and offending-segment host. A cached verdict skips the host without another
network request. This runtime evidence replaces the retired static ad-CDN
blocklist.

## Adding a source

- Stamp every external stream and subtitle URL at its backend emitter.
- Forward signature/masked-token fields through normalized API types and the
  frontend adapter.
- Exercise the URL through the real proxy, including redirects and one media
  segment; a playlist-only 200 is not sufficient.
- Add probe coverage that replays the signature and verifies real media bytes.
- Do not add host exceptions or a new static poison list.

Primary implementation: `libs/videoutils/proxy.go`,
`libs/videoutils/provenance.go`, `libs/videoutils/streamtoken.go`,
`libs/streamprobe/`, and the gogoanime resolve-time gate.
