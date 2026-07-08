package git

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/toolrun"
)

const maxCommitsModuleTouches = 500

// ModuleTouchStatus reports whether git-history corroboration was available.
type ModuleTouchStatus string

// ModuleTouchStatus values.
const (
	ModuleTouchStatusOK          ModuleTouchStatus = "ok"
	ModuleTouchStatusTimeout     ModuleTouchStatus = "timeout"
	ModuleTouchStatusUnavailable ModuleTouchStatus = "unavailable"
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
// report-only: unavailable or timed-out history never breaks analysis.
func TouchCounts(ctx context.Context, workDir, subtreePrefix string, moduleFor func(string) (string, bool), runner toolrun.Runner) ModuleTouches {
	result, timedOut := runTouchCounts(ctx, workDir, subtreePrefix, moduleFor, runner, maxCommitsModuleTouches)
	if timedOut {
		return ModuleTouches{Status: ModuleTouchStatusTimeout}
	}
	if len(result.TouchedByModule) > 0 {
		result.Status = ModuleTouchStatusOK
		result.CommitWindow = maxCommitsModuleTouches
		return result
	}
	fallback, timedOut := runTouchCounts(ctx, workDir, subtreePrefix, moduleFor, runner, 0)
	if timedOut {
		return ModuleTouches{Status: ModuleTouchStatusTimeout}
	}
	if len(fallback.TouchedByModule) == 0 {
		return ModuleTouches{Status: ModuleTouchStatusUnavailable}
	}
	fallback.Status = ModuleTouchStatusOK
	fallback.FullHistory = true
	return fallback
}

func runTouchCounts(ctx context.Context, workDir, subtreePrefix string, moduleFor func(string) (string, bool), runner toolrun.Runner, maxCommits int) (ModuleTouches, bool) {
	args := []string{"log", "--format=%H", "--name-only"}
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
		return ModuleTouches{}, true
	}
	if err != nil || out.ExitCode != 0 {
		return ModuleTouches{}, false
	}

	counts := make(map[string]int)
	currentTouched := map[string]struct{}{}
	commits := 0
	sawCommit := false
	flush := func() {
		if len(currentTouched) == 0 {
			return
		}
		for mod := range currentTouched {
			counts[mod]++
		}
		clear(currentTouched)
	}

	for _, raw := range strings.Split(string(out.Stdout), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if isCommitHash(line) {
			if sawCommit {
				flush()
			}
			sawCommit = true
			commits++
			continue
		}
		gitRel := line
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

	return ModuleTouches{TouchedByModule: counts, CommitsScanned: commits}, false
}

func isCommitHash(line string) bool {
	if len(line) != 40 {
		return false
	}
	for _, r := range line {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
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
