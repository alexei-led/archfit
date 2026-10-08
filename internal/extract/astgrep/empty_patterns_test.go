package astgrep_test

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/astgrep"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/pattern"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestFind_EmptyPatternsDisableWithoutProbingTools(t *testing.T) {
	for _, cfg := range []pattern.Config{nil, {}} {
		runner := &toolrun.RunnerMock{}
		matches, cov, err := astgrep.New(runner).Find(context.Background(), testScope, cfg)
		if err != nil || len(matches) != 0 {
			t.Fatalf("empty patterns = %v, %v", matches, err)
		}
		if cov.Tool != toolAstGrep || cov.Status != evidence.StatusDisabled || cov.Version != "" {
			t.Fatalf("empty patterns coverage = %+v", cov)
		}
		if len(runner.DetectCalls()) != 0 || len(runner.RunCalls()) != 0 {
			t.Fatal("empty patterns probed the environment")
		}
	}
}
