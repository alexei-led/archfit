package coverage

import (
	"errors"
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestCoveragePyJSONParser(t *testing.T) {
	// covered_lines and missing_lines are the line-based counts; num_statements
	// is intentionally ignored (one line can hold multiple statements).
	data := []byte(`{
  "meta": {"version": "7.5.0"},
  "files": {
    "src/z.py": {"executed_lines": [1], "summary": {"covered_lines": 1, "missing_lines": 1, "num_statements": 3}},
    "src/a.py": {"summary": {"covered_lines": 0, "missing_lines": 4, "num_statements": 4}}
  },
  "totals": {"covered_lines": 1, "missing_lines": 5, "num_statements": 7}
}`)
	facts, err := (coveragePyJSONParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.CoverageFact{
		{File: "src/a.py", CoveredUnits: 0, TotalUnits: 4, Unit: coverageUnitLines, Format: FormatCoveragePyJSON},
		{File: "src/z.py", CoveredUnits: 1, TotalUnits: 2, Unit: coverageUnitLines, Format: FormatCoveragePyJSON},
	}
	if len(facts) != len(want) {
		t.Fatalf("facts = %+v, want %+v", facts, want)
	}
	for i := range want {
		if facts[i] != want[i] {
			t.Errorf("fact[%d] = %+v, want %+v", i, facts[i], want[i])
		}
	}
}

// TestCoveragePyJSONParserMultiStatementLine guards against the covered_lines /
// num_statements cross-unit mix: a single covered line with three statements
// must report 100% line coverage (1/1), not 33% statement coverage (1/3).
func TestCoveragePyJSONParserMultiStatementLine(t *testing.T) {
	data := []byte(`{"files":{"x.py":{"summary":{"covered_lines":1,"missing_lines":0,"num_statements":3}}}}`)
	facts, err := (coveragePyJSONParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("want 1 fact, got %d", len(facts))
	}
	got := facts[0]
	if got.CoveredUnits != 1 || got.TotalUnits != 1 || got.Unit != coverageUnitLines {
		t.Errorf("multi-statement-line fact = %+v, want CoveredUnits=1 TotalUnits=1 Unit=%q", got, coverageUnitLines)
	}
}

func TestCoveragePyJSONParserRejectsTruncatedEmptyAndHeaderOnly(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		[]byte("{}"),
		[]byte(`{"files":{}}`),
		// missing missing_lines
		[]byte(`{"files":{"x.py":{"summary":{"covered_lines":1}}}}`),
		// has num_statements but not missing_lines
		[]byte(`{"files":{"x.py":{"summary":{"covered_lines":1,"num_statements":1}}}}`),
		// malformed JSON
		[]byte(`{"files":{"x.py":{"summary":{"covered_lines":1,"missing_lines":0}}}`),
	} {
		facts, err := (coveragePyJSONParser{}).Parse(data)
		if err == nil || facts != nil || !errors.Is(err, ErrMalformedCoveragePyJSON) {
			t.Fatalf("data %q: facts=%+v err=%v", data, facts, err)
		}
	}
}

func FuzzCoveragePyJSONParser(f *testing.F) {
	f.Add([]byte(`{"files":{"x.py":{"summary":{"covered_lines":1,"missing_lines":0}}}}`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = (coveragePyJSONParser{}).Parse(data)
	})
}
