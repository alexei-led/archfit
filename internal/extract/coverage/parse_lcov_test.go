package coverage

import (
	"errors"
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestLCOVParserUsesDARecords(t *testing.T) {
	data := []byte(`TN:
SF:src/a.ts
FN:1,main
FNDA:1,main
FNF:1
FNH:1
DA:1,3
DA:2,0
LF:2
LH:1
end_of_record
SF:src/b.ts
DA:4,1
LF:1
LH:1
end_of_record
`)
	facts, err := (lcovParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.CoverageFact{
		{File: "src/a.ts", CoveredUnits: 1, TotalUnits: 2, Unit: coverageUnitLines, Format: FormatLCOV},
		{File: "src/b.ts", CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines, Format: FormatLCOV},
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

func TestLCOVParserReportsSummaryDiscrepancyButKeepsDAFacts(t *testing.T) {
	facts, err := (lcovParser{}).Parse([]byte("SF:file.ts\nDA:1,1\nDA:2,0\nLF:9\nLH:9\nend_of_record\n"))
	if !errors.Is(err, ErrLCOVSummaryDiscrepancy) || len(facts) != 1 {
		t.Fatalf("facts=%+v err=%v, want DA facts plus discrepancy", facts, err)
	}
	if facts[0].CoveredUnits != 1 || facts[0].TotalUnits != 2 {
		t.Fatalf("fact=%+v, want 1/2 from DA records", facts[0])
	}
}

func TestLCOVParserRejectsTruncatedAndMalformed(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("TN:\n"), []byte("SF:file.ts\nDA:1,1\n"), []byte("SF:file.ts\nnot-a-record\nend_of_record\n")} {
		facts, err := (lcovParser{}).Parse(data)
		if err == nil || facts != nil || !errors.Is(err, ErrMalformedLCOV) {
			t.Fatalf("data %q: facts=%+v err=%v", data, facts, err)
		}
	}
}

func FuzzLCOVParser(f *testing.F) {
	f.Add([]byte("SF:file.ts\nDA:1,1\nend_of_record\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = (lcovParser{}).Parse(data)
	})
}
