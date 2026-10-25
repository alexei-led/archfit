package rust_test

import (
	"context"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/rust"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

func TestModuleGraphUnsupportedTargetIsPartial(t *testing.T) {
	metadata := strings.Replace(libCargoMeta, `"kind": ["lib"]`, `"kind": ["bench"]`, 1)
	runner := moduleGraphRunner(metadata, libCrateDOT)
	extractor := rust.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, ModuleGraph: true})
	_, coverage, err := extractor.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil || coverage.Status != "ok" {
		t.Fatalf("primary inventory: status=%s error=%v", coverage.Status, err)
	}
	module := extractor.LastModuleGraphCoverage()
	if module.Status != "partial" || !strings.Contains(module.Reason, "mylib") {
		t.Fatalf("unsupported target must disclose partial evidence: %+v", module)
	}
	if len(extractor.LastModuleGraphCrates()) != 0 {
		t.Fatal("unsupported target invented a graphed crate")
	}
}
