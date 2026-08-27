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
LF:99
LH:99
end_of_record
SF:src/b.ts
DA:4,1
LF:1
LH:1
end_of_record
`)
	facts, err := (LCOVParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.CoverageFact{
		{File: "src/a.ts", CoveredUnits: 1, TotalUnits: 2, Unit: "lines", Format: FormatLCOV},
		{File: "src/b.ts", CoveredUnits: 1, TotalUnits: 1, Unit: "lines", Format: FormatLCOV},
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

func TestLCOVParserRejectsTruncatedAndMalformed(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("TN:\n"), []byte("SF:file.ts\nDA:1,1\n"), []byte("SF:file.ts\nnot-a-record\nend_of_record\n")} {
		facts, err := (LCOVParser{}).Parse(data)
		if err == nil || facts != nil || !errors.Is(err, ErrMalformedLCOV) {
			t.Fatalf("data %q: facts=%+v err=%v", data, facts, err)
		}
	}
}

func FuzzLCOVParser(f *testing.F) {
	f.Add([]byte("SF:file.ts\nDA:1,1\nend_of_record\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = (LCOVParser{}).Parse(data)
	})
}
