// Package toolrun defines the Runner interface and its concrete implementation.
// It is the single choke point for all subprocess execution in archfit.
// Only this package may import os/exec — the arch_test gate enforces this.

//go:generate moq -out runner_moq.go . Runner

package toolrun

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// ToolInfo describes a detected external tool.
type ToolInfo struct {
	Name    string
	Path    string
	Version string
}

// ToolCmd describes a subprocess invocation.
type ToolCmd struct {
	Name    string
	Args    []string
	Env     []string
	Timeout time.Duration
	// WorkDir sets the working directory for the subprocess.
	// When empty the current process directory is used.
	WorkDir string
}

// Output holds the result of a subprocess invocation.
// A non-zero exit code is recorded in ExitCode, not returned as an error.
// Error is reserved for exec failures (binary not found, I/O error, etc.).
type Output struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner is the process boundary for all external tool invocations.
// Adapters (extract/*, history/git) depend on this interface so they can be
// tested with a mock without touching os/exec.
type Runner interface {
	// Detect checks whether tool is available on PATH and returns its info.
	// Returns (ToolInfo{}, false) if the tool is not found.
	Detect(ctx context.Context, tool string) (ToolInfo, bool)

	// Run executes cmd and returns its captured output.
	// A non-zero exit code is recorded in Output.ExitCode — it is NOT an error.
	// Error is returned only for exec-level failures (binary missing, I/O error).
	Run(ctx context.Context, cmd ToolCmd) (Output, error)
}

// ToolRunner is the concrete Runner implementation.
// It is the only type in the codebase that may use os/exec.
type ToolRunner struct{}

// New returns a new ToolRunner.
func New() *ToolRunner {
	return &ToolRunner{}
}

// Detect uses exec.LookPath to find tool on PATH.
func (r *ToolRunner) Detect(_ context.Context, tool string) (ToolInfo, bool) {
	path, err := exec.LookPath(tool)
	if err != nil {
		return ToolInfo{}, false
	}
	return ToolInfo{Name: tool, Path: path, Version: ""}, true
}

// Run executes cmd.Name with cmd.Args.
// It pins LC_ALL=C and TZ=UTC for deterministic output, then appends any
// caller-supplied cmd.Env on top. If cmd.Timeout > 0 a deadline is applied
// via context.WithTimeout.
// Non-zero exit codes are recorded in Output.ExitCode, not returned as errors.
func (r *ToolRunner) Run(ctx context.Context, cmd ToolCmd) (Output, error) {
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}

	// Resolve real path via Detect so callers can use bare tool names.
	info, ok := r.Detect(ctx, cmd.Name)
	if !ok {
		return Output{}, &exec.Error{Name: cmd.Name, Err: exec.ErrNotFound}
	}

	c := exec.CommandContext(ctx, info.Path, cmd.Args...) //nolint:gosec // path is resolved via LookPath in Detect; args are caller-controlled by design

	// Set working directory when specified (required for language extractors that
	// must run from the project root, e.g. dependency-cruiser, grimp).
	if cmd.WorkDir != "" {
		c.Dir = cmd.WorkDir
	}

	// Pin locale and timezone for deterministic output; caller env appended last
	// so callers can override if needed.
	c.Env = append([]string{"LC_ALL=C", "TZ=UTC"}, cmd.Env...)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	out := Output{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}

	if err != nil {
		// Context cancellation / timeout takes priority: the exit code is
		// meaningless (process was killed), so surface the context error.
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Non-zero exit — record code, not an error.
			out.ExitCode = exitErr.ExitCode()
			return out, nil
		}
		// Real exec failure (binary gone, I/O error, etc.).
		return out, err
	}

	return out, nil
}
