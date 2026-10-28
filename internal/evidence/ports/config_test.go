package ports

import (
	"encoding/json"
	"strings"
	"testing"
)

// The measurement settings hash encodes each language's ExtractConfig. Fields
// added for .NET must not move the encoding of any other language, or every
// stored baseline would stop comparing. The wants are the encodings before
// those fields existed.
func TestExtractConfigEncodingWithoutDotNetFieldsIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  ExtractConfig
		want string
	}{
		{"zero", ExtractConfig{}, `{"Src":"","Paths":null,"Exclusions":null,"Internal":null,"Mode":"","BuildFlags":null,"GoModuleInclude":null,"GoModuleExclude":null,"TSConfig":"","PyPackage":"","ProjectRoot":"","CargoManifest":"","CargoFeatures":null,"IncludeDevDeps":false,"ModuleGraph":false}`},
		{"every other language's field set", ExtractConfig{
			Src: ".", Paths: []string{"a/**"}, Exclusions: []string{"**/x/**"}, Internal: []string{"a/internal/**"}, Mode: ModeAuto,
			BuildFlags: []string{"-tags", "x"}, GoModuleInclude: []string{"svc/**"}, TSConfig: "tsconfig.json", PyPackage: "pkg",
			ProjectRoot: "r", CargoManifest: "Cargo.toml", CargoFeatures: []string{"f"}, IncludeDevDeps: true, ModuleGraph: true,
		}, `{"Src":".","Paths":["a/**"],"Exclusions":["**/x/**"],"Internal":["a/internal/**"],"Mode":"auto","BuildFlags":["-tags","x"],"GoModuleInclude":["svc/**"],"GoModuleExclude":null,"TSConfig":"tsconfig.json","PyPackage":"pkg","ProjectRoot":"r","CargoManifest":"Cargo.toml","CargoFeatures":["f"],"IncludeDevDeps":true,"ModuleGraph":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("encoding moved:\n got %s\nwant %s", got, tc.want)
			}
		})
	}
	set, err := json.Marshal(ExtractConfig{DotNetProjects: []string{"Server.slnx"}, TypeGraphDisabled: true, TypeGraphTimeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"DotNetProjects":["Server.slnx"]`, `"TypeGraphDisabled":true`, `"TypeGraphTimeout":1`} {
		if !strings.Contains(string(set), field) {
			t.Errorf("encoding %s lacks %s: .NET settings must reach the .NET slice", set, field)
		}
	}
}
