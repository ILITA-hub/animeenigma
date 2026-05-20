Verdict: converged

# SPIKE-MIRURO — Miruro Obfuscation Reverse-Engineering (SCRAPER-HEAL-34)

**Plan:** 28-00-PLAN.md
**Phase:** 28-provider-expansion-r2
**Wave:** 0
**Probe date:** 2026-05-20
**Probe source:** production server (this checkout's host)

## TL;DR

Miruro's GET-path "obfuscation" is **not cryptographic** — it is **transport encoding**:

- **Request:** `https://www.miruro.tv/api/secure/pipe?e=<base64url(json({path, method, query, body, version?}))>`
- **Response:** body is `base64url(gzip(jsonBody))` when `x-obfuscated: 1`, OR `base64url(xor(gzip(jsonBody), VITE_PIPE_OBF_KEY))` when `x-obfuscated: 2`.

`VITE_PROXY_OBF_KEY` is **never used** for the GET flow. `pro.ultracloud.cc` is a separate `VITE_PROXY_A` host the SPA can route through but is NOT the path the React app actually calls; the React app calls `https://www.miruro.tv/api/secure/pipe?e=...`. POST flow uses ECDH-ES + A256GCM JWE (stdlib `crypto/ecdh` + `crypto/aes`/`cipher.NewGCM`), but Miruro's anime API surface is **all GETable** (info/episodes/sources), so the POST flow is not required for SCRAPER-HEAL-37.

All four D3 convergence gates pass with **stdlib-only** primitives.

## D3 Convergence Gate Evaluation

### Gate 1: Transform implementable in pure Go via stdlib — PASSED

The GET pipeline reduces to:

| Step | Direction | Primitive | Go stdlib |
|------|-----------|-----------|-----------|
| Build `e=` param | request | JSON marshal → base64url-no-padding | `encoding/json`, `encoding/base64` (`base64.RawURLEncoding`) |
| Decode response (`x-obfuscated: 1`) | response | base64url-no-padding → gunzip → JSON unmarshal | `encoding/base64`, `compress/gzip`, `encoding/json` |
| Decode response (`x-obfuscated: 2`) | response | base64url → XOR-with-cycling-key → gunzip → JSON unmarshal | + `encoding/hex` for `VITE_PIPE_OBF_KEY` decode |

**No HMAC, no AES, no `VITE_PROXY_OBF_KEY` ever involved in the GET flow.** A pure-Go port is a ~120-line file using only `encoding/json`, `encoding/base64`, `compress/gzip`, `encoding/hex`, and `bytes`.

Bundle evidence (minified, line-anchored by `grep`):

```
Go=$n(`VITE_PIPE_OBF_KEY`),                            // Go = hex string "71951034f8fbcf53d89db52ceb3dc22c"
Ko=Go?new Uint8Array(Go.match(/.{2}/g).map(e=>parseInt(e,16))):null,   // Ko = 16 bytes (AES-128 / XOR key)
// In makeSecureGet:
let e=await Qo(Zo(`/api/secure/pipe`,{e:this.base64urlEncode(r)}));     // GET request
let t=await e.text(), n=e.headers.get(`x-obfuscated`);
if(n){
  let e=t.replace(/-/g,`+`).replace(/_/g,`/`),
      r=e.length%4,
      i=e+(r?`=`.repeat(4-r):``),
      a=Uint8Array.from(atob(i),e=>e.charCodeAt(0));     // base64url → bytes
  if(n===`2`&&Ko){
    let e=new Uint8Array(a.length);
    for(let t=0;t<a.length;t++)e[t]=a[t]^Ko[t%Ko.length]; // XOR cycle
    a=e
  }
  return JSON.parse(Jo.decode(Ki(a)))                    // Ki = gunzip
}
```

Source: `https://www.miruro.tv/assets/index-VOBoBQUM.js` (snapshot saved as `services/scraper/internal/providers/miruro/testdata/env2.js` adjacent to the env2 capture; full bundle is 735KB, not committed — anchor strings above are sufficient for verification).

### Gate 2: Live HTTP 200 from production server — PASSED

Three live probes against `https://www.miruro.tv/api/secure/pipe?e=<base64url-of-json>` from this checkout's production host:

| Endpoint | HTTP | x-obfuscated | Body Size | Decoded Type |
|----------|------|--------------|-----------|--------------|
| `info/154587` | **200** | 1 | 37,116 B (base64url) → 27,842 B (raw) → 1,306,251 B (gunzipped JSON) | Frieren AniList metadata + 28-episode embedded list |
| `episodes` w/ `anilistId=154587` | **200** | 1 | 24,491 B → 17,876 B → 136,824 B JSON | 5 provider blocks (`dune`, `kiwi`, `hop`, `bee`, `ANIMEKAI`); `dune.sub=28`, `kiwi.sub=28`, `hop.dub=28`, `bee.sub=28` |
| `sources` w/ `episodeId=YW5pbWVwYWhlOjUzMTk6NjAwNTk6MQ`, `provider=kiwi` | **200** | 1 | 615 B → 372 B → JSON | `{streams:[{url:".../uwu.m3u8",type:"hls",quality:"1080p",referer:"https://kwik.cx/"},...]}` |

CORRECTION to plan's prior assumption: `pro.ultracloud.cc` is `VITE_PROXY_A` — a backup/alternate endpoint exposed in `env2.js`, but the React SPA does NOT call it. All anime API calls go through `https://www.miruro.tv/api/secure/pipe?...` directly. No Cloudflare TLS-fingerprint friction triggered from the production IP (Pitfall 8 was a hypothetical — `www.miruro.tv` responds 200 to plain Go-style HTTP clients with a Chrome User-Agent header).

### Gate 3: Key stability across ≥3 sequential `env2.js` fetches — PASSED

Three `https://www.miruro.tv/env2.js` fetches spaced ≥32s apart on 2026-05-20:

| Fetch | HTTP | SHA-256 of body |
|-------|------|-----------------|
| 1 | 200 | `02233bdffa93f4b00f05028d4cf321c05c2a7b9d25af222ba0b584e27d980e1f` |
| 2 | 200 | `02233bdffa93f4b00f05028d4cf321c05c2a7b9d25af222ba0b584e27d980e1f` |
| 3 | 200 | `02233bdffa93f4b00f05028d4cf321c05c2a7b9d25af222ba0b584e27d980e1f` |

All three identical. `VITE_PROXY_OBF_KEY=a54d389c18527d9fd3e7f0643e27edbe` and `VITE_PIPE_OBF_KEY=71951034f8fbcf53d89db52ceb3dc22c` stable across the probe window. Cache headers indicate static-asset cache-bust at deploy time only — no per-session rotation.

Captured to `services/scraper/internal/providers/miruro/testdata/env2.js`.

### Gate 4: Frieren AniList 154587 spot-check returns non-error episode listing — PASSED

`episodes?anilistId=154587` returned 28 episodes per provider for the `sub` audio track across all 4 active providers (`dune`, `kiwi`, `hop`, `bee`). Sample episode 1 metadata from the `kiwi` (animepahe-derived) block:

```json
{
  "id": "YW5pbWVwYWhlOjUzMTk6NjAwNTk6MQ",
  "number": 1,
  "title": "Shall We Go, Then?",
  "airDate": "2026-01-16",
  "duration": 1561,
  "audio": "sub",
  "filler": false,
  "description": "Frieren, Fern, and Stark leave the magic city of Äußerst behind and travel along a road in the northern lands."
}
```

The `sources` follow-up call returned a live `1080p` HLS m3u8 URL (`https://vault-08.uwucdn.top/stream/08/13/.../uwu.m3u8`) plus the Kwik embed fallback. Episode count = 28 ≥ 1 ⇒ Gate 4 PASSED.

## Transform Shape Identified

**Family:** Base64url(JSON) for request URLs; Base64url(gzip(JSON)) or Base64url(XOR-cycle(gzip(JSON), KEY)) for response bodies.

The plan's hypothesised `TransformProxyURL(endpoint string, obfKey []byte) (string, error)` signature is preserved by the Go port, but its `obfKey` argument is **unused** in the GET-only path. Kept as an argument for API stability so the future POST-path implementation (if needed for `/api/secure/pipe` POST envelope) can reuse the signature.

## Go Port

Files committed in this plan:

- `services/scraper/internal/providers/miruro/obfuscation.go` — exports:
  - `TransformProxyURL(endpoint string, obfKey []byte) (string, error)` — builds the `e=` query value (and returns full URL).
  - `BuildSecurePipeURL(host, endpoint, method string, query map[string]any) (string, error)` — full URL builder.
  - `DecodeObfuscatedResponse(body []byte, xObfuscated string, pipeKey []byte) ([]byte, error)` — handles `x-obfuscated: 1|2`.
- `services/scraper/internal/providers/miruro/obfuscation_test.go` — table-driven test with 3 vectors from `testdata/transform_vectors.json` + 2 negative cases.

Stdlib imports only: `encoding/base64`, `encoding/json`, `encoding/hex`, `compress/gzip`, `bytes`, `errors`, `fmt`, `io`, `net/url`, `strings`.

No `chromedp`, `utls`, `tls-client`, `goja`, `cloudscraper`, or `flaresolverr`.

## Downstream Effect

**Plan 28-04 (Miruro lift, SCRAPER-HEAL-37) — PROCEEDS in Wave 2.**

- Reuses `obfuscation.go` for all upstream calls.
- The provider client wires three GET endpoints (`info/{anilistId}`, `episodes?anilistId={id}`, `sources?episodeId={id}&provider={p}&category=`) through `BuildSecurePipeURL` + `DecodeObfuscatedResponse`.
- `FindID` uses ARM-mapped AniList ID directly (no fuzzy required).
- `GetStream` returns the `streams[]` items where `type=hls`; the embed types (`kwik.cx`) are skippable since the direct HLS URL is in the same response.
- HLS proxy allowlist: add `vault-*.uwucdn.top`, plus keep `kwik.cx`/`kwik.si` (already allowlisted for animepahe extractor reuse). Adjust per per-host probe in Plan 28-04.
- v3.1 REQUIREMENTS.md non-goal forbids `utls`/`chromedp` — no risk of regression since none are needed.

**SCRAPER-HEAL-37 does NOT roll to v3.2.** Wave 2 may proceed once Wave 1 (AnimeFever) is also green.

## Residual Artifacts

- `testdata/env2.js` — first stability-probe snapshot. Retain as evidence of the 2026-05-20 key state. Re-fetch + diff on every Wave 2 deploy as a sanity check.
- `testdata/transform_vectors.json` — golden vectors for the table-driven test. Append new vectors when adding new endpoints in Plan 28-04.

## Threat Surface Notes

- T-28-00-04 (Cloudflare TLS fingerprint): **did not fire** for any of the 7 probes during this spike. Production-IP plain Go-style HTTP with a Chrome UA header sufficed for all 200 responses.
- T-28-00-03 (response size DoS): largest decoded body was 1.3 MiB (Frieren `info`). Cap reads in Plan 28-04's client at 4 MiB via `io.LimitReader` per CONTEXT.md threat register.

## Live Integration Probe (Gate 2 + Gate 4 reproducibility artifact)

In addition to the manual curl probes recorded under Gate 2 / Gate 4 above, the Go port itself was exercised against production via a `-tags=integration` test (`obfuscation_integration_test.go`). Output captured 2026-05-20:

```
=== RUN   TestLiveMiruroSecurePipe/info_154587
    fetched info/154587 — status=200 x-obfuscated="1" bytes=37116
    info OK: id=154587 idMal=52991 title="Sousou no Frieren"
--- PASS: TestLiveMiruroSecurePipe/info_154587 (0.06s)

=== RUN   TestLiveMiruroSecurePipe/episodes_154587
    fetched episodes — status=200 x-obfuscated="1" bytes=24491
    provider dune sub: 28 eps
    provider kiwi sub: 28 eps  / dub: 28 eps
    provider hop  sub: 28 eps  / dub: 28 eps
    provider bee  sub: 28 eps  / dub: 28 eps
    Gate 4 PASSED: aggregate sub episode count = 112
--- PASS: TestLiveMiruroSecurePipe/episodes_154587 (0.03s)
```

This proves the Go port produces the same wire shape the React SPA does (otherwise upstream would return 4xx/5xx as it did for the unprefixed `/api/anime/154587` probe earlier) and that the gunzip+JSON decode logic round-trips correctly.

Re-run at any time with:
```bash
go test -tags=integration -run TestLiveMiruroSecurePipe \
  ./services/scraper/internal/providers/miruro/...
```

## Final Verdict (locked)

**Verdict: converged.** All four D3 convergence gates pass with stdlib-only Go primitives. No `utls`, `chromedp`, or third-party HTTP-fingerprinting library is required.

Plan 28-04 (Miruro provider lift, SCRAPER-HEAL-37) **proceeds in Wave 2** and consumes this package's `BuildSecurePipeURL` + `DecodeObfuscatedResponse` directly. No further architectural decisions are deferred — the obfuscation surface is fully characterised.

## Probe Method (for Reproduction)

```bash
# Build request
PAYLOAD='{"path":"info/154587","method":"GET","query":{},"body":null}'
E=$(printf '%s' "$PAYLOAD" | base64 -w 0 | tr -d '=' | tr '/+' '_-')

# Fetch
curl -sS -o body.bin -D headers.txt \
  -A "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36" \
  -H "Referer: https://www.miruro.tv/" \
  "https://www.miruro.tv/api/secure/pipe?e=$E"

# Decode
python3 -c "
import base64,gzip,json,sys
b=open('body.bin','rb').read().rstrip(b'\n').replace(b'-',b'+').replace(b'_',b'/')
b+=b'='*((-len(b))%4)
plain=gzip.decompress(base64.b64decode(b))
print(json.dumps(json.loads(plain), indent=2)[:600])"
```
