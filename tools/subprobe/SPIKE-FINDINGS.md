---
spike: burned-in-subtitle-probe
type: standard
validates: "Given one real burned-in-hardsub stream and clean (no-burned-text) frames, when we run a tier-1 edge/contrast/bright-stroke heuristic on the bottom-center band + a Tesseract OCR pass, then SUB(hardsub)-vs-RAW(clean) separation is clean enough to build a reconciliation probe on."
verdict: VALIDATED (with scope corrections)
related: [self-healing-playback-loop, provider-policy-self-healing, unified-probe]
tags: [subtitle, probe, ffmpeg, tesseract, hardsub, self-heal, scraper]
date: 2026-06-29
source_todo: 2026-06-20T01-29-59_claude-code_feedback
---

# Spike: Burned-in Subtitle Probe

## Verdict — VALIDATED, with two scope corrections that change the engine design

The pixel + OCR separation between **burned-in hardsub** and **clean video** is clean and
robust on real anime. BUT two findings reframe what the engine should be:

1. **The pixel probe is a HARDSUB detector, not a universal SUB detector.** Not all "sub"
   providers burn subs in. `gogoanime`/megaplay serves **softsub** — clean JP video + separate
   `.vtt` tracks (English/Arabic/French). A pixel probe on it correctly sees no burned text.
   So the full content-class taxonomy needs THREE independent signals:
   - **pixel probe** → is there burned-in hardsub text? (SUB-hardsub vs not)
   - **track-list presence** → is a soft `.vtt`/`.ass` track offered? (softsub availability)
   - **audio language-ID** → is the audio non-JP? (DUB) — pixels can't answer this.
   The `miruro` failure that MOTIVATED this TODO (advertised SUB but lost its sub track) is a
   **softsub-availability / audio** problem a pixel probe would NOT catch. Pixels are one leg
   of a tripod, not the whole stool.

2. **Stream acquisition dominates the engineering risk, not the CV.** See "Infra reality".

## Evidence

Subject: **Sousou no Frieren** ep.12 (uuid `f0b40660…`), real streams via our own
gateway→catalog→scraper→streaming-proxy pipeline. Hardsub source = `miruro` (which fronts
**animepahe**, EN hardsub) over the stable `uwucdn.top` direct CDN.

### Tier-1 heuristic (bottom-center band, bottom ~26%, x 6–94%)
Per-frame cheap scores. `edge_density` = strong-gradient fraction; `text_stroke` = bright
(>195 luma) pixels sitting on a strong edge (white glyph stroke); `row_peak` = busiest row.

| Class (real) | edge_density | text_stroke | OCR |
|---|---|---|---|
| Hardsub frame (text on screen) | 0.024 – 0.096 | 0.006 – 0.019 | English read |
| Clean frame, calm scene | 0.000 – 0.005 | ~0.000 | no text |
| Clean frame, **busy character scene** (f_005) | 0.029 | **0.002** | no text |

- Dense 125-frame mid-episode sweep: tier-1 (`edge≥0.02`) fired on 66/125; **60 OCR-confirmed
  text**. The 6 "FP candidates" were almost all real subs that OCR under-read
  (e.g. `"The village we're heading to is north of here"`), NOT busy-scene noise.
- **`text_stroke` beats raw `edge_density` as the gate.** Busy character scenes (robes/capes in
  the band) push `edge_density` over a naive threshold but keep `text_stroke` ≈ 0.002, well
  below real subs (≥~0.006). OCR is the reliable backstop on the residue.

### Tier-2 OCR (Tesseract 5.3.4, `-l eng+rus+jpn`, band upscaled 3×, `--psm 6`, tsv)
- **Script-ID works**: synthetic RU → cyrillic (conf 58–94), EN → latin; real animepahe → latin.
  This is the RU-hardsub vs EN-hardsub distinguisher.
- OCR over a video frame full of background detail is **noisy** — confidence is the signal, not
  the literal string. Gate on word-confidence + alnum count, not exact text.

### OP/ED karaoke = a real false positive (expected, and trivially mitigated)
OP sweep fired 6/15 — f_008 conf=96 read `"So 'til the day I expire, I'll carry them along"`
(the OP song subtitle). Karaoke IS burned-in text. **Mitigation is free**: the scraper stream
response already returns `intro{start,end}` and `outro{start,end}` markers — sample strictly
between them.

## Recommended design (folds into self-healing-playback-loop)

Role: an **async reconciliation probe** that VERIFIES + auto-corrects a provider's sub/dub
labels on a mismatch signal (user report / periodic audit / provider change). NOT on the
playback hot path. Cache the verdict per `(provider, anime, category)`.

Pipeline per (provider, anime, category):
1. Resolve the stream (one shot) and skip OP/ED via intro/outro markers.
2. Sample ~15–20 mid-episode frames (ffmpeg, dialogue-dense region).
3. **Tier-1 gate** = `text_stroke` over the bottom-center band (primary), `edge_density`
   secondary. Aggregate by **high percentile / count-over-threshold across frames**, NOT
   median — subs are intermittent (~50% of frames have text on screen).
4. **Tier-2** = Tesseract only on tier-1 hits → confirm real text (kills busy-scene FPs) +
   script (Cyrillic→RU, Latin→EN, CJK→on-screen JP signage, ignore).
5. Combine with **track-list presence** (softsub) and **audio LID** (dub, e.g. whisper —
   separate probe) for the full SUB-hardsub / softsub / RAW / DUB verdict.

## Infra reality (the part that bites)

- **Host has it all**: ffmpeg/ffprobe present; installed `tesseract-ocr` + `eng/rus/jpn`
  (5.3.4); numpy+PIL are enough (no OpenCV needed). The `library` container ships ffmpeg and
  is on the docker network.
- **Browser-engine providers are probe-hostile.** `gogoanime`/`nineanime` stream through an
  **ephemeral `stealth-scraper:3000/hls` sid** that returns **410 Gone within ~1 min**, and
  *segments* are sid-gated too — so ffmpeg's segment-by-segment read fails. Worse, the resolved
  stream URL is **cached ~1h** pointing at the dead sid → an async probe that re-fetches a
  cached URL later gets a corpse. **The probe must resolve-and-read in one continuous shot**,
  and should prefer **direct-CDN providers** (animepahe via kwik/uwucdn, miruro) which are
  stable for the ~1h cache window.
- **Proxy mechanics that worked** (host → real frames):
  - Playback URL = `http://localhost:8000/api/streaming/hls-proxy?url=<src>&exp=&sig=&referer=<stream.headers.Referer>`.
    The referer is REQUIRED (uwucdn 403s without it).
  - The proxied master playlist has **relative** `/api/streaming/hls-proxy?...` segment/key
    URIs that ffmpeg mis-resolves. Fix: fetch the playlist, rewrite those to absolute
    `http://localhost:8000/...`, save locally, then
    `ffmpeg -allowed_extensions ALL -protocol_whitelist file,http,https,tcp,tls,crypto -i local.m3u8`.
  - animepahe HLS is **AES-128 encrypted with `.jpg`-disguised segments** → `-allowed_extensions ALL`
    is mandatory; ffmpeg fetches the (proxied) key and decrypts fine.
- **Rate budget**: rapid repeated resolves exhaust the Camoufox pool (503 + breaker trip). The
  probe must be gentle and async — consistent with the existing pool breaker design.
- No first-party/library content currently (`has_video=true` → 0 rows), so a clean RAW control
  has to come from a live provider, not MinIO.

## Files (scratchpad)
- `analyze.py` — tier-1 + tier-2 per frame
- `analyze2.py` — gated design (tier-1 → OCR only on hits) + FP surfacing
- `extract_frames.sh`, `resolve_and_frames.sh` — stream → frames
- `make_synthetic.py` — synthetic SUB/RAW self-test
- `frames/`, `work/` — extracted frames, overlays, playlists
