package config

import (
	"strings"
	"testing"
)

func TestReportPolicySelectorsEscapeModuleNames(t *testing.T) {
	t.Parallel()
	data := []byte("version: 3\nowners: [owner]\nmodules:\n  'a/b~c':\n    paths: ['app/**']\n    volatility: high\n")
	in, err := NewReportInput(data)
	if err != nil {
		t.Fatal(err)
	}
	ref := in.Components[0].Volatility.Refs
	if len(ref) != 1 || ref[0].Selector != "/modules/a~1b~0c/volatility" || ref[0].SHA256 == nil || !strings.HasPrefix(*ref[0].SHA256, "sha256:") {
		t.Fatalf("selector: %+v", ref)
	}
}
