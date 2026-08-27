package coverage

import (
	"errors"
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestCoveragePyJSONParser(t *testing.T) {
	data := []byte(`{
  "meta": {"version": "7.5.0"},
  "files": {
    "src/z.py": {"executed_lines": [1], "summary": {"covered_lines": 1, "num_statements": 2}},
    "src/a.py": {"summary": {"covered_lines": 0, "num_statements": 4}}
  },
  "totals": {"covered_lines": 1, "num_statements": 6}
}`)
	facts, err := (coveragePyJSONParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.CoverageFact{
		{File: "src/a.py", CoveredUnits: 0, TotalUnits: 4, Unit: coverageUnitStatements, Format: FormatCoveragePyJSON},
		{File: "src/z.py", CoveredUnits: 1, TotalUnits: 2, Unit: coverageUnitStatements, Format: FormatCoveragePyJSON},
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

func TestCoveragePyJSONParserRejectsTruncatedEmptyAndHeaderOnly(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("{}"), []byte(`{"files":{}}`), []byte(`{"files":{"x.py":{"summary":{"covered_lines":1}}}}`), []byte(`{"files":{"x.py":{"summary":{"covered_lines":1,"num_statements":1}}}`)} {
		facts, err := (coveragePyJSONParser{}).Parse(data)
		if err == nil || facts != nil || !errors.Is(err, ErrMalformedCoveragePyJSON) {
			t.Fatalf("data %q: facts=%+v err=%v", data, facts, err)
		}
	}
}

func FuzzCoveragePyJSONParser(f *testing.F) {
	f.Add([]byte(`{"files":{"x.py":{"summary":{"covered_lines":1,"num_statements":1}}}}`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = (coveragePyJSONParser{}).Parse(data)
	})
}
