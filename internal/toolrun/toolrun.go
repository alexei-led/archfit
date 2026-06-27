// Package toolrun defines the Runner interface and its concrete implementation.
// It is the single choke point for all subprocess execution in archfit.
// Only this package may import os/exec — the arch_test gate enforces this.

//go:generate moq -out runner_moq.go . Runner

package toolrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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
// For Stream calls, Stdout is nil — bytes were consumed by the callback.
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

	// Stream executes cmd and calls consume with the live process stdout.
	// consume must drain r to EOF on success; returning non-nil aborts the
	// process via context cancellation before Wait. Output.Stdout is always nil —
	// bytes were consumed by the callback. Stderr is always captured.
	// A non-zero exit code is recorded in Output.ExitCode and is not an error
	// (same contract as Run).
	Stream(ctx context.Context, cmd ToolCmd, consume func(io.Reader) error) (Output, error)
}

const gitCommand = "git"

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

// buildCmd creates an exec.Cmd from cmd, applying timeout, path resolution,
// WorkDir, and environment pinning. The returned cancel must always be called;
// on error the returned cancel is a no-op safe to call without effect.
func (r *ToolRunner) buildCmd(parentCtx context.Context, cmd ToolCmd) (context.Context, *exec.Cmd, context.CancelFunc, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	if cmd.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parentCtx, cmd.Timeout)
	} else {
		ctx, cancel = context.WithCancel(parentCtx)
	}

	// Resolve real path via Detect so callers can use bare tool names.
	info, ok := r.Detect(ctx, cmd.Name)
	if !ok {
		cancel() // don't leak the derived context
		return nil, nil, func() {}, &exec.Error{Name: cmd.Name, Err: exec.ErrNotFound}
	}

	c := exec.CommandContext(ctx, info.Path, cmd.Args...) //nolint:gosec // path is resolved via LookPath in Detect; args are caller-controlled by design

	// Set working directory when specified (required for language extractors that
	// must run from the project root, e.g. dependency-cruiser, grimp).
	if cmd.WorkDir != "" {
		c.Dir = cmd.WorkDir
	}

	// Inherit the parent process environment for tools that need PATH, GOPATH,
	// HOME, etc. Git commands that target a specific WorkDir must not inherit
	// hook-provided repo locators (GIT_DIR, GIT_WORK_TREE, …), otherwise a push
	// hook can make git treat an unrelated temp dir as the current repo.
	env := os.Environ()
	if cmd.Name == gitCommand && cmd.WorkDir != "" {
		env = scrubGitRepoEnv(env)
	}
	// Pin locale and timezone on top for deterministic output; caller env
	// appended last so callers can override if needed.
	env = append(env, "LC_ALL=C", "TZ=UTC")
	env = append(env, cmd.Env...)
	c.Env = env

	return ctx, c, cancel, nil
}

// Run executes cmd and returns its captured output.
// It pins LC_ALL=C and TZ=UTC for deterministic output, then appends any
// caller-supplied cmd.Env on top. If cmd.Timeout > 0 a deadline is applied
// via context.WithTimeout.
// Non-zero exit codes are recorded in Output.ExitCode, not returned as errors.
func (r *ToolRunner) Run(ctx context.Context, cmd ToolCmd) (Output, error) {
	cmdCtx, c, cancel, err := r.buildCmd(ctx, cmd)
	if err != nil {
		return Output{}, err
	}
	defer cancel()

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	runErr := c.Run()
	out := Output{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}

	if runErr != nil {
		// Context cancellation / timeout takes priority: the exit code is
		// meaningless (process was killed), so surface the context error.
		if cmdCtx.Err() != nil {
			return out, cmdCtx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			// Non-zero exit — record code, not an error.
			out.ExitCode = exitErr.ExitCode()
			return out, nil
		}
		// Real exec failure (binary gone, I/O error, etc.).
		return out, runErr
	}

	return out, nil
}

// Stream executes cmd and calls consume with the live stdout stream.
// The process is started, consume is called with its stdout, then Wait is
// called. If consume returns non-nil the process is cancelled (SIGKILL via
// context) before Wait so it cannot deadlock on a full pipe.
// Remaining unread stdout bytes are drained after consume returns.
func (r *ToolRunner) Stream(ctx context.Context, cmd ToolCmd, consume func(io.Reader) error) (Output, error) {
	cmdCtx, c, cancel, err := r.buildCmd(ctx, cmd)
	if err != nil {
		return Output{}, err
	}
	defer cancel()

	var stderrBuf bytes.Buffer
	c.Stderr = &stderrBuf

	// Backstop: if the process does not exit within this window after its context
	// is cancelled, os/exec forces the process group to stop.
	c.WaitDelay = 10 * time.Second

	stdout, err := c.StdoutPipe()
	if err != nil {
		return Output{}, fmt.Errorf("toolrun: stdout pipe for %s: %w", cmd.Name, err)
	}

	if err := c.Start(); err != nil {
		return Output{}, fmt.Errorf("toolrun: start %s: %w", cmd.Name, err)
	}

	consumeErr := consume(stdout)
	if consumeErr != nil {
		// Kill the process so the pipe drains quickly and Wait does not block.
		cancel()
	}
	// Drain any unread bytes so the child is not blocked writing to a full pipe.
	_, _ = io.Copy(io.Discard, stdout)

	waitErr := c.Wait()
	out := Output{Stderr: stderrBuf.Bytes()}

	if consumeErr != nil {
		return out, consumeErr
	}
	if cmdCtx.Err() != nil {
		return out, cmdCtx.Err()
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			out.ExitCode = exitErr.ExitCode()
			return out, nil
		}
		return out, waitErr
	}
	return out, nil
}

// WithWatchdog returns a derived context that is cancelled after the given
// timeout. If timeout is zero or negative, def is used instead. The caller
// must call the returned cancel function to release resources.
//
// Usage:
//
//	ctx, cancel := toolrun.WithWatchdog(ctx, cfg.Timeout, defaultTimeout)
//	defer cancel()
func WithWatchdog(ctx context.Context, timeout, def time.Duration) (context.Context, context.CancelFunc) {
	to := timeout
	if to <= 0 {
		to = def
	}
	return context.WithTimeout(ctx, to)
}

func scrubGitRepoEnv(env []string) []string {
	blocked := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_COMMON_DIR":                   true,
		"GIT_PREFIX":                       true,
		"GIT_INDEX_FILE":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	}

	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && blocked[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
