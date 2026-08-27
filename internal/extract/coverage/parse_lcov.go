package coverage

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

// LCOVParser parses LCOV tracefiles. Line-level DA records are authoritative;
// LF/LH summaries are intentionally not used for the fact counts because they
// can be stale or produced by a different tool version.
type LCOVParser struct{}

func (LCOVParser) Format() string  { return FormatLCOV }
func (LCOVParser) Version() string { return "coverage-parser.lcov.v1" }

func (LCOVParser) Parse(data []byte) ([]evidence.CoverageFact, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrMalformedLCOV)
	}
	type record struct {
		path    string
		lines   map[int]int
		lf, lh  *int
		hasData bool
	}
	totals := make(map[string]map[int]int)
	var current *record
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			if line == "end_of_record" {
				key, value, ok = line, "", true
			} else {
				return nil, lcovError(lineNumber, "missing record separator")
			}
		}
		switch key {
		case "TN":
			// Test names may be empty, as allowed by the LCOV format.
		case "SF":
			if current != nil {
				return nil, lcovError(lineNumber, "new source before end_of_record")
			}
			if strings.TrimSpace(value) == "" {
				return nil, lcovError(lineNumber, "empty source path")
			}
			current = &record{path: strings.TrimSpace(value), lines: make(map[int]int)}
		case "DA":
			if current == nil {
				return nil, lcovError(lineNumber, "DA outside source record")
			}
			parts := strings.Split(value, ",")
			if len(parts) < 2 || len(parts) > 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return nil, lcovError(lineNumber, "invalid DA record")
			}
			lineNo, err := positiveInteger(parts[0])
			if err != nil {
				return nil, lcovError(lineNumber, "invalid DA line")
			}
			hits, err := nonNegativeInteger(parts[1])
			if err != nil {
				return nil, lcovError(lineNumber, "invalid DA hit count")
			}
			if len(parts) == 3 && strings.TrimSpace(parts[2]) == "" {
				return nil, lcovError(lineNumber, "empty DA checksum")
			}
			if old, exists := current.lines[lineNo]; !exists || hits > old {
				current.lines[lineNo] = hits
			}
			current.hasData = true
		case "LF", "LH":
			if current == nil {
				return nil, lcovError(lineNumber, key+" outside source record")
			}
			value, err := nonNegativeInteger(value)
			if err != nil {
				return nil, lcovError(lineNumber, "invalid "+key+" summary")
			}
			if key == "LF" {
				current.lf = &value
			} else {
				current.lh = &value
			}
		case "FN", "FNDA", "FNF", "FNH", "BRDA", "BRF", "BRH":
			// Function and branch records do not contribute to line coverage, but
			// are valid LCOV records and must not make real tracefiles fail.
			if current == nil {
				return nil, lcovError(lineNumber, key+" outside source record")
			}
		case "end_of_record":
			if value != "" || current == nil || !current.hasData {
				return nil, lcovError(lineNumber, "incomplete source record")
			}
			mergeLCOVLines(totals, current.path, current.lines)
			current = nil
		default:
			return nil, lcovError(lineNumber, "unknown record "+key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedLCOV, err)
	}
	if current != nil {
		return nil, fmt.Errorf("%w: missing end_of_record", ErrMalformedLCOV)
	}
	if len(totals) == 0 {
		return nil, fmt.Errorf("%w: tracefile contains no source records", ErrMalformedLCOV)
	}
	paths := make([]string, 0, len(totals))
	for path := range totals {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	facts := make([]evidence.CoverageFact, 0, len(paths))
	for _, path := range paths {
		covered, total := 0, 0
		for _, hits := range totals[path] {
			total++
			if hits > 0 {
				covered++
			}
		}
		facts = append(facts, evidence.CoverageFact{File: path, CoveredUnits: covered, TotalUnits: total, Unit: "lines", Format: FormatLCOV})
	}
	return facts, nil
}

func mergeLCOVLines(all map[string]map[int]int, path string, lines map[int]int) {
	merged := all[path]
	if merged == nil {
		merged = make(map[int]int)
		all[path] = merged
	}
	for line, hits := range lines {
		if old, exists := merged[line]; !exists || hits > old {
			merged[line] = hits
		}
	}
}

func lcovError(line int, reason string) error {
	return fmt.Errorf("%w: line %d: %s", ErrMalformedLCOV, line, reason)
}

func positiveInteger(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("not positive")
	}
	return value, nil
}

func nonNegativeInteger(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return 0, fmt.Errorf("not non-negative")
	}
	return value, nil
}

func NewLCOVParser() Parser { return LCOVParser{} }
