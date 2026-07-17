# OP/ED Audio Detection (opskip) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatic OP/ED skip windows for every anime × provider × team × episode, detected by cross-episode audio fingerprint matching inside content-verify, blended into the existing `/api/skip-times` surface with AniSkip as fallback.

**Architecture:** content-verify (:8101) gains a second probe lane: queue enumeration emits SkipUnits alongside verify Units from the same catalog fetch pass; a pure planner picks pair-bootstrap vs locate tasks; the skip prober extracts head/tail audio windows via ffmpeg (lowest HLS variant), fingerprints with chromaprint (`fpcalc`), and matches with a new `opskip.py` analyzer. Timings live in new `skip_timings`/`skip_fingerprints` tables, served via a new internal endpoint, blended by the catalog skip-times handler (detected > AniSkip), consumed by `useSkipTimes` with combo context.

**Tech Stack:** Go (content-verify, catalog), Python 3 + numpy + chromaprint/fpcalc (analyzer), Vue 3 + TS (frontend), GORM AutoMigrate, sqlite for tests.

**Spec:** `docs/superpowers/specs/2026-07-17-opskip-audio-detection-design.md` — read §2 (algorithm), §3 (bootstrap ownership), §4 (data model) before any task.

**Metrics:** UXΔ = +4 (Better) · CDI = 0.03 * 21 · MVQ = Kraken 90%/85%

## Global Constraints

- Head/tail analysis windows: **480s** each (`CV_SKIP_HEAD_WINDOW` / `CV_SKIP_TAIL_WINDOW`).
- Match bounds: length **[50s, 150s]** (`CV_SKIP_MIN_MATCH`/`CV_SKIP_MAX_MATCH`), per-frame bit-similarity threshold **0.75** (`CV_SKIP_SIM_THRESHOLD`).
- Statuses: `detected | no_match | pending_fp | unreachable` (consts in domain). `detected`/`no_match` are terminal; `pending_fp` re-dues after 6h; `unreachable` uses the existing `queue.Backoff(fails)`.
- Fingerprints are **per (anime, kind)** — NOT per provider; multiple allowed. Kinds: `op | ed`.
- Two-adjacent-failures rule: one `no_match` alone is terminal; two adjacent episodes `no_match` with `pair_tried=false` on the earlier → one pair re-bootstrap attempt, then `pair_tried=true` on both regardless of outcome.
- Kodik skip rows store the translation **TITLE** in `Team` (what the FE combo carries); the unit still carries the numeric ID for resolving.
- Skip units are claimed ONLY when the candidate's verify units are all settled (verify first).
- Serve only `detected` rows; catalog precedence: detected → AniSkip passthrough; detected responses reuse the AniSkip wire shape plus `"source":"detected"`.
- Adult group excluded (same rule as verify enumeration).
- content-verify stays **replicas: 1** (single-writer store).
- Effort metrics: never days/hours — UXΔ/CDI/MVQ only.

---

### Task 1: opskip.py analyzer (pair + locate) + chromaprint dep

**Files:**
- Create: `services/content-verify/analyzers/opskip.py`
- Modify: `services/content-verify/Dockerfile` (apt line — add `libchromaprint-tools`)
- Test: selftest mode inside the script (`--selftest`), invoked from Go test in Task 6; here verify by running it directly.

**Interfaces:**
- Produces CLI contract consumed by Task 6's runner method:
  - `python3 opskip.py pair <a.wav> <b.wav> --min 50 --max 150 --sim 0.75` → stdout JSON `{"found":bool,"a_start":f,"a_end":f,"b_start":f,"b_end":f,"similarity":f,"fp":[int,...]}`
  - `python3 opskip.py locate <ep.wav> <fps.json> --min 50 --max 150 --sim 0.75` → stdout JSON `{"found":bool,"start":f,"end":f,"similarity":f,"fp_index":int}` (`fps.json` = `[{"id":"<uuid>","fp":[int,...]}, ...]`)
  - `python3 opskip.py --selftest` → exit 0 / non-zero.
- Non-JSON diagnostics go to stderr only.

- [ ] **Step 1: Write the analyzer**

```python
#!/usr/bin/env python3
"""opskip — OP/ED audio fingerprint matching (content-verify skip lane).

pair   : cross-correlate two episodes' window fingerprints, emit the longest
         common segment (the OP/ED) with per-file bounds + its fp slice.
locate : find a stored fingerprint inside one episode's window.

Fingerprints come from chromaprint's fpcalc (-raw): int32 frames at ~8 fps.
Similarity = 1 - popcount(a XOR b)/32 per frame; a "hit" frame has
similarity >= --sim. Runs tolerate gaps <= GAP_FRAMES (~1s) so a single
noisy frame doesn't split the OP in two.
"""
import argparse
import json
import subprocess
import sys

import numpy as np

GAP_FRAMES = 8  # ~1s at chromaprint's ~8fps

POP = np.array([bin(i).count("1") for i in range(65536)], dtype=np.uint8)


def popcount32(x: np.ndarray) -> np.ndarray:
    x = x.astype(np.uint32)
    return POP[x & 0xFFFF].astype(np.uint16) + POP[(x >> 16) & 0xFFFF]


def fpcalc(path: str) -> tuple[np.ndarray, float]:
    """Raw chromaprint of a wav → (int32 frames, frames-per-second rate)."""
    out = subprocess.run(
        ["fpcalc", "-raw", "-json", path],
        capture_output=True, text=True, check=True,
    ).stdout
    data = json.loads(out)
    fp = np.array(data["fingerprint"], dtype=np.uint32)
    dur = float(data["duration"])
    if len(fp) == 0 or dur <= 0:
        raise ValueError(f"empty fingerprint for {path}")
    return fp, len(fp) / dur


def sim_series(a: np.ndarray, b: np.ndarray, lag: int) -> np.ndarray:
    """Per-frame similarity of a vs b shifted by lag (b[i+lag] matched to a[i])."""
    if lag >= 0:
        n = min(len(a), len(b) - lag)
        if n <= 0:
            return np.empty(0)
        d = popcount32(a[:n] ^ b[lag:lag + n])
    else:
        n = min(len(a) + lag, len(b))
        if n <= 0:
            return np.empty(0)
        d = popcount32(a[-lag:-lag + n] ^ b[:n])
    return 1.0 - d / 32.0


def longest_run(hits: np.ndarray, min_frames: int) -> tuple[int, int] | None:
    """Longest run of True allowing gaps <= GAP_FRAMES; None if < min_frames."""
    best, cur_start, gap, start = None, None, 0, None
    for i, h in enumerate(hits):
        if h:
            if start is None:
                start = i
            gap = 0
        elif start is not None:
            gap += 1
            if gap > GAP_FRAMES:
                end = i - gap
                if best is None or end - start > best[1] - best[0]:
                    best = (start, end)
                start, gap = None, 0
    if start is not None:
        end = len(hits) - 1
        while end > start and not hits[end]:
            end -= 1
        if best is None or end - start > best[1] - best[0]:
            best = (start, end)
    if best and best[1] - best[0] + 1 >= min_frames:
        return best
    return None


def best_common_segment(a, b, rate, args):
    """Scan all lags; return (a0, a1, lag, mean_sim) frame bounds of the
    longest common run within [min,max] length, or None."""
    min_f = int(args.min * rate)
    max_f = int(args.max * rate)
    best = None
    for lag in range(-(len(b) - min_f), len(b) - min_f):
        s = sim_series(a, b, lag)
        if len(s) < min_f:
            continue
        run = longest_run(s >= args.sim, min_f)
        if run is None:
            continue
        a_off = 0 if lag >= 0 else -lag
        r0, r1 = run[0] + a_off, run[1] + a_off
        if r1 - r0 + 1 > max_f:
            r1 = r0 + max_f - 1
        score = r1 - r0
        if best is None or score > best[0]:
            seg = s[run[0]:run[1] + 1]
            best = (score, r0, r1, lag, float(np.mean(seg)))
    if best is None:
        return None
    _, r0, r1, lag, ms = best
    return r0, r1, lag, ms


def cmd_pair(args):
    a, rate_a = fpcalc(args.files[0])
    b, rate_b = fpcalc(args.files[1])
    rate = (rate_a + rate_b) / 2
    seg = best_common_segment(a, b, rate, args)
    if seg is None:
        print(json.dumps({"found": False}))
        return
    a0, a1, lag, ms = seg
    print(json.dumps({
        "found": True,
        "a_start": a0 / rate, "a_end": (a1 + 1) / rate,
        "b_start": (a0 + lag) / rate, "b_end": (a1 + 1 + lag) / rate,
        "similarity": ms,
        "fp": [int(x) for x in a[a0:a1 + 1]],
    }))


def cmd_locate(args):
    ep, rate = fpcalc(args.files[0])
    with open(args.files[1]) as f:
        stored = json.load(f)
    best = None
    for idx, item in enumerate(stored):
        q = np.array(item["fp"], dtype=np.uint32)
        if len(q) < 8 or len(q) > len(ep):
            continue
        need = int(len(q) * 0.85)  # run must cover >=85% of the query
        for lag in range(0, len(ep) - len(q) + 1):
            s = sim_series(q, ep, lag)
            run = longest_run(s >= args.sim, need)
            if run is None:
                continue
            ms = float(np.mean(s[run[0]:run[1] + 1]))
            if best is None or ms > best[0]:
                best = (ms, lag, len(q), idx)
    if best is None:
        print(json.dumps({"found": False}))
        return
    ms, lag, qlen, idx = best
    print(json.dumps({
        "found": True,
        "start": lag / rate, "end": (lag + qlen) / rate,
        "similarity": ms, "fp_index": idx,
    }))


def selftest():
    rng = np.random.default_rng(7)
    rate = 8.0
    op = rng.integers(0, 2**32, size=int(90 * rate), dtype=np.uint32)

    def episode(op_at: float, total: float = 480.0):
        ep = rng.integers(0, 2**32, size=int(total * rate), dtype=np.uint32)
        i = int(op_at * rate)
        ep[i:i + len(op)] = op
        return ep

    a, b = episode(20.0), episode(65.0)

    class A:  # argparse stand-in
        min, max, sim = 50, 150, 0.75

    seg = best_common_segment(a, b, rate, A)
    assert seg is not None, "pair: common segment not found"
    a0, a1, lag, ms = seg
    assert abs(a0 / rate - 20.0) < 2, f"pair a_start off: {a0 / rate}"
    assert abs((a0 + lag) / rate - 65.0) < 2, f"pair b_start off: {(a0 + lag) / rate}"
    assert ms > 0.95, f"pair similarity low: {ms}"

    # no shared segment → not found
    assert best_common_segment(episode(20.0), rng.integers(0, 2**32, size=int(480 * rate), dtype=np.uint32), rate, A) is None

    print("selftest OK", file=sys.stderr)


def main():
    if "--selftest" in sys.argv:
        selftest()
        return
    p = argparse.ArgumentParser()
    p.add_argument("mode", choices=["pair", "locate"])
    p.add_argument("files", nargs=2)
    p.add_argument("--min", type=float, default=50)
    p.add_argument("--max", type=float, default=150)
    p.add_argument("--sim", type=float, default=0.75)
    args = p.parse_args()
    (cmd_pair if args.mode == "pair" else cmd_locate)(args)


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Dockerfile dep** — in `services/content-verify/Dockerfile`, extend the apt-get install line that already installs `python3 python3-pip` and ffmpeg with ` libchromaprint-tools`.

- [ ] **Step 3: Run the selftest** — `python3 services/content-verify/analyzers/opskip.py --selftest` on the host (numpy present from the verify analyzers' requirements; if fpcalc is absent on the host that's fine — selftest doesn't call fpcalc). Expected: `selftest OK`, exit 0.

- [ ] **Step 4: Commit** — `git add services/content-verify/analyzers/opskip.py services/content-verify/Dockerfile && git commit -m "feat(content-verify): opskip analyzer — chromaprint pair/locate matching"`

**Perf note for the implementer:** `best_common_segment`'s lag loop is O(lags × n) with n≈3840 — a few seconds in numpy, inside the skip budget. Do NOT try to be cleverer in v1.

---

### Task 2: skip domain + repo store

**Files:**
- Create: `services/content-verify/internal/domain/skip.go`
- Test: `services/content-verify/internal/domain/skip_test.go` (status consts sanity), `services/content-verify/internal/repo/skip_store_test.go`
- Create: `services/content-verify/internal/repo/skip_store.go`
- Modify: `services/content-verify/cmd/content-verify-api/main.go` (AutoMigrate the two new models next to the existing `domain.ContentVerification` migrate call)

**Interfaces:**
- Produces:

```go
// domain/skip.go
package domain

const (
	SkipDetected    = "detected"
	SkipNoMatch     = "no_match"
	SkipPendingFP   = "pending_fp"
	SkipUnreachable = "unreachable"

	SkipKindOp = "op"
	SkipKindEd = "ed"
)

// SkipTiming: one row per (anime × provider × team × episode) skip probe.
type SkipTiming struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())" json:"id"`
	AnimeID    string    `gorm:"type:uuid;uniqueIndex:idx_skip_unit" json:"anime_id"`
	Provider   string    `gorm:"size:64;uniqueIndex:idx_skip_unit" json:"provider"`
	Team       string    `gorm:"size:128;uniqueIndex:idx_skip_unit;default:''" json:"team,omitempty"`
	Episode    int       `gorm:"uniqueIndex:idx_skip_unit" json:"episode"`
	OpStart    float64   `json:"op_start"`
	OpEnd      float64   `json:"op_end"`
	EdStart    float64   `json:"ed_start"`
	EdEnd      float64   `json:"ed_end"`
	OpStatus   string    `gorm:"size:16" json:"op_status"`
	EdStatus   string    `gorm:"size:16" json:"ed_status"`
	Confidence float64   `json:"confidence"`
	PairTried  bool      `json:"pair_tried,omitempty"`
	Fails      int       `json:"fails,omitempty"`
	ProbedAt   time.Time `json:"probed_at"`
}

// SkipFingerprint: season fingerprint, several per (anime, kind) allowed.
type SkipFingerprint struct {
	ID         string    `gorm:"type:uuid;primaryKey;default:(gen_random_uuid())" json:"id"`
	AnimeID    string    `gorm:"type:uuid;index:idx_skip_fp_anime" json:"anime_id"`
	Kind       string    `gorm:"size:4;index:idx_skip_fp_anime" json:"kind"`
	Fp         FpInts    `gorm:"type:jsonb" json:"-"`
	Length     float64   `json:"length"`
	SourceNote string    `gorm:"size:128" json:"source_note"`
	CreatedAt  time.Time `json:"created_at"`
}

// FpInts serializes the raw chromaprint frames as JSON (sqlite + postgres).
type FpInts []uint32

func (f FpInts) Value() (driver.Value, error) { return json.Marshal(f) }
func (f *FpInts) Scan(src any) error { /* same switch as UnitList.Scan */ }

func (t *SkipTiming) BeforeCreate(*gorm.DB) error      // uuid fill, mirror ContentVerification
func (f *SkipFingerprint) BeforeCreate(*gorm.DB) error // uuid fill
```

```go
// repo/skip_store.go — methods on the EXISTING *Store type
func (s *Store) SkipByAnime(ctx context.Context, animeID string) ([]domain.SkipTiming, error)          // ordered provider, team, episode
func (s *Store) UpsertSkip(ctx context.Context, t domain.SkipTiming) error                              // upsert by (anime,provider,team,episode); preserves row ID
func (s *Store) Fingerprints(ctx context.Context, animeID string) ([]domain.SkipFingerprint, error)     // both kinds, oldest first
func (s *Store) AddFingerprint(ctx context.Context, fp domain.SkipFingerprint) error
```

- [ ] **Step 1: failing store test** — `repo/skip_store_test.go` mirroring `store_test.go`'s sqlite setup: upsert a timing, re-upsert same unit key with new statuses → still one row, updated values; `SkipByAnime` ordering; `AddFingerprint`+`Fingerprints` roundtrip preserving `Fp` ints.
- [ ] **Step 2: run** — `go test ./services/content-verify/internal/repo/` → FAIL (types missing).
- [ ] **Step 3: implement** domain/skip.go + repo/skip_store.go per the interfaces above (UpsertSkip: `First` by the four key columns; not-found → Create, else copy ID/CreatedAt and `Save`).
- [ ] **Step 4: AutoMigrate** — in main.go, next to the existing migrate: `db.DB.AutoMigrate(&domain.SkipTiming{}, &domain.SkipFingerprint{})` (match however ContentVerification is migrated there).
- [ ] **Step 5: run** — `go test ./services/content-verify/internal/...` → PASS; `go vet ./services/content-verify/...`.
- [ ] **Step 6: Commit** — `feat(content-verify): skip timing + fingerprint models and store`

---

### Task 3: audio window extraction + lowest-variant playlist

**Files:**
- Modify: `services/content-verify/internal/prober/extract.go` (add `ExtractWindow`)
- Modify: `services/content-verify/internal/prober/playlist.go` (variant selection)
- Test: extend `extract_test.go`, `playlist_test.go`

**Interfaces:**
- Produces `func ExtractWindow(ctx context.Context, ffmpegPath, input string, seek, durSec float64, name, dir string) (string, error)` — audio-only mono 16k wav named `<name>.wav` in `dir`; same `-allowed_extensions ALL` + `hlsPickyOpts` guards for `.m3u8` inputs, `-ss`/`-t` as INPUT options; no frames output.
- Produces `func LocalizeHLSVariant(ctx context.Context, hc *http.Client, gatewayBase, masterURL, dir string, lowest bool) (string, float64, error)`; existing `LocalizeHLS(...)` becomes a thin `lowest=false` wrapper so the verify path is untouched.

- [ ] **Step 1: failing playlist test** — master with three `#EXT-X-STREAM-INF` variants (BANDWIDTH 2000000/800000/300000, out of order) served by the test's httptest mux: `LocalizeHLSVariant(..., lowest=true)` must fetch the 300000 variant's media playlist; `lowest=false` keeps today's first-listed behavior (existing tests must stay green).
- [ ] **Step 2: implement** — in the master branch of the localizer, when `lowest` is set parse `BANDWIDTH=(\d+)` from `#EXT-X-STREAM-INF` lines and pick the URI following the smallest one (fall back to first when no BANDWIDTH attrs).
- [ ] **Step 3: failing extract test** — mirror the existing `ExtractFragment` test pattern (skip when ffmpeg absent): generate a 20s sine-wav input via ffmpeg `-f lavfi -i sine`, call `ExtractWindow(ctx, ffmpeg, input, 5, 10, "head", dir)` → file exists, ffprobe duration ≈10s, 1 channel.
- [ ] **Step 4: implement ExtractWindow** — same arg scaffolding as `ExtractFragment` minus the frames output: `-vn -ac 1 -ar 16000 -y <dir>/<name>.wav`.
- [ ] **Step 5: run** — `go test ./services/content-verify/internal/prober/` → PASS.
- [ ] **Step 6: Commit** — `feat(content-verify): audio window extraction + lowest-variant HLS localization`

---

### Task 4: skip unit enumeration (single fetch pass)

**Files:**
- Modify: `services/content-verify/internal/queue/enumerate.go`
- Test: extend `services/content-verify/internal/queue/enumerate_test.go`

**Interfaces:**
- Produces:

```go
// SkipUnit is one skip-probe target. TeamID resolves kodik streams; Team is
// the translation TITLE persisted in rows (FE combo carries titles).
type SkipUnit struct {
	AnimeID   string
	Provider  string
	Team      string // kodik: title; "" otherwise
	TeamID    int    // kodik: numeric id; 0 otherwise
	Episode   int
	EpisodeID string // scraper: per-episode opaque id; "" otherwise
	StateRank int
}

// EnumerateAll returns verify units AND skip units from ONE catalog pass.
// EnumerateUnits (existing signature) becomes a wrapper returning .Verify —
// existing callers/tests untouched.
type Enumeration struct {
	Verify []Unit
	Skip   []SkipUnit
}
func EnumerateAll(ctx context.Context, c *catalogclient.Client, animeID string, log *logger.Logger) (Enumeration, error)
```

- Skip enumeration rules (inside the SAME provider switch that builds verify units — reuse the already-fetched translations/episodes):
  - kodik: for each translation, episodes `1..maxInt(tr.EpisodesCount,1)`, pinned/roster order as returned; `Team=tr.Title, TeamID=tr.ID`.
  - scraper providers: one SkipUnit per episode from the already-fetched `eps` (`Episode=e.Number, EpisodeID=e.ID`), ascending by number. NO per-episode server fetches at enumeration (the prober resolves servers per episode on demand).
  - animejoy legs: one per episode number, ascending.
  - firstparty (`ae`) group: one per episode? — ae has no episode list in the capabilities call; SKIP ae in v1 (library titles are few; AniSkip fallback holds). Note this in a code comment.
  - adult: skipped (same as verify).

- [ ] **Step 1: failing test** — extend `TestEnumerateUnits`'s fixture server (it already serves kodik translations `episodes_count:28` and scraper episodes 1+28): call `EnumerateAll`, assert: gogoanime SkipUnits == 2 (episodes 1 and 28, EpisodeIDs `ep-1`/`ep-28`, ascending), kodik SkipUnits == 56 (2 translations × 28, `Team` = title, `TeamID` set), no `hanime`/`ae` skip units; `EnumerateUnits` still returns the same verify units as before.
- [ ] **Step 2: implement** — restructure `EnumerateUnits` body into `EnumerateAll`; keep the wrapper.
- [ ] **Step 3: run** — `go test ./services/content-verify/internal/queue/` → PASS.
- [ ] **Step 4: Commit** — `feat(content-verify): skip unit enumeration from the same catalog pass`

---

### Task 5: skip planner (pure task selection)

**Files:**
- Create: `services/content-verify/internal/queue/skipplan.go`
- Test: `services/content-verify/internal/queue/skipplan_test.go`

**Interfaces:**
- Produces:

```go
// SkipTask is what the worker executes: locate one episode, or pair two.
type SkipTask struct {
	Unit   SkipUnit
	Pair   *SkipUnit // non-nil => pair-bootstrap with this second episode
	RePair bool      // true when re-pairing two adjacent no_match rows
}

// NextSkipTask picks the next skip work item, or nil when the anime's skip
// lane is settled. Rules (spec §2.3, §3):
//  - rows keyed by (provider|team|episode); units grouped by family
//    (provider|team), episodes ascending within a family.
//  - due(unit): no row → due; pending_fp → due after 6h; unreachable → due
//    after Backoff(fails); detected/no_match → terminal.
//  - hasFP(kind) := any stored fingerprint of that kind exists (anime-level).
//  - If NO fingerprint exists for BOTH kinds: pair the first two due episodes
//    of the highest-StateRank family; if only one due episode exists overall,
//    return a locate task for it anyway (the prober records pending_fp).
//  - Else: locate task for the first due unit (family order, then episode).
//  - Re-pair: two ADJACENT episodes of one family both no_match with
//    PairTried=false on the earlier → SkipTask{Unit: earlier, Pair: &later,
//    RePair: true}. Checked BEFORE the due scan so self-heal wins.
func NextSkipTask(units []SkipUnit, rows []domain.SkipTiming, fps []domain.SkipFingerprint, now time.Time) *SkipTask
```

- [ ] **Step 1: failing tests** — table-driven `skipplan_test.go` covering: (a) empty rows + no fps → pair of episodes 1+2; (b) fps exist → locate of first missing episode; (c) all rows detected → nil; (d) adjacent no_match pair with PairTried=false → RePair task; PairTried=true → nil; (e) single due episode + no fps → locate task (not nil); (f) pending_fp row older than 6h → due again, younger → not; (g) unreachable row respects `Backoff(fails)`; (h) family ordering: kodik pinned team's episodes before the second team's.
- [ ] **Step 2: run** → FAIL.
- [ ] **Step 3: implement** `skipplan.go` per the rules above (pure function; group with a `map[string][]SkipUnit` keyed `provider+"|"+team`, preserve first-seen family order via a slice).
- [ ] **Step 4: run** → PASS.
- [ ] **Step 5: Commit** — `feat(content-verify): skip task planner (pair/locate/re-pair selection)`

---

### Task 6: skip prober

**Files:**
- Create: `services/content-verify/internal/prober/skipprobe.go`
- Modify: `services/content-verify/internal/prober/runner.go` (add `Opskip` to `AnalyzerRunner` + execRunner)
- Test: `services/content-verify/internal/prober/skipprobe_test.go` (fake runner + httptest catalog, mirroring `prober_test.go`), plus a gated real-python test running `opskip.py --selftest` (skip when `python3` absent).

**Interfaces:**
- Consumes: `catalogclient.Client` streams (`KodikStream/ScraperStream/ScraperServers/AnimejoyStream`), `LocalizeHLSVariant(lowest=true)`, `ExtractWindow`, Task-5 `queue.SkipTask`, Task-2 store (via interfaces below).
- Produces:

```go
// runner.go additions
type OpskipPair struct {
	Found  bool    `json:"found"`
	AStart float64 `json:"a_start"`; AEnd float64 `json:"a_end"`
	BStart float64 `json:"b_start"`; BEnd float64 `json:"b_end"`
	Similarity float64 `json:"similarity"`
	Fp     []uint32 `json:"fp"`
}
type OpskipLocate struct {
	Found bool `json:"found"`
	Start float64 `json:"start"`; End float64 `json:"end"`
	Similarity float64 `json:"similarity"`
	FpIndex int `json:"fp_index"`
}
// AnalyzerRunner gains:
OpskipPair(ctx context.Context, a, b string, minS, maxS, sim float64) (*OpskipPair, error)
OpskipLocate(ctx context.Context, wav, fpsJSON string, minS, maxS, sim float64) (*OpskipLocate, error)
```

```go
// skipprobe.go
type SkipConfig struct {
	HeadWindow, TailWindow time.Duration // 480s defaults live in config, not here
	MinMatch, MaxMatch     time.Duration
	SimThreshold           float64
}
type FingerprintStore interface {
	Fingerprints(ctx context.Context, animeID string) ([]domain.SkipFingerprint, error)
	AddFingerprint(ctx context.Context, fp domain.SkipFingerprint) error
}
type SkipProber struct { /* cat, gateway, ffmpeg, workDir, runner, fps FingerprintStore, cfg SkipConfig, log, now, retryWait */ }
func NewSkipProber(cat *catalogclient.Client, gatewayURL, ffmpegPath, workDir string, runner AnalyzerRunner, fps FingerprintStore, cfg SkipConfig, log *logger.Logger) *SkipProber
// Probe never errors; returns the rows to upsert (1 for locate, 2 for pair).
func (p *SkipProber) Probe(ctx context.Context, t queue.SkipTask, prevFails int) []domain.SkipTiming
```

- Probe flow (write it exactly like Prober.Probe's structure — temp dir, resolve with the same 3×25s retry policy factored into a shared helper or duplicated verbatim):
  1. `resolve(unit)`: kodik → `KodikStream(anime, ep, TeamID)`; scraper (`EpisodeID != ""`) → `ScraperServers(anime, EpisodeID, provider)` → first server with `Type != "dub"` (else first) → `ScraperStream(anime, EpisodeID, serverID, "sub", provider)`; animejoy → `AnimejoyStream`. Resolve failure → single row `{OpStatus, EdStatus: SkipUnreachable, Fails: prevFails+1}` (budget-ctx expiry → statuses `SkipPendingFP` with Fails untouched, mirroring verify's budget rule).
  2. HLS → `LocalizeHLSVariant(lowest=true)` for input + duration; mp4 → proxied URL direct, duration unknown → tail seek uses ffprobe? NO — for mp4 without duration, extract tail via `-sseof -480` (ffmpeg input option, works on seekable mp4): `ExtractWindow` gains support: negative seek → emit `-sseof <seek>` instead of `-ss`.
  3. Extract head (`seek 0, dur HeadWindow`) and tail (`seek duration-TailWindow`, or `-480` via sseof for mp4) for the unit — and for `t.Pair`'s episode too in pair mode (resolve it the same way).
  4. Pair mode: `OpskipPair(headA, headB)` → found → `AddFingerprint(kind op, fp, note "<provider> ep<A>+ep<B>")` + both rows' Op windows detected (tail windows likewise for ed; tail times are absolute: `duration - TailWindow + t`). Not found → both rows' side = `SkipNoMatch`; in RePair mode set `PairTried: true` on both rows regardless.
  5. Locate mode: write `fps.json` (`[{"id","fp"}...]` filtered per kind) into the temp dir, `OpskipLocate(head, fpsOp)` / `OpskipLocate(tail, fpsEd)`; found → `SkipDetected` with absolute times + `Confidence: similarity`; not found → `SkipNoMatch`; no fingerprints of a kind stored at all → that side `SkipPendingFP`.
- [ ] **Step 1: failing tests** — fake runner returning canned pair/locate JSON; httptest catalog serving a tiny local m3u8; assert: locate-found row (absolute tail math: duration 1440, TailWindow 480, locate start 100 → EdStart 1060); pair-found adds fingerprint + returns 2 detected rows; resolve failure → unreachable + Fails bump; budget-expired ctx → pending_fp, Fails untouched; RePair not-found → PairTried on both.
- [ ] **Step 2: run** → FAIL. **Step 3: implement.** **Step 4: run** → PASS (whole `./services/content-verify/...`).
- [ ] **Step 5: Commit** — `feat(content-verify): skip prober (pair bootstrap + locate) with opskip runner`

---

### Task 7: engine + worker + config integration

**Files:**
- Modify: `services/content-verify/internal/queue/engine.go` (Claim returns skip tasks when verify lane settles)
- Modify: `services/content-verify/internal/service/worker.go` + `worker_test.go`
- Modify: `services/content-verify/internal/config/config.go` + `config_test.go`
- Modify: `services/content-verify/cmd/content-verify-api/main.go` (wire SkipProber + config)
- Modify: `services/content-verify/internal/cvmetrics/` (add `SkipProbesTotal` counter vec `{provider,result}`)

**Interfaces:**
- `Engine.Claim` new signature: `Claim(ctx) (*Unit, *SkipTask, bool, error)` — per candidate: verify pending → verify unit (skip task nil); else if skip enabled → `EnumerateAll`'s skip units + `store.SkipByAnime` + `store.Fingerprints` → `NextSkipTask`; non-nil → return it; both empty → cooldown as today. Engine gains `skipEnabled bool` (constructor param) and store methods (already on `*repo.Store`).
- Worker: `Claimer` interface updated to the new signature; `SkipProber` interface `ProbeSkip(ctx, t queue.SkipTask, prevFails int) []domain.SkipTiming`; `SkipStore` interface `{UpsertSkip; SkipByAnime}` for prevFails lookup (max Fails across the task's unit row). Skip probes run under `skipBudget` ctx; each returned row upserted; metric `SkipProbesTotal{provider, opStatus}` inc.
- Config: `SkipEnabled bool` (`CV_SKIP_ENABLED`, default true), `SkipBudget time.Duration` (`CV_SKIP_BUDGET`, 480s), `SkipHeadWindow/SkipTailWindow` (480s), `SkipMinMatch` (50s), `SkipMaxMatch` (150s), `SkipSimThreshold` (`CV_SKIP_SIM_THRESHOLD`, 0.75).

- [ ] **Step 1: failing worker tests** — extend fakes to the new Claimer signature; new cases: claimer returns a SkipTask → skip prober called with prevFails from the existing row, rows persisted via UpsertSkip, verify prober NOT called; skip claim while shed → skipped (existing shed test covers the gate before claim).
- [ ] **Step 2: failing engine test** — engine fixture (existing `buildTestCatalog`): all verify units already verified in store → Claim returns a SkipTask (kodik pair of ep1+ep2 given no fingerprints); with `skipEnabled=false` → Claim returns all-nil + cooldown set (assert via the existing cooldown fake).
- [ ] **Step 3: implement** engine/worker/config/metrics/main wiring. **Step 4:** `go test ./services/content-verify/...` + `go vet` → PASS.
- [ ] **Step 5: Commit** — `feat(content-verify): skip lane in claim/worker loop, config + metrics`

---

### Task 8: internal skip endpoint

**Files:**
- Modify: `services/content-verify/internal/handler/verify.go` (+`Skip` method), `services/content-verify/internal/transport/router.go` (`r.Get("/internal/verify/skip", h.Skip)`)
- Test: extend `services/content-verify/internal/handler/verify_test.go`

**Interfaces:**
- `GET /internal/verify/skip?anime_id=<uuid>` → `{"success":true,"data":{"anime_id":"...","timings":[SkipTiming...]}}` (JSON tags from Task 2; empty list, not null, when none). Missing `anime_id` → 400. Handler needs the store's `SkipByAnime` — extend the handler's store interface the same way Verdicts uses `ByAnime`.

- [ ] **Step 1: failing handler test** (sqlite store seeded with 2 rows; assert order + empty-list shape). **Step 2: implement + route.** **Step 3: run** → PASS. **Step 4: Commit** — `feat(content-verify): internal skip timings endpoint`

---

### Task 9: catalog blending (detected > AniSkip)

**Files:**
- Modify: `services/catalog/internal/service/capability/verify_client.go` (+`SkipTimings` method on VerifyClient)
- Modify: `services/catalog/internal/handler/skip_times.go` + `skip_times_test.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` + `services/catalog/internal/transport/router.go` ONLY if the handler's constructor signature changes (it does — it gains the skip source).

**Interfaces:**
- `VerifyClient.SkipTimings(ctx, animeID) []SkipTimingRow` — best-effort nil on any failure (mirror `Summaries`); `SkipTimingRow` struct in the capability package mirroring Task 2's JSON (`provider, team, episode, op_start, op_end, ed_start, ed_end, op_status, ed_status`).
- `SkipTimesHandler` gains an optional `skip SkipSource` dep where `type SkipSource interface { SkipTimings(ctx context.Context, animeID string) []capability.SkipTimingRow }` (nil = feature off, pure AniSkip behavior).
- Handler `Get`: read optional query params `anime` (uuid), `provider`, `team`. When all of anime+provider present AND a row matches `(provider, team, episode)` with a `detected` side → respond `{found:true, source:"detected", results:[...]}` building one `SkipTimesResultItem` per detected side (`skipType` `"op"`/`"ed"`, `interval{startTime,endTime}`, `episodeLength:0`, `skipId:""`). Cache key `skip-times:detected:<anime>:<provider>:<team>:<ep>` TTL 10min. No match / params absent → existing AniSkip path unchanged (existing tests must stay green).
- Wire response gains optional `"source"` field on `SkipTimesResult` (`json:"source,omitempty"`; aniskip path leaves it empty).

- [ ] **Step 1: failing handler tests** — fake SkipSource: (a) detected op+ed row → both results, `source:"detected"`, correct intervals; (b) row with op detected + ed no_match → only op result; (c) no matching row → AniSkip upstream called (existing httptest upstream fixture); (d) params absent → AniSkip path, upstream URL unchanged.
- [ ] **Step 2: implement**, wire `NewSkipTimesHandler(cache, log, verifyClient)` in main/router (pass nil-safe when content-verify disabled — reuse the existing verify client enable flag).
- [ ] **Step 3: run** — `go test ./services/catalog/...` → PASS. **Step 4: Commit** — `feat(catalog): blend detected skip windows ahead of AniSkip`

---

### Task 10: FE — combo-aware skip times

**Files:**
- Modify: `frontend/web/src/composables/useSkipTimes.ts`, `frontend/web/src/composables/aePlayer/useSkipIntro.ts`, `frontend/web/src/api/client.ts` (skip-times call gains optional params), `frontend/web/src/components/player/aePlayer/AePlayer.vue` (pass combo context into useSkipIntro deps)
- Test: `frontend/web/src/composables/useSkipTimes.spec.ts` (or extend existing), `skipSegments.spec.ts` untouched

**Interfaces:**
- `useSkipTimes(malId, episode, combo?: Ref<{ animeId?: string; provider?: string; team?: string | null } | null>)` — appends `?anime=&provider=&team=` when present; refetches when the combo ref changes (provider/team switch → per-encode timings differ).
- `useSkipIntro` deps gain `getCombo: () => { animeId?: string; provider?: string; team?: string | null } | null`; AePlayer supplies it from `state.combo.value` + anime id.
- Everything downstream (`segmentsToChapters`, `activeSkipSegment`, auto-skip) consumes the same `{opening, ending}` refs — no changes.

- [ ] **Step 1: failing spec** — mock api client; assert the query params are passed when combo present, omitted when null; assert a combo change triggers refetch (existing race-token pattern).
- [ ] **Step 2: implement.** **Step 3:** `bin/ae-fe-verify.sh` (DS-lint, eslint, build, vitest) → ALL PASS. **Step 4: Commit** — `feat(aeplayer): per-encode skip windows via combo-aware skip-times`

---

### Task 11: docs + deploy

- [ ] Add the `CV_SKIP_*` block to `docs/environment-variables.md` (content-verify section).
- [ ] Note the new tables + endpoint in the spec's §9 if anything drifted during implementation.
- [ ] Run `/animeenigma-after-update` (redeploys content-verify + catalog + web; Trump-mode changelog: automatic OP/ED skip; commits + pushes).
- [ ] Live verification per spec §9: `docker logs animeenigma-content-verify` for `skip` lane activity; pick a queue-top anime, wait for a pair bootstrap, then `curl /internal/verify/skip?anime_id=` and check the FE skip chip lands correctly vs manual scrub.

## Self-review notes

- Type names cross-checked: `queue.SkipUnit/SkipTask` (T4/T5) consumed by T6/T7; `domain.SkipTiming` (T2) consumed by T5/T6/T7/T8; `OpskipPair/OpskipLocate` (T6) match opskip.py's JSON (T1).
- `EnumerateUnits` keeps its exact old signature (wrapper) so T4 can't break existing engine/tests before T7 lands.
- ae (firstparty) intentionally has no skip lane in v1 (T4 note); movies/single-episode = `pending_fp`-then-stall by design (spec §8).
- mp4 tail seek uses `-sseof` (T6 step 2) — no ffprobe dependency added.
