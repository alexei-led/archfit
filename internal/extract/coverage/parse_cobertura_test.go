package coverage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

const (
	coberturaPath   = "TestResults/coverage.cobertura.xml"
	coberturaSource = "src/Data/Comb.cs"
)

func coberturaDoc(sources []string, classes ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?><coverage version="1.9">`)
	if sources != nil {
		b.WriteString("<sources>")
		for _, s := range sources {
			b.WriteString("<source>" + s + "</source>")
		}
		b.WriteString("</sources>")
	}
	b.WriteString(`<packages><package name="Data"><classes>`)
	for _, c := range classes {
		b.WriteString(c)
	}
	b.WriteString("</classes></package></packages></coverage>")
	return b.String()
}

func coberturaClass(filename string, lines ...string) string {
	var b strings.Builder
	b.WriteString(`<class name="C" filename="` + filename + `"><lines>`)
	for _, n := range lines {
		b.WriteString(`<line number="` + n + `" hits="1" branch="False"/>`)
	}
	b.WriteString("</lines></class>")
	return b.String()
}

// coberturaRoot returns a symlink-resolved scan root holding the given files.
func coberturaRoot(t *testing.T, files ...string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		writeCoverageFile(t, root, f, "class C {}\n")
	}
	return root
}

func ingestCobertura(t *testing.T, root, content string) evidence.CoverageIngest {
	t.Helper()
	writeCoverageFile(t, root, coberturaPath, content)
	ingests, _ := New(nil).IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: coberturaPath, Format: FormatCobertura}}})
	if len(ingests) != 1 {
		t.Fatalf("ingests = %+v", ingests)
	}
	return ingests[0]
}

func TestCoberturaIngestNormalizesPaths(t *testing.T) {
	trimmed := func(root string) string { return strings.TrimPrefix(root, "/") }
	cases := []struct {
		name       string
		files      []string
		doc        func(root string) string
		want       []evidence.CoverageFact
		unresolved int
	}{
		{
			name:  "source is the filesystem root",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{"/"}, coberturaClass(trimmed(root)+"/"+coberturaSource, "1", "2"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 2, TotalUnits: 2}},
		},
		{
			name:  "source is the root with a trailing slash",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{root + "/"}, coberturaClass(coberturaSource, "1"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
		{
			name:  "missing sources entry takes filename relative to the root",
			files: []string{coberturaSource},
			doc: func(string) string {
				return coberturaDoc(nil, coberturaClass(coberturaSource, "3"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
		{
			name:  "absolute filename is used as is",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{"/elsewhere"}, coberturaClass(root+"/"+coberturaSource, "3"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
		{
			name:  "first source that resolves wins",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{"/nowhere", root, "/"}, coberturaClass(coberturaSource, "3"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
		{
			name:  "file outside the root is dropped without being counted",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{"/"},
					coberturaClass(trimmed(root)+"/"+coberturaSource, "1"),
					coberturaClass("home/user/.nuget/packages/x/Program.cs", "1"),
					coberturaClass("../outside.cs", "1"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
		{
			name:  "report produced under another root counts every file unresolved",
			files: []string{coberturaSource},
			doc: func(string) string {
				return coberturaDoc([]string{"/other/checkout/"}, coberturaClass(coberturaSource, "1"), coberturaClass("src/Data/Gone.cs", "1"))
			},
			unresolved: 2,
		},
		{
			name:  "missing file inside the root is counted unresolved",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{root}, coberturaClass(coberturaSource, "1"), coberturaClass("src/Gone.cs", "1"))
			},
			want:       []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
			unresolved: 1,
		},
		{
			name:  "duplicate class entries merge with max hits",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{root},
					`<class name="A" filename="`+coberturaSource+`"><lines><line number="1" hits="0"/><line number="2" hits="4"/></lines></class>`,
					`<class name="A.Nested" filename="`+coberturaSource+`"><lines><line number="1" hits="2"/><line number="2" hits="0"/><line number="3" hits="0"/></lines></class>`)
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 2, TotalUnits: 3}},
		},
		{
			name:  "UTF-8 BOM is stripped",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return "\xEF\xBB\xBF" + coberturaDoc([]string{root}, coberturaClass(coberturaSource, "1"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
		{
			name:  "Windows separators in filename",
			files: []string{coberturaSource},
			doc: func(root string) string {
				return coberturaDoc([]string{root}, coberturaClass(`src\Data\Comb.cs`, "1"))
			},
			want: []evidence.CoverageFact{{File: coberturaSource, CoveredUnits: 1, TotalUnits: 1}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := coberturaRoot(t, tc.files...)
			got := ingestCobertura(t, root, tc.doc(root))
			if got.UnresolvedPaths != tc.unresolved || len(got.Facts) != len(tc.want) {
				t.Fatalf("ingest = %+v, want facts %+v unresolved %d", got, tc.want, tc.unresolved)
			}
			for i, want := range tc.want {
				fact := got.Facts[i]
				if fact.File != want.File || fact.CoveredUnits != want.CoveredUnits || fact.TotalUnits != want.TotalUnits ||
					fact.Unit != coverageUnitLines || fact.Format != FormatCobertura {
					t.Errorf("fact[%d] = %+v, want %+v in lines", i, fact, want)
				}
			}
		})
	}
}

func TestCoberturaIngestRealSample(t *testing.T) {
	sample, err := os.ReadFile(filepath.Join("testdata", "coverlet.cobertura.xml"))
	if err != nil {
		t.Fatal(err)
	}
	const file = "src/Libraries/Data/Utilities/CombGuid.cs"
	root := coberturaRoot(t, file)
	doc := strings.ReplaceAll(string(sample), "REPO", strings.TrimPrefix(root, "/"))
	got := ingestCobertura(t, root, doc)
	if len(got.Facts) != 1 || got.Facts[0].File != file || got.Facts[0].CoveredUnits != 14 || got.Facts[0].TotalUnits != 14 || got.UnresolvedPaths != 0 {
		t.Fatalf("ingest = %+v, want 14/14 lines for %s", got, file)
	}
}

func TestCoberturaParserRejectsMalformed(t *testing.T) {
	cases := []struct{ name, doc string }{
		{"empty", ""},
		{"not XML", goCoverSample},
		{"truncated XML", `<coverage><packages><package><classes><class filename="a.cs"><lines><line number="1" hits="1"/>`},
		{"wrong root", `<report><class filename="a.cs"/></report>`},
		{"no classes", `<coverage><packages/></coverage>`},
		{"class without filename", `<coverage><packages><package><classes><class name="C"/></classes></package></packages></coverage>`},
		{"bad hits", coberturaDoc(nil, `<class filename="a.cs"><lines><line number="1" hits="many"/></lines></class>`)},
		{"negative hits", coberturaDoc(nil, `<class filename="a.cs"><lines><line number="1" hits="-1"/></lines></class>`)},
		{"zero line number", coberturaDoc(nil, `<class filename="a.cs"><lines><line number="0" hits="1"/></lines></class>`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := (coberturaParser{}).Parse([]byte(tc.doc))
			if facts != nil || !errors.Is(err, ErrMalformedCobertura) {
				t.Fatalf("facts=%+v err=%v, want ErrMalformedCobertura", facts, err)
			}
		})
	}
}

func TestCoberturaIngestMalformedReportsParseFailure(t *testing.T) {
	root := coberturaRoot(t)
	got := ingestCobertura(t, root, "<coverage><unclosed>")
	if len(got.Facts) != 0 || !strings.Contains(got.Reason, "coverage_parse_failed") {
		t.Fatalf("ingest = %+v, want coverage_parse_failed", got)
	}
}

func TestCoberturaUnitDiffersFromGoProfile(t *testing.T) {
	root := coberturaRoot(t, coberturaSource)
	writeCoverageFile(t, root, "coverage.out", "mode: set\n"+coberturaSource+":1.1,2.2 1 1\n")
	writeCoverageFile(t, root, coberturaPath, coberturaDoc([]string{root}, coberturaClass(coberturaSource, "1", "2", "3")))
	ingests, _ := New(nil).IngestAll(root, Options{Enabled: true, Sources: []Source{
		{Path: "coverage.out", Format: FormatGoCoverProfile},
		{Path: coberturaPath, Format: FormatCobertura},
	}})
	if len(ingests) != 2 || len(ingests[0].Facts) != 1 || len(ingests[1].Facts) != 1 {
		t.Fatalf("ingests = %+v", ingests)
	}
	goFact, netFact := ingests[0].Facts[0], ingests[1].Facts[0]
	if goFact.File != netFact.File || goFact.Unit != coverageUnitStatements || netFact.Unit != coverageUnitLines {
		t.Fatalf("go=%+v cobertura=%+v, want same file in statements vs lines", goFact, netFact)
	}
}

func TestCoberturaAutoDetection(t *testing.T) {
	doc := coberturaDoc(nil, coberturaClass(coberturaSource, "1"))
	cases := []struct {
		name, path, input string
		want              string
		wantErr           bool
	}{
		{name: "xml extension", path: "coverage.cobertura.xml", input: doc, want: FormatCobertura},
		{name: "BOM and no extension", path: "report", input: "\xEF\xBB\xBF" + doc, want: FormatCobertura},
		{name: "xml extension, Go profile content", path: "report.xml", input: goCoverSample, wantErr: true},
		{name: "cobertura content under lcov extension", path: "report.info", input: doc, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectFormat(tc.path, []byte(tc.input))
			if tc.wantErr {
				if !errors.Is(err, ErrAmbiguousCoverageFormat) {
					t.Fatalf("format=%q err=%v, want ambiguity", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("format=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestCoberturaAutoIngestUsesRoot(t *testing.T) {
	root := coberturaRoot(t, coberturaSource)
	writeCoverageFile(t, root, coberturaPath, coberturaDoc([]string{"/nowhere", root}, coberturaClass(coberturaSource, "1")))
	ingests, _ := New(nil).IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: coberturaPath, Format: FormatAuto}}})
	if len(ingests) != 1 || ingests[0].Format != FormatCobertura || len(ingests[0].Facts) != 1 || ingests[0].Facts[0].File != coberturaSource {
		t.Fatalf("ingests = %+v", ingests)
	}
}

func FuzzCoberturaParser(f *testing.F) {
	f.Add([]byte(coberturaDoc([]string{"/"}, coberturaClass("a.cs", "1"))))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = (coberturaParser{}).Parse(data)
	})
}

func TestCoberturaIngestFromAnotherRootNamesIt(t *testing.T) {
	root := coberturaRoot(t, coberturaSource)
	got := ingestCobertura(t, root, coberturaDoc([]string{"/other/checkout/"}, coberturaClass(coberturaSource, "1")))
	want := "unresolved_coverage_paths: 1 path(s) name no file under the scan root, e.g. /other/checkout/" + coberturaSource
	if !strings.Contains(got.Reason, want) {
		t.Errorf("reason = %q, want it to contain %q", got.Reason, want)
	}
}

func TestCoverageRowCountsAFileCoveredByTwoSourcesOnce(t *testing.T) {
	root := coberturaRoot(t, coberturaSource, "src/Data/Other.cs")
	writeCoverageFile(t, root, "a.cobertura.xml", coberturaDoc([]string{root}, coberturaClass(coberturaSource, "1")))
	writeCoverageFile(t, root, "b.cobertura.xml", coberturaDoc([]string{root}, coberturaClass(coberturaSource, "1"), coberturaClass("src/Data/Other.cs", "1")))
	_, row := New(nil).IngestAll(root, Options{Enabled: true, Sources: []Source{
		{Path: "a.cobertura.xml", Format: FormatCobertura}, {Path: "b.cobertura.xml", Format: FormatCobertura},
	}})
	if row.FilesSeen != 2 || row.FilesApplicable != 2 {
		t.Errorf("row = %+v, want 2 distinct files seen and applicable", row)
	}
}
