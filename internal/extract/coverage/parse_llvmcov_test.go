package coverage

import (
	"errors"
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestLLVMCovJSONParser(t *testing.T) {
	data := []byte(`{
  "type": "llvm.coverage.json.export",
  "version": "2.0.1",
  "data": [{
    "files": [
      {"filename": "src/z.rs", "summary": {"lines": {"count": 8, "covered": 6, "percent": 75.0}}},
      {"filename": "src/a.rs", "summary": {"lines": {"count": 2, "covered": 0, "percent": 0.0}}}
    ]
  }]
}`)
	facts, err := (LLVMCovJSONParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.CoverageFact{
		{File: "src/a.rs", CoveredUnits: 0, TotalUnits: 2, Unit: "lines", Format: FormatLLVMCovJSON},
		{File: "src/z.rs", CoveredUnits: 6, TotalUnits: 8, Unit: "lines", Format: FormatLLVMCovJSON},
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

func TestLLVMCovJSONParserRejectsTruncatedEmptyAndHeaderOnly(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("{}"), []byte(`{"data":[]}`), []byte(`{"data":[{}]}`), []byte(`{"data":[{"files":[{"filename":"x.rs","summary":{"lines":{"covered":1}}}]}]}`)} {
		facts, err := (LLVMCovJSONParser{}).Parse(data)
		if err == nil || facts != nil || !errors.Is(err, ErrMalformedLLVMCovJSON) {
			t.Fatalf("data %q: facts=%+v err=%v", data, facts, err)
		}
	}
}

func FuzzLLVMCovJSONParser(f *testing.F) {
	f.Add([]byte(`{"data":[{"files":[{"filename":"x.rs","summary":{"lines":{"covered":1,"count":1}}}]}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = (LLVMCovJSONParser{}).Parse(data)
	})
}
