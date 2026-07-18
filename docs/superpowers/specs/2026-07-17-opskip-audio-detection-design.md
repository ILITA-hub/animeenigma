# OP/ED Audio Detection (opskip) — Design Spec

**Date:** 2026-07-17 · **Owner request:** feedback `2026-07-08T14-46-10_tNeymik_manual` — «Сделать чтобы для всех аниме был доступен скип опенинга/эндинга», надёжно, дёшево, полностью автоматически. Approved approach: **B only** (audio fingerprint detection; provider-native intro/outro passthrough and AnimeThemes correlation are explicitly OUT of scope for this iteration).

**Metrics:** UXΔ = +4 (Better) · CDI = 0.03 * 21 · MVQ = Kraken 90%/85%

## 1. Problem

Skip windows today come ONLY from AniSkip (crowdsourced, no SLA). Ongoing and less-popular titles often have no markings, and AniSkip timings are keyed to *some* release's encode — our providers' encodes can differ (cold opens, sponsor cards, team logos). Result: no skip button, or a skip that lands wrong.

## 2. Core idea (Jellyfin Intro-Skipper approach, adapted)

The OP music is (near-)identical across episodes of the same title. Therefore:

1. **Bootstrap (per anime × kind):** take the audio of the first `HEAD_WINDOW` (480s) of TWO episodes of the same (anime, provider[, team]), chromaprint both, cross-correlate → the longest common segment with similarity ≥ `SIM_THRESHOLD` and length in `[MIN_MATCH, MAX_MATCH]` = the OP. Its fingerprint slice is stored as the anime's **season OP fingerprint**. Same on the last `TAIL_WINDOW` (480s) for the ED.
2. **Incremental (per provider/team/episode):** the OP *music* is identical across encodes and providers — locating a stored fingerprint in ONE episode's head/tail window is a single-extraction job. Every additional (provider, team, episode) costs one low-quality audio pull + one match, no pairs.
3. **Multi-OP titles:** long shows swap OPs mid-season. The fingerprint store allows MULTIPLE fingerprints per (anime, kind); matching tries all stored ones. When locate fails on TWO ADJACENT episodes of the same unit family (one failure alone = a legit no-OP finale/recap), those two episodes are paired to bootstrap an additional fingerprint — the pair pass overwrites the earlier episode's `no_match` row with its detected window. A pair that finds no common segment (e.g. recap+finale, both genuinely OP-less) stores no fingerprint and both rows stay `no_match`. One-off special OPs (single-episode variants) intentionally stay `no_match` — a segment must occur twice to prove it's an OP and yield boundaries; AniSkip fallback still covers that episode.
4. **No-match is a first-class answer:** finales/recaps legitimately skip the OP. A below-threshold match stores `no_match` (never served, never retried before its cooldown), NOT a fabricated window.

## 3. Where it runs

Inside **content-verify (:8101)** — the queue (priority: visits +15, ongoing +10, top-100 +5), governor shed gate, budget ctx, Redis cooldowns, catalogclient (the exact gateway e2e path aePlayer uses), ffmpeg extraction, and the Python analyzer runner are all reused as-is.

- **New analyzer:** `analyzers/opskip.py` — chromaprint via `fpcalc -raw` (Dockerfile gains `libchromaprint-tools`), numpy sliding-window Hamming matching. Two modes: `pair` (bootstrap: two wavs → common segment + per-file offsets) and `locate` (one wav + stored fingerprints → best offset/similarity).
- **Extraction:** reuse `playlist.go` (media playlist localization + summed EXTINF duration) and a new audio-only `ExtractWindow` (mono 16k wav, no frames; `-ss 0 -t 480` head / `-ss duration-480 -t 480` tail; lowest-bandwidth HLS variant when the master offers a ladder). MP4 legs (animejoy) use `-ss` over the range-capable proxy.
- **Scheduling:** the same worker loop. Enumeration emits **skip units** after verify units for an anime (verify = higher value). One probe at a time, same `CV_INTERVAL` pause, same budget. Skip units per (anime, provider, team?, episode), episodes ascending. Kodik: per (team, episode), pinned team first, then roster order. Adult group excluded (same rule as verify).
- **Bootstrap ownership:** when the anime has NO stored fingerprint of a kind, the claimed skip unit runs **pair mode**: it extracts its own episode AND the next available episode of the same unit family in one probe (double extraction, one budget ctx), stores the fingerprint + both episodes' windows. Every later unit runs single-extraction **locate**. An episode probed while no fingerprint could be built stores `pending_fp` and re-enters the queue after its cooldown.
- **Cooldowns:** successful window or `no_match` = terminal for that unit (timings are immutable per encode) — re-probe only via manual wipe. Failures (unreachable stream) back off like verify units.

## 4. Data model (new tables, GORM AutoMigrate)

```go
// one row per detected/attempted episode window pair
type SkipTiming struct {
    ID        string  `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())"`
    AnimeID   string  `gorm:"type:uuid;uniqueIndex:idx_skip_unit"`
    Provider  string  `gorm:"size:64;uniqueIndex:idx_skip_unit"`
    Team      string  `gorm:"size:128;uniqueIndex:idx_skip_unit"` // kodik translation TITLE (what the FE combo carries); "" otherwise
    Episode   int     `gorm:"uniqueIndex:idx_skip_unit"`
    OpStart   float64 // seconds; Op/Ed pair zeroed when that side is no_match
    OpEnd     float64
    EdStart   float64
    EdEnd     float64
    OpStatus  string  // detected | no_match | pending_fp (no fingerprint yet)
    EdStatus  string
    Confidence float64
    ProbedAt  time.Time
}

// season fingerprints, several per (anime, kind) allowed
type SkipFingerprint struct {
    ID        string `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())"`
    AnimeID   string `gorm:"type:uuid;index"`
    Kind      string // op | ed
    Fp        []byte // raw chromaprint int32 slice of the matched segment
    Length    float64
    SourceNote string // "gogoanime ep3+ep4" — debugging provenance
    CreatedAt time.Time
}
```

## 5. Serving & blending

- **cv:** `GET /internal/verify/skip?anime_id=` → all SkipTiming rows for the anime (Docker-network-only, like the verdicts endpoint).
- **catalog:** the existing `/api/skip-times/{malId}/{episode}` handler gains optional query params `anime=<uuid>&provider=&team=`. **Resolution order (REVERSED 2026-07-18, owner directive): AniSkip wins per SIDE (op/ed independently); a detected row fills only the sides AniSkip lacks.** Responses reuse the AniSkip wire shape (`found/results[{interval, skipType, episodeLength}]`) plus per-item `source: "aniskip"|"detected"` and a top-level roll-up (`detected`/`mixed`; empty = pure AniSkip). When both sources cover a side and either boundary differs by >10s, catalog logs the distinct **"skip divergence: detected vs aniskip"** WARN (deduped 24h per unit+side) — the "our probe stored a window first, AniSkip appeared later and disagrees" investigation case. Note the accepted trade-off vs the original detected-first order: on encodes whose cuts differ from the release AniSkip was keyed to, AniSkip's window may land wrong even though our per-encode window was right — divergence WARNs are how those cases surface for review. Detected side cached 10min (rows only grow).
- **FE:** `useSkipTimes` passes the active combo's animeId/provider/team and carries per-segment `source`; hacker mode shows a `SKIP op:aniskip 90.0–180.0 · ed:detected …` debug line. `useSkipIntro`/`skipSegments`/auto-skip settings work unchanged.

## 6. Cost envelope

Per (episode, side): ~480s of lowest-quality audio ≈ 15–40 MB traffic, ffmpeg 10–30s, fpcalc+match <2s. A 24-ep season on one provider ≈ 48 extractions ≈ 1–2h at the 60s probe cadence — inside the existing budget; the governor sheds under pressure. Fingerprint storage is ~15 KB per window — negligible.

## 7. Config (all optional, defaults in parens)

`CV_SKIP_ENABLED` (true) · `CV_SKIP_HEAD_WINDOW` (480s) · `CV_SKIP_TAIL_WINDOW` (480s) · `CV_SKIP_MIN_MATCH` (50s) · `CV_SKIP_MAX_MATCH` (150s) · `CV_SKIP_SIM_THRESHOLD` (0.75)

## 8. Out of scope (explicit)

- Provider-native intro/outro passthrough (approach A) — later iteration.
- AnimeThemes correlation for movies/single episodes (approach C) — later; movies simply get no detected windows (AniSkip fallback still applies).
- Submitting timings to AniSkip (the feedback's admin-marking annex) — unchanged, deferred.
- Classic Kodik iframe — no player control there.

## 9. Implementation deltas (v1, 2026-07-17 — post final review)

- **Per-kind bootstrap:** the pair task carries `PairKinds` — only the kinds with no stored fingerprint are pair-bootstrapped; kinds that already have fingerprints are located on both pair episodes instead (no duplicate fingerprints, no OP-blocks-ED asymmetry). Re-pair triggers per kind too (both adjacent rows `no_match` on that kind).
- **mp4 (animejoy) EDs are terminal `no_match` in v1** — without a known duration the tail window's absolute times can't be computed; AniSkip covers EDs there.
- **ae (firstparty) has no skip lane in v1** (no episode list in the capability pass); AniSkip fallback holds.
- Residual accepted: a title where one kind never bootstraps keeps its last un-paired episode on the 6h `pending_fp` cycle (bounded to ~1 episode; the movie/single-episode case from §8).
- **Live-found deltas (2026-07-17 evening, `ea065073`):** fpcalc runs with `-length 0` — its default `-length 120` silently fingerprinted only the first 2 minutes of each 480s window while reporting the full duration, skewing the frame rate (and every boundary time) ~4×; fpcalc with an uncapped length exits non-zero after printing complete JSON (decoder EOF), so the analyzer judges success by parseable output. Pair mode reports `duplicate` (common run ≥300s at mean sim ≥0.95) when a provider serves the same file for both episodes (seen live: nineanime/Frieren) — the prober maps it to `pending_fp` and never stores a fingerprint. Equal-length capped runs tie-break on higher mean similarity (first-lag-wins picked arbitrary musically-self-similar alignments).
- **AniSkip-first + probe gate (2026-07-18, owner directive):** serving order reversed (see §5). The skip lane now consults AniSkip coverage per episode (Engine: 6h cache, ≤50 fetches/claim, pure-AniSkip proxy path so our own detected windows can't feed the gate) and does NOT re-probe covered sides: fully-covered units are dropped before planning (no claim tick, no row); a partially-covered unit skips the covered side's window extraction and stores the terminal `aniskip` status. Pair-bootstrap ignores coverage (fully-covered units were already filtered; pairs are rare and need clean semantics) — live consequence (observed 2026-07-18, kodik/AnimeVost ep11+12): an ED-no_match re-pair on two gate-partial rows re-locates the op side from the audio it already extracted and overwrites `aniskip`→`detected`; bounded by PairTried, harmless for serving (AniSkip still wins per side), and it feeds the divergence QC for free. New residual: a title AniSkip covers except one episode can't pair-bootstrap from the lone episode — it idles on the 6h `pending_fp` cycle unless another family contributes a fingerprint. `CV_PIN_ANIME` (`uuid[:provider]`) added as the operator "probe THIS now" lever: top rank, cooldown bypass, preferred skip family first.

## 10. Verification

- Unit tests: opskip.py pair/locate on synthetic fingerprints; enumeration ordering; blending precedence in the catalog handler.
- Live: pick an ongoing with no AniSkip data (roster/queue evidence), let the queue bootstrap, verify the skip chip appears in aePlayer at the right seconds vs. manual scrub.
