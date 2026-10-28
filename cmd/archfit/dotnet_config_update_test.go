package main

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
)

func TestDotNetTypeGraphConfigUpdate(t *testing.T) {
	roslyn := func(mode evidenceports.ToolMode) config.Config {
		return config.Config{Analyzers: config.AnalyzersConfig{Roslyn: config.TimedAnalyzer{Enabled: mode}}}
	}
	t.Run("needs", func(t *testing.T) {
		off := roslyn("")
		off.Languages.DotNet.Enabled = config.ModeOff
		for _, tc := range []struct {
			name      string
			cfg       config.Config
			hasDotNet bool
			want      bool
		}{
			{"implicit default on a .NET tree", roslyn(""), true, true},
			{"auto on a .NET tree", roslyn(config.ModeAuto), true, true},
			{"explicit true is kept", roslyn(config.ModeOn), true, false},
			{"explicit false is kept", roslyn(config.ModeOff), true, false},
			{"no .NET project", roslyn(""), false, false},
			{"language switched off", off, true, false},
		} {
			if got := needsDotNetTypeGraphConfig(tc.cfg, tc.hasDotNet); got != tc.want {
				t.Errorf("%s: needsDotNetTypeGraphConfig = %v, want %v", tc.name, got, tc.want)
			}
		}
	})
	t.Run("ensure", func(t *testing.T) {
		for _, tc := range []struct {
			name, src, want string
		}{
			{
				name: "adds the analyzers section after languages",
				src:  "version: 2\nlanguages:\n  dotnet:\n    enabled: auto\nmodules: {}\n",
				want: "version: 2\nlanguages:\n  dotnet:\n    enabled: auto\nanalyzers:\n  roslyn:\n    enabled: true\n\nmodules: {}\n",
			},
			{
				name: "adds roslyn to an existing section",
				src:  "version: 2\nanalyzers:\n  scip:\n    enabled: true\nmodules: {}\n",
				want: "version: 2\nanalyzers:\n  scip:\n    enabled: true\n  roslyn:\n    enabled: true\nmodules: {}\n",
			},
			{
				name: "turns an auto roslyn on",
				src:  "version: 2\nanalyzers:\n  roslyn:\n    enabled: auto\n    timeout: 20m\nmodules: {}\n",
				want: "version: 2\nanalyzers:\n  roslyn:\n    enabled: true\n    timeout: 20m\nmodules: {}\n",
			},
		} {
			if got := string(ensureDotNetTypeGraphConfig([]byte(tc.src), roslyn(config.ModeAuto))); got != tc.want {
				t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
			}
		}
		src := "version: 2\nanalyzers:\n  roslyn:\n    enabled: false\n"
		if got := string(ensureDotNetTypeGraphConfig([]byte(src), roslyn(config.ModeOff))); !strings.Contains(got, "roslyn:\n    enabled: false") {
			t.Errorf("explicit false was overwritten:\n%s", got)
		}
	})
}
