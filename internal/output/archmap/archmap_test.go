package archmap

import (
	"errors"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

func render(t *testing.T, in Input, format string) string {
	t.Helper()
	var b strings.Builder
	if err := Render(in, format, &b); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return b.String()
}

func TestDrawRankPutsOutermostLayersFirst(t *testing.T) {
	layers := []string{"domain", "application", "entrypoint"}
	for _, tc := range []struct {
		layer string
		want  int
	}{
		{"entrypoint", 0}, {"application", 1}, {"domain", 2}, {"adapter", 3}, {"", 4},
	} {
		if got := drawRank(layers, tc.layer); got != tc.want {
			t.Errorf("drawRank(%q) = %d, want %d", tc.layer, got, tc.want)
		}
	}
}

// TestRenderKeepsEveryEndpointAndQuotesLabels: a seam endpoint no config
// declares (a go.work member) is still drawn, and a module name a bare
// Mermaid ID cannot hold is safe inside a quoted label.
func TestRenderKeepsEveryEndpointAndQuotesLabels(t *testing.T) {
	in := Input{
		Modules: []Module{{Name: `odd"name`}},
		Seams:   []report.Seam{{FromModule: `odd"name`, ToModule: "tools/member", Policy: "observed", Strength: "model"}},
	}
	out := render(t, in, FormatMermaid)
	for _, want := range []string{`m0["odd#quot;name"]`, `m1["tools/member"]`, `m0 -->|"observed · model"| m1`} {
		if !strings.Contains(out, want) {
			t.Errorf("mermaid missing %q:\n%s", want, out)
		}
	}
}

func TestFocusCountsWhatItOmits(t *testing.T) {
	in := Input{
		Modules: []Module{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		Seams: []report.Seam{
			{FromModule: "a", ToModule: "b", Policy: "allowed"},
			{FromModule: "b", ToModule: "c", Policy: "violation"},
		},
		Focus: []string{"a"},
	}
	out := render(t, in, FormatMermaid)
	if strings.Contains(out, `"c"`) || !strings.Contains(out, "%% omitted by --focus: 1 seam (1 violation)") {
		t.Errorf("focus must keep a and its neighbour b, and count the omitted violation:\n%s", out)
	}
	in.Focus = []string{"ghost"}
	if err := Render(in, FormatText, &strings.Builder{}); !errors.Is(err, ErrUnknownFocus) {
		t.Errorf("Render with an unknown focus = %v, want ErrUnknownFocus", err)
	}
}

func TestRenderRejectsUnknownFormat(t *testing.T) {
	if err := Render(Input{}, "d2", &strings.Builder{}); err == nil {
		t.Error("Render accepted an unknown format")
	}
}
