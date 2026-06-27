// Package complexity provides cyclomatic-complexity runners for archfit.
//
// Backends:
//   - auto (default): gocyclo for Go (exact CCN, no Python pin) +
//     ast-grep decision-point proxy for TypeScript/Python/Rust.
//   - lizard: the multi-language lizard tool — exact per-function CCN but
//     requires a Python runtime. Only invoked when backend=lizard.
//   - off: complexity disabled (tools.complexity.enabled: false).
//
// When the tools are absent, disabled, or a non-fatal failure occurs the
// runner returns an empty result with absent/disabled coverage — never an error.
package complexity

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Backend selector constants for tools.complexity.backend.
const (
	BackendAuto   = "auto"   // gocyclo(Go) + ast-grep proxy(TS/Py/Rust) — default
	BackendLizard = "lizard" // exact lizard; re-pins Python runtime
)

const (
	toolName      = "lizard"
	statusOK      = "ok"
	statusAbsent  = "absent"
	lizardTimeout = 2 * time.Minute

	// defaultTimeout is the per-analyzer outer watchdog when tools.complexity.timeout
	// is not configured. Generous relative to per-subprocess timeouts so it guards
	// only pathological hangs (e.g. a 122k-LOC generated file).
	defaultTimeout = 5 * time.Minute

	// Absent-coverage reasons: why complexity is n/a and the enable step.
	reasonDisabled       = "complexity is opt-in — set `tools.complexity.enabled: true` in .archfit.yaml"
	reasonNotInstalled   = "no complexity tool found — install gocyclo (`go install github.com/fzipp/gocyclo/cmd/gocyclo@latest`) or have `sg` (ast-grep) available for the proxy"
	reasonLizardMissing  = "lizard not found — install it (`pip install lizard`, or have `uvx` available) to enable complexity"
	reasonRunFailed      = "complexity tool run failed — check the install and rerun"
	reasonSGNotInstalled = "sg (ast-grep) not found — install ast-grep to enable the complexity proxy for TS/Py/Rust"
	reasonTimedOut       = "complexity analysis timed out — increase tools.complexity.timeout or reduce the scope"
)

// lizardExcludes keep tests, mocks, vendored, and generated trees out of the
// complexity scan (the same exclusions as the size-skew walk).
var lizardExcludes = []string{
	"*_test*", "*test_*", "*/tests/*", "*/__tests__/*", "*/test/*",
	"*/node_modules/*", "*/mocks/*", "*/vendor/*", "*/dist/*", "*.d.ts",
}

// lizardLanguages pins the languages lizard analyses to the set archfit
// supports (Go, Python, TS/JS, Rust). Passing explicit -l flags makes detection
// deterministic and version-independent: lizard's default "all languages it
// knows" set has drifted across releases (typescript/tsx were not auto-detected
// in older lizard), so without this lizard could silently skip Python or TS
// files and report complexity n/a even when the tool is installed. Names are
// lizard's own language identifiers.
var lizardLanguages = []string{langGo, langPython, "javascript", langTypeScript, "tsx", langRust}

// Run invokes the complexity backend and returns per-function CCN records.
// timeout is the per-analyzer outer watchdog; 0 uses defaultTimeout. backend
// selects the implementation: "" or "auto" → gocyclo+proxy; "lizard" → exact
// lizard. When enabled is false an absent coverage record is returned. A nil
// result on tool absence is always returned without error — callers treat
// absent coverage as n/a. On watchdog timeout StatusTimedOut is returned with
// nil error so the overall run continues.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool, backend string, timeout time.Duration) ([]signal.ComplexityFunc, diagnostic.Coverage, error) {
	if !enabled {
		return nil, absentCov(reasonDisabled), nil
	}

	// Apply per-analyzer watchdog before any subprocess call.
	ctx, cancel := toolrun.WithWatchdog(ctx, timeout, defaultTimeout)
	defer cancel()

	var funcs []signal.ComplexityFunc
	var cov diagnostic.Coverage
	if backend == BackendLizard {
		funcs, cov = runLizard(ctx, runner, root)
	} else {
		funcs, cov = runAuto(ctx, runner, root)
	}

	// Check whether the per-analyzer watchdog fired after sub-runners returned.
	// Sub-runners swallow failures and return absent coverage; ctx.Err() tells us
	// whether that was because of a timeout vs a genuinely missing/failed tool.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusTimedOut, Reason: reasonTimedOut}, nil
	}
	return funcs, cov, nil
}

// runAuto runs gocyclo for Go (exact CCN) and the ast-grep decision-point
// proxy for TS/Py/Rust. When gocyclo is absent the proxy also covers Go.
// Never returns an error — failures degrade to absent coverage.
func runAuto(ctx context.Context, runner toolrun.Runner, root string) ([]signal.ComplexityFunc, diagnostic.Coverage) {
	gocycloFuncs, gocycloOK := runGocyclo(ctx, runner, root)

	// proxy covers TS/Py/Rust; also covers Go when gocyclo is absent.
	proxyLangs := []string{langTypeScript, langPython, langRust}
	if !gocycloOK {
		proxyLangs = append([]string{langGo}, proxyLangs...)
	}
	proxyFuncs, proxyCov, err := runProxy(ctx, runner, root, proxyLangs)
	if err != nil {
		return nil, absentCov(reasonRunFailed)
	}

	all := gocycloFuncs
	all = append(all, proxyFuncs...)

	switch {
	case gocycloOK:
		cov := diagnostic.Coverage{Tool: gocycloTool, Status: statusOK}
		if len(proxyFuncs) > 0 {
			cov.Tool = gocycloTool + "+ast-grep"
		}
		return all, cov
	case proxyCov.Status == statusOK:
		return all, proxyCov
	default:
		return nil, absentCov(reasonNotInstalled)
	}
}

// runLizard invokes lizard (directly or via uvx) and returns per-function CCN.
// Only called when backend=lizard. Never returns an error — failures degrade to absent coverage.
func runLizard(ctx context.Context, runner toolrun.Runner, root string) ([]signal.ComplexityFunc, diagnostic.Coverage) {
	name, pre := lizardCommand(ctx, runner)
	if name == "" {
		return nil, absentCov(reasonLizardMissing)
	}

	args := append(append([]string{}, pre...), root, "--csv")
	for _, l := range lizardLanguages {
		args = append(args, "-l", l)
	}
	for _, x := range lizardExcludes {
		args = append(args, "-x", x)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: name, Args: args, WorkDir: root, Timeout: lizardTimeout})
	if err != nil {
		return nil, absentCov(reasonRunFailed)
	}

	funcs := parseLizardCSV(out.Stdout, root)
	// lizard returns its warning count as the process exit code, so a non-zero
	// exit with parseable output is a successful analysis that merely found hot
	// functions — not a failure. Discarding it would zero the whole complexity
	// signal for any repo with a CCN>threshold function. Only treat the run as
	// failed when it produced no records at all.
	if out.ExitCode != 0 && len(funcs) == 0 {
		return nil, absentCov(reasonRunFailed)
	}

	cov := diagnostic.Coverage{Tool: toolName, Status: statusOK}
	return funcs, cov
}

// absentCov builds an absent coverage record with zero file counts and the
// given reason (why complexity is n/a + how to enable it).
func absentCov(reason string) diagnostic.Coverage {
	return diagnostic.Coverage{Tool: toolName, Status: statusAbsent, Reason: reason}
}

// lizardCommand resolves how to invoke lizard: the binary directly, or via uvx.
func lizardCommand(ctx context.Context, runner toolrun.Runner) (name string, prefix []string) {
	if _, ok := runner.Detect(ctx, toolName); ok {
		return toolName, nil
	}
	if _, ok := runner.Detect(ctx, "uvx"); ok {
		return "uvx", []string{toolName}
	}
	return "", nil
}

// parseLizardCSV parses lizard --csv output into ComplexityFunc records. Columns:
// nloc, ccn, token, param, length, location, file, function, long_name, start, end.
func parseLizardCSV(data []byte, root string) []signal.ComplexityFunc {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	var out []signal.ComplexityFunc
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || len(rec) < 10 {
			continue
		}
		ccn, err := strconv.Atoi(strings.TrimSpace(rec[1]))
		if err != nil {
			continue
		}
		nloc, _ := strconv.Atoi(strings.TrimSpace(rec[0]))
		line, _ := strconv.Atoi(strings.TrimSpace(rec[9]))
		file := rec[6]
		if rel, rerr := filepath.Rel(root, file); rerr == nil {
			file = rel
		}
		out = append(out, signal.ComplexityFunc{
			File: filepath.ToSlash(file), Name: rec[7], CCN: ccn, NLOC: nloc, Line: line,
		})
	}
	return out
}
