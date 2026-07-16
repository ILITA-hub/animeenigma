package prober

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

type AnalyzerRunner interface {
	LID(ctx context.Context, wavs []string) (*LIDResult, error)
	Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error)
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
