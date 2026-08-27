package coverage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// coveragePyJSONParser parses coverage.py's --cov-report=json output. The
// summary's covered_lines and num_statements are the only per-file values used;
// aggregate totals are deliberately ignored.
type coveragePyJSONParser struct{}

func (coveragePyJSONParser) Format() string  { return FormatCoveragePyJSON }
func (coveragePyJSONParser) Version() string { return "coverage-parser.coverage-py-json.v1" }

func (coveragePyJSONParser) Parse(data []byte) ([]evidence.CoverageFact, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrMalformedCoveragePyJSON)
	}
	var document struct {
		Files map[string]struct {
			Summary *struct {
				CoveredLines  *int `json:"covered_lines"`
				NumStatements *int `json:"num_statements"`
			} `json:"summary"`
		} `json:"files"`
	}
	if err := decodeJSON(data, &document); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedCoveragePyJSON, err)
	}
	if len(document.Files) == 0 {
		return nil, fmt.Errorf("%w: files is empty", ErrMalformedCoveragePyJSON)
	}
	paths := make([]string, 0, len(document.Files))
	for path := range document.Files {
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("%w: empty file path", ErrMalformedCoveragePyJSON)
		}
		entry := document.Files[path]
		if entry.Summary == nil || entry.Summary.CoveredLines == nil || entry.Summary.NumStatements == nil {
			return nil, fmt.Errorf("%w: file %q has incomplete summary", ErrMalformedCoveragePyJSON, path)
		}
		if *entry.Summary.CoveredLines < 0 || *entry.Summary.NumStatements < 0 || *entry.Summary.CoveredLines > *entry.Summary.NumStatements {
			return nil, fmt.Errorf("%w: file %q has invalid summary counts", ErrMalformedCoveragePyJSON, path)
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	facts := make([]evidence.CoverageFact, 0, len(paths))
	for _, path := range paths {
		summary := document.Files[path].Summary
		facts = append(facts, evidence.CoverageFact{File: path, CoveredUnits: *summary.CoveredLines, TotalUnits: *summary.NumStatements, Unit: coverageUnitStatements, Format: FormatCoveragePyJSON})
	}
	return facts, nil
}

// NewCoveragePyJSONParser returns the built-in coverage.py JSON parser.
func NewCoveragePyJSONParser() Parser { return coveragePyJSONParser{} }

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("trailing JSON value")
	} else if err != io.EOF {
		return err
	}
	return nil
}
