package intramodule_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/intramodule"
	"github.com/alexei-led/archfit/internal/model/signal"
)

func TestComplexity_FlagsOverThreshold(t *testing.T) {
	in := signal.ComplexityInput{Complexity: signal.ComplexitySignals{Funcs: []signal.ComplexityFunc{
		{File: "a/parse.py", Name: "parse_entries", CCN: 72, Line: 401},
		{File: "a/x.py", Name: "small", CCN: 8, Line: 10},
		{File: "b/y.go", Name: "GetSpotSavings", CCN: 25, Line: 183},
	}}}
	res := intramodule.ComplexityMetric{}.Calculate(in)
	if res.Value != 2 { // 72 and 25 are over CCN 15; 8 is not
		t.Errorf("expected 2 complex funcs got %v; display=%q", res.Value, res.Display)
	}
	if !strings.Contains(res.Display, "parse_entries CCN 72") {
		t.Errorf("expected worst function first; display=%q", res.Display)
	}
	if strings.Contains(res.Display, "small") {
		t.Errorf("below-threshold function must not appear; display=%q", res.Display)
	}
}

func TestComplexity_NoDataIsNA(t *testing.T) {
	res := intramodule.ComplexityMetric{}.Calculate(signal.ComplexityInput{})
	if res.Band != bandNAStr {
		t.Errorf("expected n/a without complexity data, got %q", res.Band)
	}
}
