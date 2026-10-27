package coverage

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// ErrMalformedCobertura identifies a Cobertura XML artifact that is not
// well-formed or lacks per-class line records.
var ErrMalformedCobertura = errors.New("malformed Cobertura")

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// rootAwareParser is implemented by parsers whose path resolution needs the
// scan root. Cobertura filenames are relative to one of several <source>
// directories, and which one is right depends on what exists under the root.
type rootAwareParser interface {
	Parser
	parseWithRoot([]byte, *Normalizer) ([]evidence.CoverageFact, error)
}

// coberturaParser parses Cobertura XML as written by coverlet and
// dotnet-coverage. A file appears once per class (nested and generated classes
// repeat their parent's file), so line hits are merged per file with max hits.
type coberturaParser struct{}

func (coberturaParser) Format() string  { return FormatCobertura }
func (coberturaParser) Version() string { return "coverage-parser.cobertura.v1" }

// Parse resolves each file against the first <source> only. Without a root it
// cannot test the others; the ingestor uses parseWithRoot instead.
func (coberturaParser) Parse(data []byte) ([]evidence.CoverageFact, error) {
	return parseCobertura(data, nil)
}

func (coberturaParser) parseWithRoot(data []byte, root *Normalizer) ([]evidence.CoverageFact, error) {
	return parseCobertura(data, root)
}

func parseCobertura(data []byte, root *Normalizer) ([]evidence.CoverageFact, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrMalformedCobertura)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var (
		sources  []string
		files    = make(map[string]map[int]int)
		current  map[int]int
		text     strings.Builder
		inSource bool
		sawRoot  bool
	)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrMalformedCobertura, err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "coverage":
				sawRoot = true
			case "source":
				inSource = true
				text.Reset()
			case "class":
				name := strings.TrimSpace(attr(element, "filename"))
				if name == "" {
					return nil, fmt.Errorf("%w: class without filename", ErrMalformedCobertura)
				}
				current = files[name]
				if current == nil {
					current = make(map[int]int)
					files[name] = current
				}
			case "line":
				if current == nil {
					continue
				}
				number, numErr := strconv.Atoi(attr(element, "number"))
				hits, hitsErr := strconv.Atoi(attr(element, "hits"))
				if numErr != nil || hitsErr != nil || number <= 0 || hits < 0 {
					return nil, fmt.Errorf("%w: invalid line record", ErrMalformedCobertura)
				}
				if old, seen := current[number]; !seen || hits > old {
					current[number] = hits
				}
			}
		case xml.CharData:
			if inSource {
				text.Write(element)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "source":
				inSource = false
				if source := strings.TrimSpace(text.String()); source != "" {
					sources = append(sources, source)
				}
			case "class":
				current = nil
			}
		}
	}
	if !sawRoot {
		return nil, fmt.Errorf("%w: missing coverage element", ErrMalformedCobertura)
	}
	return coberturaFacts(files, sources, root)
}

func coberturaFacts(files map[string]map[int]int, sources []string, root *Normalizer) ([]evidence.CoverageFact, error) {
	merged := make(map[string]map[int]int, len(files))
	for name, lines := range files {
		resolved, keep := resolveCoberturaPath(name, sources, root)
		if !keep {
			continue
		}
		mergeLCOVLines(merged, resolved, lines)
	}
	if len(merged) == 0 {
		if len(files) > 0 {
			return nil, nil // every file lies outside the scan root
		}
		return nil, fmt.Errorf("%w: report contains no classes", ErrMalformedCobertura)
	}
	paths := make([]string, 0, len(merged))
	for p := range merged {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	facts := make([]evidence.CoverageFact, 0, len(paths))
	for _, p := range paths {
		covered, total := lcovCounts(merged[p])
		facts = append(facts, evidence.CoverageFact{File: p, CoveredUnits: covered, TotalUnits: total, Unit: coverageUnitLines, Format: FormatCobertura})
	}
	return facts, nil
}

// resolveCoberturaPath joins filename with each <source>; the first join the
// root resolves wins. When none resolves, a join inside the root is kept so the
// ingestor counts it unresolved, and joins outside the root are dropped: a
// report from a solution-wide run also lists NuGet-cache sources.
func resolveCoberturaPath(filename string, sources []string, root *Normalizer) (string, bool) {
	candidates := coberturaCandidates(filename, sources)
	if root == nil {
		return candidates[0], true
	}
	var inside string
	for _, candidate := range candidates {
		if rel, err := root.Normalize(candidate); err == nil {
			return filepath.Join(root.root, filepath.FromSlash(rel)), true
		}
		if inside == "" && root.lexicallyInside(candidate) {
			inside = candidate
		}
	}
	return inside, inside != ""
}

func coberturaCandidates(filename string, sources []string) []string {
	filename = strings.ReplaceAll(filename, `\`, "/")
	if len(sources) == 0 || path.IsAbs(filename) {
		return []string{filename}
	}
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		out = append(out, path.Join(strings.ReplaceAll(source, `\`, "/"), filename))
	}
	return out
}

func attr(element xml.StartElement, name string) string {
	for _, a := range element.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// NewCoberturaParser returns the built-in Cobertura parser.
func NewCoberturaParser() Parser { return coberturaParser{} }
