package acquisition

import (
	"maps"
	"testing"

	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
)

func TestDeclaredOutOfScopeHonorsExclusionsAndSwitchedOffLanguages(t *testing.T) {
	const (
		langTypeScript = "typescript"
		langPython     = "python"
		toolScript     = "tools/gen.mjs"
		pyHelper       = "scripts/helper.py"
		tsSource       = "web/app.ts"
		modeOff        = "off"
	)
	f := evidencecontract.Facts{
		FileLOC: map[string]int{"internal/a.go": 10, toolScript: 3, pyHelper: 4, tsSource: 5},
		FileClassIndex: map[string]fileclass.FileClass{
			"internal/a_test.go": fileclass.Test, toolScript: fileclass.Production,
		},
	}
	for _, tc := range []struct {
		name       string
		exclusions []string
		cov        CoverageOptions
		want       map[string]struct{}
	}{
		{name: "nothing declared out of scope"},
		{
			name:       "exclude glob",
			exclusions: []string{"tools/**", "**/testdata/**"},
			want:       map[string]struct{}{toolScript: {}},
		},
		{
			name: "switched-off language",
			cov:  CoverageOptions{Modes: map[string]string{langPython: modeOff}},
			want: map[string]struct{}{pyHelper: {}},
		},
		{
			// An explicit gate on a switched-off language asks to be told the
			// producer did not run, so its sources stay in rule scope.
			name: "switched-off language with an explicit gate",
			cov: CoverageOptions{
				Modes: map[string]string{langTypeScript: modeOff},
				Gates: map[string]string{langTypeScript: gateWarn},
			},
		},
		{
			name:       "both declarations",
			exclusions: []string{"tools/**"},
			cov:        CoverageOptions{Modes: map[string]string{langPython: modeOff, langTypeScript: modeOff}},
			want:       map[string]struct{}{toolScript: {}, pyHelper: {}, tsSource: {}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := declaredOutOfScope(f, tc.exclusions, tc.cov)
			if !maps.Equal(got, tc.want) {
				t.Fatalf("declaredOutOfScope = %v, want %v", got, tc.want)
			}
		})
	}
}
