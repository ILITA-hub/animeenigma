package prober

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
)

type AnalyzerRunner interface {
	LID(ctx context.Context, wavs []string) (*LIDResult, error)
	Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error)
	// OpskipPair cross-correlates two episodes' head/tail window
	// fingerprints (opskip.py "pair" mode) and returns the longest common
	// segment, if any.
	OpskipPair(ctx context.Context, a, b string, minS, maxS, sim float64) (*OpskipPair, error)
	// OpskipLocate finds a stored season fingerprint inside one episode's
	// window (opskip.py "locate" mode).
	OpskipLocate(ctx context.Context, wav, fpsJSON string, minS, maxS, sim float64) (*OpskipLocate, error)
}

// OpskipPair mirrors opskip.py's `pair` mode JSON output.
type OpskipPair struct {
	Found bool `json:"found"`
	// Duplicate marks a not-found whose cause is the two inputs being the
	// SAME content (provider episode-mapping bug) — not a fingerprint-worthy
	// comparison and not evidence the episodes lack an OP/ED.
	Duplicate  bool     `json:"duplicate"`
	AStart     float64  `json:"a_start"`
	AEnd       float64  `json:"a_end"`
	BStart     float64  `json:"b_start"`
	BEnd       float64  `json:"b_end"`
	Similarity float64  `json:"similarity"`
	Fp         []uint32 `json:"fp"`
}

// OpskipLocate mirrors opskip.py's `locate` mode JSON output.
type OpskipLocate struct {
	Found      bool    `json:"found"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Similarity float64 `json:"similarity"`
	FpIndex    int     `json:"fp_index"`
}

type execRunner struct {
	python string
	dir    string
}

func NewExecRunner(python, analyzersDir string) AnalyzerRunner {
	return &execRunner{python: python, dir: analyzersDir}
}

func (r *execRunner) run(ctx context.Context, script string, args []string, dst any) error {
	argv := append([]string{filepath.Join(r.dir, script)}, args...)
	cmd := exec.CommandContext(ctx, r.python, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{w: &stderr, n: 2048}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\nstderr tail:\n%s", script, err, stderr.String())
	}
	return json.Unmarshal(stdout.Bytes(), dst)
}

func (r *execRunner) LID(ctx context.Context, wavs []string) (*LIDResult, error) {
	var out LIDResult
	if err := r.run(ctx, "lid.py", wavs, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *execRunner) Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error) {
	var out HardsubResult
	if err := r.run(ctx, "hardsub.py", []string{framesDir}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// opskipArgs formats the shared --min/--max/--sim flags. The runner passes
// floats straight through (no rounding) so a tuned MinMatch/MaxMatch/
// SimThreshold config value survives the CLI round-trip exactly.
func opskipArgs(mode string, files []string, minS, maxS, sim float64) []string {
	args := append([]string{mode}, files...)
	return append(args,
		"--min", strconv.FormatFloat(minS, 'f', -1, 64),
		"--max", strconv.FormatFloat(maxS, 'f', -1, 64),
		"--sim", strconv.FormatFloat(sim, 'f', -1, 64),
	)
}

func (r *execRunner) OpskipPair(ctx context.Context, a, b string, minS, maxS, sim float64) (*OpskipPair, error) {
	var out OpskipPair
	if err := r.run(ctx, "opskip.py", opskipArgs("pair", []string{a, b}, minS, maxS, sim), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *execRunner) OpskipLocate(ctx context.Context, wav, fpsJSON string, minS, maxS, sim float64) (*OpskipLocate, error) {
	var out OpskipLocate
	if err := r.run(ctx, "opskip.py", opskipArgs("locate", []string{wav, fpsJSON}, minS, maxS, sim), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
