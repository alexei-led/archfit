package complexity

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// ---------------------------------------------------------------------------
// assignDecisionPoints
// ---------------------------------------------------------------------------

// buildCCNMatch builds a ccnMatch for use in tests.
func buildCCNMatch(ruleID, file string, startLine, endLine int, name string) ccnMatch {
	m := ccnMatch{
		File:   file,
		RuleID: ruleID,
	}
	m.Range.Start.Line = startLine
	m.Range.End.Line = endLine
	if name != "" {
		m.MetaVariables.Single = map[string]struct {
			Text string `json:"text"`
		}{"NAME": {Text: name}}
	}
	return m
}

func TestAssignDecisionPoints_BasicCCN(t *testing.T) {
	// One function spanning lines 0-9 with 3 decision points.
	raw := []ccnMatch{
		buildCCNMatch("go-ccn-func", "a.go", 0, 9, "Foo"),
		buildCCNMatch("go-ccn-dp", "a.go", 2, 2, ""),
		buildCCNMatch("go-ccn-dp", "a.go", 4, 4, ""),
		buildCCNMatch("go-ccn-dp", "a.go", 7, 7, ""),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].CCN != 4 { // 1 + 3
		t.Errorf("CCN = %d, want 4", funcs[0].CCN)
	}
	if funcs[0].Name != "Foo" {
		t.Errorf("Name = %q, want Foo", funcs[0].Name)
	}
	if funcs[0].Line != 1 { // 0-based start → 1-based
		t.Errorf("Line = %d, want 1", funcs[0].Line)
	}
}

func TestAssignDecisionPoints_NoDecisionPoints(t *testing.T) {
	raw := []ccnMatch{
		buildCCNMatch("go-ccn-func", "a.go", 0, 5, "SimpleFunc"),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].CCN != 1 { // 1 + 0
		t.Errorf("CCN = %d, want 1 (no decision points)", funcs[0].CCN)
	}
}

func TestAssignDecisionPoints_AllEnclosing(t *testing.T) {
	// Outer function spans 0-20; inner function spans 5-15.
	// DP at line 8 (inside both) — all-enclosing: both get it.
	raw := []ccnMatch{
		buildCCNMatch("go-ccn-func", "a.go", 0, 20, "Outer"),
		buildCCNMatch("go-ccn-func", "a.go", 5, 15, "Inner"),
		buildCCNMatch("go-ccn-dp", "a.go", 8, 8, ""),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 2 {
		t.Fatalf("got %d funcs, want 2", len(funcs))
	}
	byCCN := map[string]int{}
	for _, f := range funcs {
		byCCN[f.Name] = f.CCN
	}
	// Both outer and inner get the DP (all-enclosing).
	if byCCN["Outer"] != 2 {
		t.Errorf("Outer CCN = %d, want 2", byCCN["Outer"])
	}
	if byCCN["Inner"] != 2 {
		t.Errorf("Inner CCN = %d, want 2", byCCN["Inner"])
	}
}

func TestAssignDecisionPoints_DifferentFiles(t *testing.T) {
	// DP in file b.go must not be attributed to function in a.go.
	raw := []ccnMatch{
		buildCCNMatch("py-ccn-func", "a.go", 0, 10, "FuncA"),
		buildCCNMatch("py-ccn-dp", "b.go", 3, 3, ""),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].CCN != 1 {
		t.Errorf("CCN = %d, want 1 (DP in different file)", funcs[0].CCN)
	}
}

func TestAssignDecisionPoints_NoFunctions(t *testing.T) {
	raw := []ccnMatch{
		buildCCNMatch("go-ccn-dp", "a.go", 2, 2, ""),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 0 {
		t.Errorf("expected no funcs when no function rules, got %d", len(funcs))
	}
}

func TestAssignDecisionPoints_EmptyName(t *testing.T) {
	// Functions with no $NAME in metaVariables are skipped.
	raw := []ccnMatch{
		buildCCNMatch("go-ccn-func", "a.go", 0, 5, ""), // no name
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 0 {
		t.Errorf("expected 0 funcs for nameless match, got %d", len(funcs))
	}
}

func TestAssignDecisionPoints_MethodRule(t *testing.T) {
	// go-ccn-method is also a function boundary.
	raw := []ccnMatch{
		buildCCNMatch("go-ccn-method", "a.go", 0, 8, "Run"),
		buildCCNMatch("go-ccn-dp", "a.go", 2, 2, ""),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 1 || funcs[0].Name != "Run" || funcs[0].CCN != 2 {
		t.Errorf("unexpected: %+v", funcs)
	}
}

func TestAssignDecisionPoints_TSFuncArrowAndObjProp(t *testing.T) {
	// ts-ccn-func-arrow and ts-ccn-func-objprop contain "-ccn-func" so they are
	// treated as function boundaries, not decision points.
	raw := []ccnMatch{
		buildCCNMatch("ts-ccn-func-arrow", "app.ts", 0, 20, "handler"),
		buildCCNMatch("ts-ccn-func-objprop", "app.ts", 5, 15, "visit"),
		buildCCNMatch("ts-ccn-dp", "app.ts", 2, 2, ""),
		buildCCNMatch("ts-ccn-dp", "app.ts", 8, 8, ""),
	}
	funcs := assignDecisionPoints(raw)
	if len(funcs) != 2 {
		t.Fatalf("got %d funcs, want 2", len(funcs))
	}
	byCCN := map[string]int{}
	for _, f := range funcs {
		byCCN[f.Name] = f.CCN
	}
	// dp at line 2 → handler only; dp at line 8 → both (all-enclosing).
	if byCCN["handler"] != 3 { // 1 + dp@2 + dp@8
		t.Errorf("handler CCN = %d, want 3", byCCN["handler"])
	}
	if byCCN["visit"] != 2 { // 1 + dp@8 only
		t.Errorf("visit CCN = %d, want 2", byCCN["visit"])
	}
}

// ---------------------------------------------------------------------------
// decodeCCNStream
// ---------------------------------------------------------------------------

func TestDecodeCCNStream_Valid(t *testing.T) {
	matches := []ccnMatch{
		buildCCNMatch("go-ccn-func", "f.go", 0, 5, "Foo"),
		buildCCNMatch("go-ccn-dp", "f.go", 2, 2, ""),
	}
	data, _ := json.Marshal(matches)
	got, err := decodeCCNStream(bytes.NewReader(data), langGo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d matches, want 2", len(got))
	}
}

func TestDecodeCCNStream_Empty(t *testing.T) {
	got, err := decodeCCNStream(bytes.NewReader(nil), langGo)
	if err != nil {
		t.Fatalf("unexpected error on empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected nil/empty, got %d", len(got))
	}
}

func TestDecodeCCNStream_EmptyArray(t *testing.T) {
	got, err := decodeCCNStream(bytes.NewReader([]byte("[]")), langGo)
	if err != nil {
		t.Fatalf("unexpected error on empty array: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 matches, got %d", len(got))
	}
}

func TestDecodeCCNStream_NotAnArray(t *testing.T) {
	_, err := decodeCCNStream(bytes.NewReader([]byte(`{"key":"val"}`)), langGo)
	if err == nil {
		t.Error("expected error for non-array JSON")
	}
}

// ---------------------------------------------------------------------------
// runProxy
// ---------------------------------------------------------------------------

// sgProxyRunner returns a mock where sg is detected and Stream returns canned JSON.
func sgProxyRunner(matches []ccnMatch) *toolrun.RunnerMock {
	data, _ := json.Marshal(matches)
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "sg" {
				return toolrun.ToolInfo{Name: "sg", Path: "/usr/bin/sg"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		StreamFunc: func(_ context.Context, _ toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
			if err := consume(bytes.NewReader(data)); err != nil {
				return toolrun.Output{}, err
			}
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

func TestRunProxy_SGAbsent(t *testing.T) {
	funcs, cov, err := runProxy(context.Background(), absentRunner(), t.TempDir(), []string{langTypeScript}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Errorf("expected no funcs when sg absent, got %d", len(funcs))
	}
	if cov.Status != statusAbsent {
		t.Errorf("Status = %q, want %q", cov.Status, statusAbsent)
	}
}

func TestRunProxy_WithMatches(t *testing.T) {
	matches := []ccnMatch{
		buildCCNMatch("ts-ccn-func", "src/app.ts", 0, 10, "handler"),
		buildCCNMatch("ts-ccn-dp", "src/app.ts", 2, 2, ""),
		buildCCNMatch("ts-ccn-dp", "src/app.ts", 5, 5, ""),
	}
	funcs, cov, err := runProxy(context.Background(), sgProxyRunner(matches), t.TempDir(), []string{langTypeScript}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("Status = %q, want %q", cov.Status, statusOK)
	}
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].CCN != 3 { // 1 + 2 DPs
		t.Errorf("CCN = %d, want 3", funcs[0].CCN)
	}
	if funcs[0].Name != "handler" {
		t.Errorf("Name = %q, want handler", funcs[0].Name)
	}
}

func TestRunProxy_EmptyOutput(t *testing.T) {
	// sg present but returns empty array — no funcs, status absent (no match).
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "sg" {
				return toolrun.ToolInfo{Name: "sg", Path: "/usr/bin/sg"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		StreamFunc: func(_ context.Context, _ toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
			_ = consume(bytes.NewReader([]byte("[]")))
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
	funcs, cov, err := runProxy(context.Background(), runner, t.TempDir(), []string{langPython}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Errorf("expected no funcs for empty output, got %d", len(funcs))
	}
	_ = cov // status absent is expected when no matches
}

func TestRunProxy_KnownLanguageCCN(t *testing.T) {
	// Parametric test: confirm each proxy language returns the expected CCN
	// from a known fixture (1 function + N decision points).
	for _, tc := range []struct {
		lang     string
		ruleDP   string
		ruleFunc string
	}{
		{langTypeScript, "ts-ccn-dp", "ts-ccn-func"},
		{langPython, "py-ccn-dp", "py-ccn-func"},
		{langRust, "rs-ccn-dp", "rs-ccn-func"},
	} {
		tc := tc
		t.Run(tc.lang, func(t *testing.T) {
			matches := []ccnMatch{
				buildCCNMatch(tc.ruleFunc, "src/x."+tc.lang, 0, 20, "Target"),
				buildCCNMatch(tc.ruleDP, "src/x."+tc.lang, 3, 3, ""),
				buildCCNMatch(tc.ruleDP, "src/x."+tc.lang, 8, 8, ""),
				buildCCNMatch(tc.ruleDP, "src/x."+tc.lang, 12, 12, ""),
			}
			funcs, _, err := runProxy(context.Background(), sgProxyRunner(matches), t.TempDir(), []string{tc.lang}, 0)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.lang, err)
			}
			if len(funcs) != 1 {
				t.Fatalf("%s: got %d funcs, want 1", tc.lang, len(funcs))
			}
			if funcs[0].CCN != 4 { // 1 + 3
				t.Errorf("%s: CCN = %d, want 4", tc.lang, funcs[0].CCN)
			}
		})
	}
}
