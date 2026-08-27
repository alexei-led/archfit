package coverage

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

var (
	// ErrMalformedGoCoverProfile identifies a syntactically invalid Go
	// coverprofile. Parsers return no facts with this error.
	ErrMalformedGoCoverProfile = errors.New("malformed Go coverprofile")
	// ErrMalformedLCOV identifies a syntactically invalid LCOV artifact.
	ErrMalformedLCOV = errors.New("malformed LCOV")
	// ErrMalformedCoveragePyJSON identifies a coverage.py JSON artifact that
	// does not contain the required per-file summaries.
	ErrMalformedCoveragePyJSON = errors.New("malformed coverage.py JSON")
	// ErrMalformedLLVMCovJSON identifies an llvm-cov JSON artifact that does
	// not contain the required per-file line summaries.
	ErrMalformedLLVMCovJSON = errors.New("malformed llvm-cov JSON")
	// ErrAmbiguousCoverageFormat means that automatic format detection could
	// not select exactly one supported parser.
	ErrAmbiguousCoverageFormat = errors.New("ambiguous coverage format")
	// ErrAmbiguousFormat is the short compatibility name for the detector error.
	ErrAmbiguousFormat = ErrAmbiguousCoverageFormat
)

var goCoverBlockPattern = regexp.MustCompile(`^(.+):([0-9]+)\.([0-9]+),([0-9]+)\.([0-9]+) ([0-9]+) ([0-9]+)$`)

// GoCoverProfileParser parses the output of go test -coverprofile.
type GoCoverProfileParser struct{}

func (GoCoverProfileParser) Format() string  { return FormatGoCoverProfile }
func (GoCoverProfileParser) Version() string { return "coverage-parser.go-coverprofile.v1" }

func (GoCoverProfileParser) Parse(data []byte) ([]evidence.CoverageFact, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrMalformedGoCoverProfile)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	// A profile path can be long in a generated workspace. Keep parsing bounded
	// by the caller's artifact read while avoiding Scanner's small default cap.
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	modeSeen := false
	stats := make(map[string]*goCoverStats)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !modeSeen {
			if !strings.HasPrefix(line, "mode:") {
				return nil, fmt.Errorf("%w: line %d is missing mode header", ErrMalformedGoCoverProfile, lineNumber)
			}
			mode := strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			switch mode {
			case "set", "count", "atomic":
			default:
				return nil, fmt.Errorf("%w: unsupported mode %q", ErrMalformedGoCoverProfile, mode)
			}
			if mode == "" {
				return nil, fmt.Errorf("%w: empty mode", ErrMalformedGoCoverProfile)
			}
			modeSeen = true
			continue
		}
		match := goCoverBlockPattern.FindStringSubmatch(line)
		if match == nil || match[1] == "" {
			return nil, fmt.Errorf("%w: invalid block at line %d", ErrMalformedGoCoverProfile, lineNumber)
		}
		for index := 2; index <= 5; index++ {
			value, err := strconv.Atoi(match[index])
			if err != nil || value <= 0 {
				return nil, fmt.Errorf("%w: invalid source range at line %d", ErrMalformedGoCoverProfile, lineNumber)
			}
		}
		numStatements, err := strconv.Atoi(match[6])
		if err != nil || numStatements <= 0 {
			return nil, fmt.Errorf("%w: invalid statement count at line %d", ErrMalformedGoCoverProfile, lineNumber)
		}
		count, err := strconv.ParseUint(match[7], 10, 64)
		if err != nil || uint64(int(count)) != count {
			return nil, fmt.Errorf("%w: invalid count at line %d", ErrMalformedGoCoverProfile, lineNumber)
		}
		stat := stats[match[1]]
		if stat == nil {
			stat = &goCoverStats{}
			stats[match[1]] = stat
		}
		stat.total += numStatements
		if count > 0 {
			// Execution count is a hit flag, not a number of covered
			// statements. A count of 2 still covers this block once.
			stat.covered += numStatements
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedGoCoverProfile, err)
	}
	if !modeSeen || len(stats) == 0 {
		return nil, fmt.Errorf("%w: profile contains no coverage blocks", ErrMalformedGoCoverProfile)
	}
	files := make([]string, 0, len(stats))
	for file := range stats {
		files = append(files, file)
	}
	sort.Strings(files)
	facts := make([]evidence.CoverageFact, 0, len(files))
	for _, file := range files {
		stat := stats[file]
		facts = append(facts, evidence.CoverageFact{File: file, CoveredUnits: stat.covered, TotalUnits: stat.total, Unit: "statements", Format: FormatGoCoverProfile})
	}
	return facts, nil
}

type goCoverStats struct{ covered, total int }

func NewGoCoverProfileParser() Parser { return GoCoverProfileParser{} }
