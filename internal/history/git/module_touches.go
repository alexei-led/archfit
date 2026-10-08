package git

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const maxCommitsModuleTouches = 500

// ModuleTouchStatus reports whether git-history corroboration was available.
type ModuleTouchStatus string

// ModuleTouchStatus values.
const (
	ModuleTouchStatusOK      ModuleTouchStatus = "ok"
	ModuleTouchStatusTimeout ModuleTouchStatus = "timeout"
	ModuleTouchStatusError   ModuleTouchStatus = "error"
)

// ModuleTouches summarizes module-level touch frequency from git history.
// Counts are commit counts per module: a commit touching multiple files in one
// module increments that module once.
type ModuleTouches struct {
	Status          ModuleTouchStatus
	CommitWindow    int
	FullHistory     bool
	CommitsScanned  int
	TouchedByModule map[string]int
}

// TouchCounts summarizes recent git-history touch frequency for declared
// modules. It uses a bounded recent-history pass first and falls back to full
// history only when the bounded pass found no module data. Failures are
// report-only: failed or timed-out history never breaks analysis.
func TouchCounts(ctx context.Context, workDir, subtreePrefix string, moduleFor func(string) (string, bool), runner toolrun.Runner) ModuleTouches {
	result := runTouchCounts(ctx, workDir, subtreePrefix, moduleFor, runner, maxCommitsModuleTouches)
	if result.Status != ModuleTouchStatusOK || result.CommitsScanned == 0 {
		return result
	}
	result.CommitWindow = maxCommitsModuleTouches
	if len(result.TouchedByModule) > 0 {
		return result
	}
	fallback := runTouchCounts(ctx, workDir, subtreePrefix, moduleFor, runner, 0)
	if fallback.Status != ModuleTouchStatusOK {
		result.Status = fallback.Status
		return result
	}
	fallback.FullHistory = fallback.CommitsScanned > 0
	return fallback
}

func runTouchCounts(ctx context.Context, workDir, subtreePrefix string, moduleFor func(string) (string, bool), runner toolrun.Runner, maxCommits int) ModuleTouches {
	args := []string{"log", "--format=%x00%H", "--name-only", "-z"}
	if maxCommits > 0 {
		args = append(args, "-n", strconv.Itoa(maxCommits))
	}
	if subtreePrefix != "" {
		args = append(args, "--", subtreePrefix)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    gitTool,
		Args:    args,
		Timeout: gitTimeout,
		WorkDir: workDir,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return ModuleTouches{Status: ModuleTouchStatusTimeout}
	}
	if err != nil {
		return ModuleTouches{Status: ModuleTouchStatusError}
	}
	if out.ExitCode != 0 {
		if out.ExitCode == 128 {
			return ModuleTouches{Status: unbornHistoryStatus(ctx, workDir, runner)}
		}
		return ModuleTouches{Status: ModuleTouchStatusError}
	}

	counts := make(map[string]int)
	currentTouched := map[string]struct{}{}
	commits := 0
	expectCommit, firstFile := false, false
	flush := func() {
		if len(currentTouched) == 0 {
			return
		}
		for mod := range currentTouched {
			counts[mod]++
		}
		clear(currentTouched)
	}

	// The extra NUL before each header distinguishes commits from filenames.
	for _, field := range strings.Split(string(out.Stdout), "\x00") {
		if field == "" {
			expectCommit = true
			continue
		}
		if expectCommit {
			flush()
			commits++
			expectCommit, firstFile = false, true
			continue
		}
		gitRel := field
		if firstFile {
			gitRel = strings.TrimPrefix(gitRel, "\n")
			firstFile = false
		}
		scanRel := gitRel
		if subtreePrefix != "" {
			trimmed := strings.TrimPrefix(gitRel, subtreePrefix+"/")
			if trimmed == gitRel {
				continue
			}
			scanRel = trimmed
		}
		if mod, ok := moduleFor(scanRel); ok {
			currentTouched[mod] = struct{}{}
		}
	}
	flush()

	return ModuleTouches{Status: ModuleTouchStatusOK, TouchedByModule: counts, CommitsScanned: commits}
}

func unbornHistoryStatus(ctx context.Context, workDir string, runner toolrun.Runner) ModuleTouchStatus {
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name: gitTool, Args: []string{"symbolic-ref", "--quiet", "HEAD"},
		WorkDir: workDir, Timeout: gitTimeout,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return ModuleTouchStatusTimeout
	}
	branch := strings.TrimSpace(string(out.Stdout))
	if err != nil || out.ExitCode != 0 || !strings.HasPrefix(branch, "refs/heads/") {
		return ModuleTouchStatusError
	}
	out, err = runner.Run(ctx, toolrun.ToolCmd{
		Name: gitTool, Args: []string{"show-ref", "--verify", "--quiet", branch},
		WorkDir: workDir, Timeout: gitTimeout,
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return ModuleTouchStatusTimeout
	}
	if err == nil && out.ExitCode == 1 {
		return ModuleTouchStatusOK
	}
	return ModuleTouchStatusError
}

// RankedModules returns the touched modules sorted by descending commit count,
// then alphabetically for determinism.
func (m ModuleTouches) RankedModules() []string {
	mods := make([]string, 0, len(m.TouchedByModule))
	for mod := range m.TouchedByModule {
		mods = append(mods, mod)
	}
	sort.SliceStable(mods, func(i, j int) bool {
		if m.TouchedByModule[mods[i]] != m.TouchedByModule[mods[j]] {
			return m.TouchedByModule[mods[i]] > m.TouchedByModule[mods[j]]
		}
		return mods[i] < mods[j]
	})
	return mods
}
