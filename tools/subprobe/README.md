# subprobe — burned-in subtitle detector (standalone)

Detects a stream's **actual** sub delivery (burned-in HARDSUB vs clean/SOFTSUB) instead of
trusting upstream `has_sub`/`sub_delivery` labels, which go stale. Built for the owner TODO
`2026-06-20T01-29-59_claude-code_feedback`; verdict + findings in
[`SPIKE-FINDINGS.md`](SPIKE-FINDINGS.md) and [`VERIFY-RESULTS.md`](VERIFY-RESULTS.md).

This is a **standalone diagnostic tool**, not wired into any service. Run it by hand to audit /
re-verify a provider's `sub_delivery`.

## How it works
1. **Tier-1 (cheap, no ML)** — `text_stroke` (bright pixels sitting on strong edges) over the
   bottom-center subtitle band of sampled mid-episode frames. Real hardsubs ≥ ~0.006; busy
   scenes / clean video stay < ~0.003.
2. **Tier-2 (Tesseract OCR)** — runs only on tier-1 hits; confirms real text (kills busy-scene
   false positives) and the **script** (Cyrillic→RU, Latin→EN, CJK→on-screen JP, ignore).

Aggregate by a **high percentile / hit-count**, not the median — subtitles are intermittent
(~half the frames have text on screen). Skip OP/ED (karaoke = burned-in text); the scraper
stream response ships `intro{start,end}`/`outro{start,end}` markers for this.

## Requirements
`ffmpeg`/`ffprobe`, `tesseract` + `eng rus jpn` language packs, `python3` with `numpy` + `Pillow`.
```bash
apt-get install -y tesseract-ocr tesseract-ocr-eng tesseract-ocr-rus tesseract-ocr-jpn
pip install numpy Pillow            # or system python3-numpy / python3-pil
```

## Usage
```bash
# End-to-end: resolve a provider's stream for an anime + episode and verdict it.
# Needs the gateway reachable (default http://localhost:8000).
tools/subprobe/verify_provider.sh <anime-uuid> <ep-number> <provider> [sub|dub] [seek_s] [dur_s]

# e.g. is gogoanime's "sub" burned-in?  ->  SOFTSUB (clean video + .vtt tracks)
tools/subprobe/verify_provider.sh f0b40660-6627-4a59-8dcf-7ec8596b3623 12 gogoanime sub
```
Env overrides: `SUBPROBE_GATEWAY` (gateway base), `SUBPROBE_OUT` (frame/work scratch dir,
default `/tmp/subprobe-out` — never the repo).

Analyze frames you already have:
```bash
python3 tools/subprobe/analyze.py <frames_dir> <label>          # per-frame tier-1 + OCR
python3 tools/subprobe/verify_verdict.py <frames_dir> <label> <n_soft_tracks>   # HARDSUB/SOFTSUB verdict
python3 tools/subprobe/make_synthetic.py                        # self-test on synthetic SUB/RAW frames
```

## Caveats (see SPIKE-FINDINGS.md for the full set)
- **It's a hardsub detector, not a SUB detector.** Softsub providers (gogoanime) serve clean
  video + separate `.vtt` tracks — those read as "no burned text". Combine with track-list
  presence (softsub) and audio language-ID (dub) for the full SUB/SOFTSUB/RAW/DUB verdict.
- **Browser-engine providers (gogoanime/nineanime)** stream via ephemeral `stealth-scraper`
  sids that 410 within ~1 min (segments too) — resolve-and-read in one shot; prefer direct-CDN
  providers. Rapid resolves exhaust the Camoufox pool — run gently.
- Proxy mechanics for reading: pass the stream's `Referer`, rewrite relative `/api/streaming/…`
  playlist URIs to absolute, and use `ffmpeg -allowed_extensions ALL` (AES-128 `.jpg` segments).
