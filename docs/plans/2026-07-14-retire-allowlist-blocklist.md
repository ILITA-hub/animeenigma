# Retire the HLS-proxy allowlist AND the streamprobe blocklist

**Date:** 2026-07-14 · **Owner directive:** retire both static lists.
**UXΔ** = 0 (Ambiguous — invisible when done right; failure mode is a broken provider) · **CDI** = 0.05 × 21 · **MVQ** = Basilisk 85%/80% (kills what it gazes at: two static lists)

## Why

The proxy trust model is `preauth (masked token) OR allowlisted OR provenance-signed` (`libs/videoutils/proxy.go:827-830`). Signing (`streamsign` → `videoutils.SignStreamURL`) is now the norm — the static `HLSProxyAllowedDomains` list survives only for catalog paths that predate it. Every list entry is standing tech debt: new CDNs need code redeploys, stale entries widen the proxy surface.

The `adCDNHostSuffixes` blocklist (`libs/streamprobe/blocklist.go`) exists only because the cheap probe never decoded segment bytes — a fake CDN answering 200 was indistinguishable from a real one. Detect poison at resolve time and the list is unnecessary.

## Track S — allowlist → universal signing

End state: gate becomes `preauth OR first-party OR provenance-signed`. No external-domain allowlist. First-party internal hosts (`stealth-scraper`, `minio`) stay gated by the existing `FirstPartyHosts` config (they resolve to Docker-private IPs; `netguard.ValidatePublicURL` inside `SignStreamURL` rejects them by design, so they can never be publicly signed — this is a config-derived host set, not a domain allowlist).

### S0 — sign at source (additive, zero-risk: gate is OR)

Copy the animejoy pattern (`catalog.go:2036-2046` + `domain.AnimejoyStream` Exp/Sig/MaskedURL fields):

| Emitter | Site | Covers allowlist entries |
|---|---|---|
| `GetKodikStreamSource` | `catalog.go:2113` (returns ~2172) | `solodcdn.com`, `cloud.solodcdn.com` |
| `GetHanimeStream` | `catalog.go:2866` (sources ~2886) | `hanime.tv`, `highwinds-cdn.com`, `htv-*.com`, `hydaelyn-*.top`, `zodiark-*.top` |
| `Get18AnimeStream` | `anime18.go:108` | `mp4upload.com`, `turboviplay.com`, `turbosplayer.com` |
| `GetAnimeLibStream` | `catalog.go:2480` | `cdnlibs.org`, `hentaicdn.org` (dormant — no FE adapter, signed anyway) |
| jimaku subtitles | `subs_aggregator.go:319`, `catalog.go:2784` | `jimaku.cc` |

Frontend: forward `exp`/`sig`/`masked_url` in `useProviderResolver.ts` kodik (~512), hanime (~425, fix the stale "token-signed" comment), 18anime (~388) adapters via the existing `buildProxyUrl` sign arg; jimaku via `buildSubtitleProxyUrl`/`useSubtitleTracks.ts`.

Probe (:8092): `probe/kodiknoads.go` must replay `exp`/`sig` like `probe/animejoy.go:81-96` (today it relies on the allowlist). Same for any hanime/18anime probes.

OpenSubtitles/anime365 are already same-origin catalog routes — untouched.

### S1 — mint provenance across redirects (kills `mt.nekostream.site`, AUTO-517)

Manifest rewriting bases on the **pre-redirect** `sourceURL` (`proxy.go:933`), so children of a 302 target get no token and are re-gated bare. Fix: rewrite against `resp.Request.URL` (the post-redirect final URL — also the RFC-correct HLS base URI). Provenance/masked tokens are then minted for redirect-target children automatically (`proxy.go:1175-1198`). Remove the `mt.nekostream.site` stop-gap in S3.

### S2 — verify with real anime (gate for S3)

Through the deployed proxy: Kodik (solodcdn), Hanime, 18anime, jimaku subtitle fetch, one scraper stream (regression), ae/library, animejoy. Confirm signed params flow end-to-end and playback-critical URLs return 200.

### S3 — flip the gate, delete the list (only after S2 green)

- `proxy.go:827`: replace `isHLSDomainAllowed(host)` with a first-party-host check (reuse the `FirstPartyHosts` set already wired at `stream.go:148-153`).
- Delete `HLSProxyAllowedDomains` (`proxy.go:486-532`), `matchHLSDomain`/`isHLSDomainAllowed`, the `stream.go:91` wiring, and the regression-lock test `proxy_test.go:207-259`.
- Out of scope: `ProxyConfig.AllowedDomains` (`PROXY_ALLOWED_DOMAINS`) on the legacy `ProxyStreamCounted` token path, and the poster image-proxy allowlist — different features, not the HLS trust gate.

Rollback: revert the S3 commit only — S0/S1 are additive and stay.

## Track B — blocklist → runtime poison detection

End state: no static host list. The probe convicts a stream by its **bytes**; verdicts cached 24h in Redis so a poisoned host is touched at most once a day.

### B0 — segment byte sniff in `libs/streamprobe`

`checkSegments` goes ranged-GET-first (`bytes=0-1023`; HEAD already gave false negatives on megaplay — finding L718) and sniffs magic bytes:

- **Playable:** MPEG-TS sync `0x47`, fMP4 (`ftyp`/`styp`/`moof`/`moov` at offset 4), EBML/WebM `1A45DFA3`, ID3-prefixed TS.
- **Poison → `ReasonAdDecoy`:** PNG/JPEG/GIF magic, HTML (`<!doctype`/`<html`), regardless of claimed Content-Type (nekostream serves a PNG dressed as `video/mp2t`; the proxy itself *forces* `image/*`→`video/mp2t` at `proxy.go:1316`, so upstream headers prove nothing).
- **Unknown magic:** fail-open (playable) — never brick a weird-but-real CDN.

`Result` gains the offending host so callers can cache the verdict.

### B1 — 24h poison-verdict cache (replaces the static list dynamically)

In the scraper's resolve-time gate (`gogoanime.coldPathGated.attemptOne`, `client.go:1166-1216`):
- **Pre-check** `scraper:streamprobe:poison:<host(src.URL)>` — hit ⇒ drop source without touching the network, continue to next source/server (existing loop).
- **On `ReasonAdDecoy`** ⇒ `SetNX` 24h keys for both the stream host and the offending segment host.

Trade-off vs. the static list: the old list never contacted a known-bad host (T-21-03 IP-leak concern); runtime detection contacts it at most **once per 24h per host** with a 1 KiB ranged GET. Accepted — that's the price of self-maintaining detection, and the TikTok ad-CDN entries are caught by the same sniff (they serve images).

This supersedes the Redis-lift TODO (`spec §4.1.c-TODO`): instead of lifting the list into Redis, the list ceases to exist; Redis holds machine-derived 24h verdicts.

### B2 — delete the list

Remove `blocklist.go` + `blocklist_test.go`, drop the `isAdCDNHost` short-circuit (`probe.go:123-125`). Keep `ReasonAdDecoy` and `ParserAdDecoyTotal` — same meaning, now byte-derived.

## Deploy order

1. **scraper** (Track B — independent).
2. **catalog** → **web** → **analytics** (S0: emit signed → forward → probe replays). All additive.
3. Verify (S2).
4. **streaming** (S1+S3 gate flip) — LAST.
