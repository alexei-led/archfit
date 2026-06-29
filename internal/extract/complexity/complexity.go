// Package complexity provides cyclomatic-complexity runners for archfit.
//
// Backends:
//   - auto (default): gocyclo for Go (exact CCN, no Python pin) +
//     ast-grep decision-point proxy for TypeScript/Python/Rust.
//   - lizard: the multi-language lizard tool — exact per-function CCN but
//     requires a Python runtime. Only invoked when backend=lizard.
//   - off: complexity disabled (analyzers.complexity.enabled: false).
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
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/syntax"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// Backend selector constants for analyzers.complexity.backend.
const (
	BackendAuto   = "auto"   // gocyclo(Go) + ast-grep proxy(TS/Py/Rust) — default
	BackendLizard = "lizard" // exact lizard; re-pins Python runtime
)

const (
	toolName      = "lizard"
	statusOK      = "ok"
	statusAbsent  = "absent"
	lizardTimeout = 2 * time.Minute

	// defaultTimeout is the per-analyzer outer watchdog when analyzers.complexity.timeout
	// is not configured. Generous relative to per-subprocess timeouts so it guards
	// only pathological hangs (e.g. a 122k-LOC generated file).
	defaultTimeout = 5 * time.Minute

	// Absent-coverage reasons: why complexity is n/a and the enable step.
	reasonDisabled       = "complexity is opt-in — set `analyzers.complexity.enabled: true` in .archfit.yaml"
	reasonNotInstalled   = "no complexity tool found — install `sg` (ast-grep) for the Go/TS/Py/Rust proxy (`cargo install ast-grep` / `brew install ast-grep`); optionally add `gocyclo` for exact Go CCN (`go install github.com/fzipp/gocyclo/cmd/gocyclo@latest`)"
	reasonLizardMissing  = "lizard not found — install it (`pip install lizard`, or have `uvx` available) to enable complexity"
	reasonRunFailed      = "complexity tool run failed — check the install and rerun"
	reasonSGNotInstalled = "sg (ast-grep) not found — install ast-grep to enable the complexity proxy for TS/Py/Rust"
	reasonTimedOut       = "complexity analysis timed out — increase analyzers.complexity.timeout or reduce the scope"
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
// lizard. excludes is an additive set of glob patterns forwarded to lizard's
// -x flag (config exclusions + scope defaults); nil is safe. fileCfg carries
// the user-supplied file_class globs so gocyclo and the ast-grep proxy honor
// user-defined generated_globs/test_globs (same config that loc uses).
// classIndex is the FileClassIndex built by the loc walk (may be nil); when
// non-nil it provides header-sniff results (e.g. "// Code generated") that
// pure filename classification misses. When enabled is false an absent coverage
// record is returned. A nil result on tool absence is always returned without
// error — callers treat absent coverage as n/a. On watchdog timeout
// StatusTimedOut is returned with nil error so the overall run continues.
func Run(ctx context.Context, runner toolrun.Runner, root string, enabled bool, backend string, timeout time.Duration, excludes []string, fileCfg syntax.FileClassConfig, classIndex map[string]fileclass.FileClass) ([]signal.ComplexityFunc, diagnostic.Coverage, error) {
	if !enabled {
		return nil, absentCov(reasonDisabled), nil
	}

	// Apply per-analyzer watchdog before any subprocess call.
	ctx, cancel := toolrun.WithWatchdog(ctx, timeout, defaultTimeout)
	defer cancel()

	var funcs []signal.ComplexityFunc
	var cov diagnostic.Coverage
	var subErr error
	if backend == BackendLizard {
		funcs, cov, subErr = runLizard(ctx, runner, root, excludes, timeout, fileCfg, classIndex)
	} else {
		funcs, cov, subErr = runAuto(ctx, runner, root, timeout, fileCfg, classIndex)
	}

	// Check both the inner per-subprocess deadline (subErr) and the outer
	// watchdog (ctx.Err()). When an inner timeout fires, runner.Run returns
	// context.DeadlineExceeded as subErr but ctx.Err() is still nil.
	if errors.Is(subErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// Use the backend-specific tool name so the coverage record correctly
		// identifies what timed out. Auto mode may run gocyclo+ast-grep; report
		// neutrally rather than falsely claiming lizard timed out.
		timedOutTool := toolName // "lizard" for the lizard backend
		if backend != BackendLizard {
			timedOutTool = "complexity"
		}
		return nil, diagnostic.Coverage{Tool: timedOutTool, Status: diagnostic.StatusTimedOut, Reason: reasonTimedOut}, nil
	}
	return funcs, cov, nil
}

// runAuto runs gocyclo for Go (exact CCN) and the ast-grep decision-point
// proxy for TS/Py/Rust. When gocyclo is absent the proxy also covers Go.
// timeout is the configured per-analyzer cap (0 → each sub uses its built-in
// constant, which keeps the no-config path byte-identical).
// fileCfg carries the user-supplied file_class globs used to filter output.
// Returns a non-nil error only when an inner per-subprocess deadline fires so
// the caller can surface StatusTimedOut. Other failures degrade to absent coverage.
func runAuto(ctx context.Context, runner toolrun.Runner, root string, timeout time.Duration, fileCfg syntax.FileClassConfig, classIndex map[string]fileclass.FileClass) ([]signal.ComplexityFunc, diagnostic.Coverage, error) {
	gocycloFuncs, gocycloOK, err := runGocyclo(ctx, runner, root, timeout, fileCfg, classIndex)
	if err != nil {
		return nil, absentCov(reasonRunFailed), err
	}

	// proxy covers TS/Py/Rust; also covers Go when gocyclo is absent.
	proxyLangs := []string{langTypeScript, langPython, langRust}
	if !gocycloOK {
		proxyLangs = append([]string{langGo}, proxyLangs...)
	}
	proxyFuncs, proxyCov, err := runProxy(ctx, runner, root, proxyLangs, timeout, fileCfg, classIndex)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, absentCov(reasonRunFailed), err
		}
		return nil, absentCov(reasonRunFailed), nil
	}

	all := gocycloFuncs
	all = append(all, proxyFuncs...)

	switch {
	case gocycloOK:
		cov := diagnostic.Coverage{Tool: gocycloTool, Status: statusOK}
		if len(proxyFuncs) > 0 {
			cov.Tool = gocycloTool + "+ast-grep"
		}
		return all, cov, nil
	case proxyCov.Status == statusOK:
		return all, proxyCov, nil
	default:
		// Neither gocyclo nor the ast-grep proxy is available. Report the
		// coverage under the ast-grep tool name (the primary install target
		// for the auto backend) so the gap hint in pipeline_coverage.go
		// directs the user to install sg rather than lizard.
		return nil, diagnostic.Coverage{Tool: proxyTool, Status: statusAbsent, Reason: reasonNotInstalled}, nil
	}
}

// runLizard invokes lizard (directly or via uvx) and returns per-function CCN.
// Only called when backend=lizard. extraExcludes are config- and scope-derived
// glob patterns forwarded as additional -x flags (additive with lizardExcludes).
// timeout governs the inner subprocess cap when non-zero (so a configured
// analyzers.complexity.timeout can extend beyond the built-in lizardTimeout); zero
// falls back to the lizardTimeout constant. fileCfg carries the user-supplied
// file_class globs so lizard output is filtered the same way runAuto is (C5).
// Returns a non-nil error only when the inner per-subprocess deadline fires
// (context.DeadlineExceeded) so the caller can surface StatusTimedOut.
// Other failures degrade to absent coverage.
func runLizard(ctx context.Context, runner toolrun.Runner, root string, extraExcludes []string, timeout time.Duration, fileCfg syntax.FileClassConfig, classIndex map[string]fileclass.FileClass) ([]signal.ComplexityFunc, diagnostic.Coverage, error) {
	name, pre := lizardCommand(ctx, runner)
	if name == "" {
		return nil, absentCov(reasonLizardMissing), nil
	}

	args := append(append([]string{}, pre...), root, "--csv")
	for _, l := range lizardLanguages {
		args = append(args, "-l", l)
	}
	for _, x := range lizardExcludes {
		args = append(args, "-x", x)
	}
	for _, x := range extraExcludes {
		args = append(args, "-x", x)
	}
	innerTimeout := lizardTimeout
	if timeout > 0 {
		innerTimeout = timeout
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: name, Args: args, WorkDir: root, Timeout: innerTimeout})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, absentCov(reasonRunFailed), err
		}
		return nil, absentCov(reasonRunFailed), nil
	}

	funcs := parseLizardCSV(out.Stdout, root)
	// lizard returns its warning count as the process exit code, so a non-zero
	// exit with parseable output is a successful analysis that merely found hot
	// functions — not a failure. Discarding it would zero the whole complexity
	// signal for any repo with a CCN>threshold function. Only treat the run as
	// failed when it produced no records at all.
	if out.ExitCode != 0 && len(funcs) == 0 {
		return nil, absentCov(reasonRunFailed), nil
	}

	// Post-filter: drop test/generated functions using the same FileClassConfig
	// that runAuto uses. lizardExcludes already removes common test dirs, but
	// user-supplied file_class.generated_globs / test_globs are not forwarded as
	// -x flags, so this pass honors them (C5).
	funcs = filterLizardFuncs(funcs, fileCfg, classIndex)

	cov := diagnostic.Coverage{Tool: toolName, Status: statusOK}
	return funcs, cov, nil
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

// filterLizardFuncs removes non-production functions (test, generated, vendor)
// from lizard output using LookupFileClass. classIndex (from the loc walk) is
// consulted first —
// it includes header-sniff results ("// Code generated … DO NOT EDIT") that
// pure filename patterns miss. When classIndex is nil or a path is absent from
// it, classification falls back to built-in patterns plus fileCfg globs.
func filterLizardFuncs(funcs []signal.ComplexityFunc, fileCfg syntax.FileClassConfig, classIndex map[string]fileclass.FileClass) []signal.ComplexityFunc {
	out := funcs[:0:len(funcs)]
	for _, f := range funcs {
		ext := filepath.Ext(f.File)
		lang := lizardLangFromExt(ext)
		fc := syntax.LookupFileClass(f.File, classIndex, lang, fileCfg)
		if !fileclass.IsProduction(fc) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// lizardLangFromExt maps a file extension to a language tag for LookupFileClass.
func lizardLangFromExt(ext string) string {
	switch ext {
	case ".go":
		return langGo
	case ".ts", ".tsx":
		return langTypeScript
	case ".py":
		return langPython
	case ".rs":
		return langRust
	default:
		return ""
	}
}
