package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/result"
)

func TestPrimaryToolForLanguageFollowsRegistryOrder(t *testing.T) {
	t.Parallel()
	all := &result.Result{PrimaryExtractorTools: []string{"go/packages", "dependency-cruiser", "grimp", "cargo", "dotnet-msbuild"}}
	short := &result.Result{PrimaryExtractorTools: all.PrimaryExtractorTools[:4]}
	for _, tc := range []struct {
		language string
		diag     *result.Result
		want     string
	}{
		{"go", all, "go/packages"},
		{languageTypeScript, all, "dependency-cruiser"},
		{languagePython, all, "grimp"},
		{languageRust, all, "cargo"},
		{"dotnet", all, "dotnet-msbuild"},
		{"dotnet", short, ""}, // a registry without the .NET row abstains
		{"java", all, ""},
	} {
		got, ok := primaryToolForLanguage(tc.diag, tc.language)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("primaryToolForLanguage(%q) with %d tools = (%q, %v), want %q", tc.language, len(tc.diag.PrimaryExtractorTools), got, ok, tc.want)
		}
	}
}
