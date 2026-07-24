# Stream proxy security

This is the current trust contract for external video and subtitle URLs. It
replaces the completed 2026-07-14 allowlist/blocklist retirement plan.

## Proxy admission

The HLS proxy admits a request only when one of these conditions holds:

1. a masked, authenticated stream token authorizes it; or
2. the URL carries a valid provenance signature minted by AnimeEnigma.

There is no external CDN allowlist, and there is no host exemption. Naming a
first-party internal host (`minio`, `stealth-scraper`) in the client-supplied
`url` does NOT authorize a request: the proxy endpoint is public and
unauthenticated and it presigns self-hosted MinIO reads with the streaming
service's own credentials, so a host-name exemption would have been an
unauthenticated read of the private object store. Those hosts are ordinary
hostnames, so they sign and seal exactly like every other emitter.

Every catalog or scraper path that emits a stream/subtitle URL — external CDN
*and* first-party MinIO/sidecar alike — must stamp it at the source with
`streamsign` / `videoutils.SignStreamURL` and return the resulting `exp`/`sig`
or masked URL through its API shape. Frontend adapters must preserve that
authorization when building proxy URLs.

`ProxyConfig.FirstPartyHosts` remains, but only as the dial-time exemption from
the private-IP SSRF guard (internal services legitimately resolve to
Docker-private addresses). It is not an admission rule.

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
