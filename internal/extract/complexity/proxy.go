package complexity

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/signal"
	"github.com/alexei-led/archfit/internal/syntax"
	"github.com/alexei-led/archfit/internal/toolrun"
)

//go:embed rules/go_ccn.yml
var goCCNRules string

//go:embed rules/typescript_ccn.yml
var tsCCNRules string

//go:embed rules/python_ccn.yml
var pythonCCNRules string

//go:embed rules/rust_ccn.yml
var rustCCNRules string

const proxyTimeout = 2 * time.Minute

// proxyTool is the coverage tool-name string for the ast-grep proxy backend.
// Used here and in complexity.go for the absent-both-tools path.
const proxyTool = "ast-grep"

// Language identifier constants used as keys in proxyLangRules.
const (
	langGo         = "go"
	langTypeScript = "typescript"
	langPython     = "python"
	langRust       = "rust"
)

// proxyLangRules maps language identifier → CCN rule YAML for the ast-grep proxy.
// Go is included for the gocyclo-absent fallback; normal auto runs use
// only typescript/python/rust (Go is covered by gocyclo).
var proxyLangRules = map[string]string{
	langGo:         goCCNRules,
	langTypeScript: tsCCNRules,
	langPython:     pythonCCNRules,
	langRust:       rustCCNRules,
}

// ccnMatch is the JSON shape ast-grep emits for a CCN rule scan match.
// It mirrors the sgSyntaxMatch in the astgrep package; redefined here to keep
// the complexity package self-contained (no extract→extract import).
type ccnMatch struct {
	File   string `json:"file"`
	RuleID string `json:"ruleId"`
	Range  struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
		End struct {
			Line int `json:"line"`
		} `json:"end"`
	} `json:"range"`
	MetaVariables struct {
		Single map[string]struct {
			Text string `json:"text"`
		} `json:"single"`
	} `json:"metaVariables"`
}

// isCCNFuncRule reports whether ruleId is a function-boundary rule.
func isCCNFuncRule(ruleID string) bool {
	return strings.Contains(ruleID, "-ccn-func") || strings.Contains(ruleID, "-ccn-method")
}

// runProxy runs the ast-grep decision-point proxy for the given languages and
// returns per-function complexity records with CCN = 1 + decision points.
// timeout is the configured per-analyzer cap; 0 falls back to proxyTimeout
// (keeps the no-config path byte-identical).
// Returns (nil, absent coverage, nil) when sg is not on PATH.
func runProxy(ctx context.Context, runner toolrun.Runner, root string, langs []string, timeout time.Duration) ([]signal.ComplexityFunc, diagnostic.Coverage, error) {
	if _, ok := runner.Detect(ctx, "sg"); !ok {
		return nil, absentCov(reasonSGNotInstalled), nil
	}

	var all []signal.ComplexityFunc
	anyMatch := false
	for _, lang := range langs {
		rules, ok := proxyLangRules[lang]
		if !ok {
			continue
		}
		funcs, matched, err := runProxyForLang(ctx, runner, root, lang, rules, timeout)
		if err != nil {
			return nil, diagnostic.Coverage{}, err
		}
		all = append(all, funcs...)
		if matched {
			anyMatch = true
		}
	}

	status := statusAbsent
	if anyMatch {
		status = statusOK
	}
	cov := diagnostic.Coverage{Tool: proxyTool, Status: status}
	return all, cov, nil
}

// runProxyForLang runs one language's CCN rules via sg scan and returns the
// per-function complexity records. matched=true when sg produced output.
// timeout is the configured per-analyzer cap; 0 falls back to proxyTimeout.
func runProxyForLang(ctx context.Context, runner toolrun.Runner, root, lang, rules string, timeout time.Duration) ([]signal.ComplexityFunc, bool, error) {
	inner := proxyTimeout
	if timeout > 0 {
		inner = timeout
	}
	var raw []ccnMatch
	out, err := runner.Stream(ctx, toolrun.ToolCmd{
		Name:    "sg",
		Args:    []string{"scan", "--inline-rules", rules, "--json=compact", "."},
		WorkDir: root,
		Timeout: inner,
	}, func(r io.Reader) error {
		var decErr error
		raw, decErr = decodeCCNStream(r, lang)
		return decErr
	})
	if err != nil {
		return nil, false, fmt.Errorf("complexity proxy: sg scan for %q: %w", lang, err)
	}
	// sg non-zero exit with zero matches → rule file rejected (YAML error) or no
	// applicable files. Not fatal — skip this language silently.
	if out.ExitCode != 0 && len(raw) == 0 {
		return nil, false, nil
	}
	if len(raw) == 0 {
		return nil, false, nil
	}
	funcs := assignDecisionPoints(raw)
	// Drop test and generated files from the complexity hotspot signal —
	// production-code metric only. nil index: filename heuristics cover the
	// common cases (*.gen.ts, *.pb.go, *_test.go, *.test.ts, etc.) without
	// needing the LOC-walk index (proxy runs independently of the loc extractor).
	cfg := syntax.FileClassConfig{}
	prod := funcs[:0]
	for _, f := range funcs {
		fc := syntax.LookupFileClass(f.File, nil, lang, cfg)
		if fc != fileclass.Test && fc != fileclass.Generated {
			prod = append(prod, f)
		}
	}
	return prod, true, nil
}

// decodeCCNStream decodes a sg --json=compact array from r element-by-element.
// Empty output (EOF on first token) returns nil, nil — zero matches, no error.
func decodeCCNStream(r io.Reader, lang string) ([]ccnMatch, error) {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("complexity proxy: parse output for %q: %w", lang, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("complexity proxy: parse output for %q: expected JSON array, got %v", lang, tok)
	}
	var out []ccnMatch
	for dec.More() {
		var m ccnMatch
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("complexity proxy: decode match for %q: %w", lang, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// assignDecisionPoints splits matches into function-boundary and decision-point
// groups, then attributes each DP to ALL enclosing functions by line range
// (all-enclosing strategy maximises recall for the CCN>15 hotspot metric at
// the cost of precision — deliberately acceptable for a report-only signal).
// Returns []signal.ComplexityFunc with CCN = 1 + dpCount.
func assignDecisionPoints(raw []ccnMatch) []signal.ComplexityFunc {
	var funcMatches, dpMatches []ccnMatch
	for _, m := range raw {
		if isCCNFuncRule(m.RuleID) {
			funcMatches = append(funcMatches, m)
		} else {
			dpMatches = append(dpMatches, m)
		}
	}
	if len(funcMatches) == 0 {
		return nil
	}

	// funcKey uniquely identifies a function boundary by file + start line.
	type funcKey struct {
		file  string
		start int
	}
	dpCount := make(map[funcKey]int, len(funcMatches))

	for _, dp := range dpMatches {
		dpLine := dp.Range.Start.Line
		for _, fn := range funcMatches {
			if fn.File != dp.File {
				continue
			}
			if dpLine >= fn.Range.Start.Line && dpLine <= fn.Range.End.Line {
				dpCount[funcKey{fn.File, fn.Range.Start.Line}]++
			}
		}
	}

	out := make([]signal.ComplexityFunc, 0, len(funcMatches))
	for _, fn := range funcMatches {
		name := fn.MetaVariables.Single["NAME"].Text
		if name == "" {
			continue
		}
		k := funcKey{fn.File, fn.Range.Start.Line}
		out = append(out, signal.ComplexityFunc{
			File: fn.File,
			Name: name,
			CCN:  1 + dpCount[k],
			NLOC: 0,
			Line: fn.Range.Start.Line + 1, // 0-based → 1-based
		})
	}
	return out
}
