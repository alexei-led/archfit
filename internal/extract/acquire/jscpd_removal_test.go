package acquire_test

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/acquire"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestAcquisitionAndDoctorDoNotUseJscpd(t *testing.T) {
	const retiredTool = "jscpd"
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			if name == retiredTool {
				t.Error("acquisition probed retired jscpd tool")
			}
			return toolrun.ToolInfo{}, false
		},
	}
	got := acquire.Collect(context.Background(), t.TempDir(), acquire.Options{}, runner, nil)
	for _, coverage := range got.ExtraCoverage {
		if coverage.Tool == retiredTool {
			t.Errorf("retired jscpd coverage row: %+v", coverage)
		}
	}
	check, report := registry.ToolGroups()
	for _, group := range [][]registry.ToolUse{check, report} {
		for _, tool := range group {
			if tool.Name == retiredTool {
				t.Error("doctor lists retired jscpd tool")
			}
		}
	}
}
