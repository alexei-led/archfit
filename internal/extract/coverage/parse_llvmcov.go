package coverage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// llvmCovJSONParser parses llvm-cov export JSON. Per-file line summaries are
// used instead of the aggregate totals so facts can be attributed to modules.
type llvmCovJSONParser struct{}

func (llvmCovJSONParser) Format() string  { return FormatLLVMCovJSON }
func (llvmCovJSONParser) Version() string { return "coverage-parser.llvm-cov-json.v1" }

func (llvmCovJSONParser) Parse(data []byte) ([]evidence.CoverageFact, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrMalformedLLVMCovJSON)
	}
	var document struct {
		Data []struct {
			Files []struct {
				Filename *string `json:"filename"`
				Summary  *struct {
					Lines *struct {
						Covered *int `json:"covered"`
						Count   *int `json:"count"`
					} `json:"lines"`
				} `json:"summary"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := decodeJSON(data, &document); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedLLVMCovJSON, err)
	}
	if len(document.Data) == 0 {
		return nil, fmt.Errorf("%w: data is empty", ErrMalformedLLVMCovJSON)
	}
	type summary struct{ covered, count int }
	files := make(map[string]summary)
	for _, datum := range document.Data {
		if len(datum.Files) == 0 {
			return nil, fmt.Errorf("%w: data entry has no files", ErrMalformedLLVMCovJSON)
		}
		for _, file := range datum.Files {
			if file.Filename == nil || strings.TrimSpace(*file.Filename) == "" || file.Summary == nil || file.Summary.Lines == nil || file.Summary.Lines.Covered == nil || file.Summary.Lines.Count == nil {
				return nil, fmt.Errorf("%w: file has incomplete line summary", ErrMalformedLLVMCovJSON)
			}
			covered, count := *file.Summary.Lines.Covered, *file.Summary.Lines.Count
			if covered < 0 || count < 0 || covered > count {
				return nil, fmt.Errorf("%w: file %q has invalid line counts", ErrMalformedLLVMCovJSON, *file.Filename)
			}
			path := strings.TrimSpace(*file.Filename)
			if old, ok := files[path]; ok {
				// Multiple data entries can describe the same file. Keep the
				// strongest covered count only when the denominator agrees; an
				// inconsistent denominator cannot be represented honestly.
				if old.count != count {
					return nil, fmt.Errorf("%w: file %q has conflicting line counts", ErrMalformedLLVMCovJSON, path)
				}
				if covered > old.covered {
					files[path] = summary{covered: covered, count: count}
				}
			} else {
				files[path] = summary{covered: covered, count: count}
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no files", ErrMalformedLLVMCovJSON)
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	facts := make([]evidence.CoverageFact, 0, len(paths))
	for _, path := range paths {
		value := files[path]
		facts = append(facts, evidence.CoverageFact{File: path, CoveredUnits: value.covered, TotalUnits: value.count, Unit: coverageUnitLines, Format: FormatLLVMCovJSON})
	}
	return facts, nil
}
