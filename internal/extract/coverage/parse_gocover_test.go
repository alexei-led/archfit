package coverage

import (
	"errors"
	"testing"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

func TestGoCoverProfileParser(t *testing.T) {
	data := []byte(`mode: atomic
example.com/acme/app/a.go:10.1,11.2 2 3
/checkout/app/b.go:20.1,21.2 1 0
example.com/acme/app/a.go:30.1,31.2 1 1
`)
	facts, err := (goCoverProfileParser{}).Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []evidence.CoverageFact{
		{File: "/checkout/app/b.go", CoveredUnits: 0, TotalUnits: 1, Unit: coverageUnitStatements, Format: FormatGoCoverProfile},
		{File: "example.com/acme/app/a.go", CoveredUnits: 3, TotalUnits: 3, Unit: coverageUnitStatements, Format: FormatGoCoverProfile},
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

func TestGoCoverProfileParserModes(t *testing.T) {
	for _, mode := range []string{"set", "count", "atomic"} {
		facts, err := (goCoverProfileParser{}).Parse([]byte("mode: " + mode + "\nfile.go:1.1,1.2 1 9\n"))
		if err != nil || len(facts) != 1 || facts[0].CoveredUnits != 1 {
			t.Fatalf("mode %s: facts=%+v err=%v", mode, facts, err)
		}
	}
}

func TestGoCoverProfileParserRejectsMalformedWithoutFacts(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("mode: set\n"), []byte("mode: set\nnot a block\n"), []byte("mode: nope\nfile.go:1.1,1.2 1 1\n")} {
		facts, err := (goCoverProfileParser{}).Parse(data)
		if err == nil || facts != nil || !errors.Is(err, ErrMalformedGoCoverProfile) {
			t.Fatalf("data %q: facts=%+v err=%v", data, facts, err)
		}
	}
}

func FuzzGoCoverProfileParser(f *testing.F) {
	f.Add([]byte("mode: set\nfile.go:1.1,1.2 1 1\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = (goCoverProfileParser{}).Parse(data)
	})
}
