package coverage

import (
	"errors"
	"testing"
)

func TestDetectFormatUsesExtensionAndMagic(t *testing.T) {
	cases := []struct {
		name, path, input, want string
	}{
		{name: "Go magic", path: "coverage.out", input: "mode: set\nfile.go:1.1,1.2 1 1\n", want: FormatGoCoverProfile},
		{name: "LCOV magic", path: "report", input: "TN:\nSF:file.ts\nDA:1,1\nend_of_record\n", want: FormatLCOV},
		{name: "coverage.py JSON", path: "coverage.json", input: `{"files":{"x.py":{"summary":{"covered_lines":1,"num_statements":1}}}}`, want: FormatCoveragePyJSON},
		{name: "llvm-cov JSON", path: "coverage.json", input: `{"data":[{"files":[{"filename":"x.rs","summary":{"lines":{"covered":1,"count":1}}}]}]}`, want: FormatLLVMCovJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectFormat(tc.path, []byte(tc.input))
			if err != nil || got != tc.want {
				t.Fatalf("format=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestDetectFormatRejectsAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name, path, input string
	}{
		{name: "unknown JSON", path: "coverage.json", input: `{"hello":"world"}`},
		{name: "unknown artifact", path: "coverage.dat", input: "not a known report\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := (autoParser{}).Parse([]byte(tc.input))
			if facts != nil || !errors.Is(err, ErrAmbiguousCoverageFormat) {
				t.Fatalf("facts=%+v err=%v, want ambiguous error", facts, err)
			}
		})
	}
	if _, err := detectFormat("coverage.info", []byte("mode: set\nfile.go:1.1,1.2 1 1\n")); !errors.Is(err, ErrAmbiguousCoverageFormat) {
		t.Fatalf("format conflict error = %v, want ambiguity", err)
	}
}

func TestIngestAutoUsesSourceExtension(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	writeCoverageFile(t, root, "coverage.json", `{"files":{"`+sourcePath+`":{"summary":{"covered_lines":1,"num_statements":1}}}}`)
	writeSidecar(t, root, "coverage.json.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
	ingests, _ := New(nil).IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: "coverage.json", Format: FormatAuto}}})
	if len(ingests) != 1 || ingests[0].Format != FormatCoveragePyJSON || len(ingests[0].Facts) != 1 {
		t.Fatalf("auto ingest = %+v", ingests)
	}
}
