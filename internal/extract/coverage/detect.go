package coverage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// autoParser keeps automatic detection behind the same pure Parser interface.
// IngestAll supplies the source path to detectFormat so extensions can
// disambiguate JSON artifacts; direct callers fall back to content signatures.
type autoParser struct{}

func (autoParser) Format() string  { return FormatAuto }
func (autoParser) Version() string { return "coverage-parser.auto.v1" }
func (autoParser) Parse(data []byte) ([]evidence.CoverageFact, error) {
	format, err := detectFormat("", data)
	if err != nil {
		return nil, err
	}
	return parseFormat(format, data)
}

// NewAutoParser returns the built-in content-detecting parser.
func NewAutoParser() Parser { return autoParser{} }

func parseFormat(format string, data []byte) ([]evidence.CoverageFact, error) {
	var parser Parser
	switch format {
	case FormatGoCoverProfile:
		parser = goCoverProfileParser{}
	case FormatLCOV:
		parser = lcovParser{}
	case FormatCoveragePyJSON:
		parser = coveragePyJSONParser{}
	case FormatLLVMCovJSON:
		parser = llvmCovJSONParser{}
	default:
		return nil, fmt.Errorf("%w: unsupported format %q", ErrAmbiguousCoverageFormat, format)
	}
	return parser.Parse(data)
}

// detectFormat selects one format from the configured path extension and
// content signature. A conflict or an undecidable JSON document is an explicit
// ambiguity rather than a best-effort guess.
func detectFormat(path string, data []byte) (string, error) {
	extensionCandidates := formatCandidatesForExtension(path)
	magicCandidates := formatCandidatesForMagic(data)
	candidates := extensionCandidates
	if len(candidates) == 0 {
		candidates = magicCandidates
	} else if len(magicCandidates) > 0 {
		candidates = intersectFormats(candidates, magicCandidates)
	}
	if len(candidates) != 1 {
		return "", fmt.Errorf("%w: extension %q and content identify %s", ErrAmbiguousCoverageFormat, filepath.Ext(path), formatList(candidates))
	}
	return candidates[0], nil
}

func formatCandidatesForExtension(path string) []string {
	path = strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	switch strings.ToLower(filepath.Ext(path)) {
	case ".coverprofile", ".cover", ".out":
		return []string{FormatGoCoverProfile}
	case ".info", ".lcov":
		return []string{FormatLCOV}
	case ".json":
		return []string{FormatCoveragePyJSON, FormatLLVMCovJSON}
	default:
		return nil
	}
}

func formatCandidatesForMagic(data []byte) []string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil
	}
	firstLine := trimmed
	if index := bytes.IndexByte(firstLine, '\n'); index >= 0 {
		firstLine = firstLine[:index]
	}
	firstLine = bytes.TrimSpace(bytes.TrimSuffix(firstLine, []byte{'\r'}))
	if bytes.HasPrefix(firstLine, []byte("mode:")) {
		return []string{FormatGoCoverProfile}
	}
	if bytes.HasPrefix(firstLine, []byte("TN:")) || bytes.HasPrefix(firstLine, []byte("SF:")) {
		return []string{FormatLCOV}
	}
	var shape struct {
		Files json.RawMessage `json:"files"`
		Data  json.RawMessage `json:"data"`
	}
	if json.Unmarshal(trimmed, &shape) != nil {
		return nil
	}
	var candidates []string
	if coveragePyShape(shape.Files) {
		candidates = append(candidates, FormatCoveragePyJSON)
	}
	if llvmCovShape(shape.Data) {
		candidates = append(candidates, FormatLLVMCovJSON)
	}
	return candidates
}

func coveragePyShape(raw json.RawMessage) bool {
	var files map[string]struct {
		Summary map[string]json.RawMessage `json:"summary"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &files) != nil || len(files) == 0 {
		return false
	}
	for path, file := range files {
		if strings.TrimSpace(path) == "" || file.Summary == nil || file.Summary["covered_lines"] == nil || file.Summary["missing_lines"] == nil {
			return false
		}
	}
	return true
}

func llvmCovShape(raw json.RawMessage) bool {
	var data []struct {
		Files []struct {
			Filename string `json:"filename"`
			Summary  struct {
				Lines map[string]json.RawMessage `json:"lines"`
			} `json:"summary"`
		} `json:"files"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &data) != nil || len(data) == 0 {
		return false
	}
	found := false
	for _, datum := range data {
		if len(datum.Files) == 0 {
			return false
		}
		for _, file := range datum.Files {
			if strings.TrimSpace(file.Filename) == "" || file.Summary.Lines == nil || file.Summary.Lines["covered"] == nil || file.Summary.Lines["count"] == nil {
				return false
			}
			found = true
		}
	}
	return found
}

func intersectFormats(left, right []string) []string {
	present := make(map[string]struct{}, len(right))
	for _, format := range right {
		present[format] = struct{}{}
	}
	out := make([]string, 0, len(left))
	for _, format := range left {
		if _, ok := present[format]; ok {
			out = append(out, format)
		}
	}
	return out
}

func formatList(formats []string) string {
	if len(formats) == 0 {
		return "no supported format"
	}
	copyFormats := append([]string(nil), formats...)
	sort.Strings(copyFormats)
	return strings.Join(copyFormats, ", ")
}
