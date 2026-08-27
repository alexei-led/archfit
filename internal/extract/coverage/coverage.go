package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// ToolName is the analyzer-coverage row used to route a missing configured
// artifact through the existing required-tool gate.
const ToolName = "supplied-coverage"

// Supported supplied-coverage format names. Concrete parsers implement the
// pure byte-to-fact registry behind this vocabulary.
const (
	FormatAuto           = "auto"
	FormatGoCoverProfile = "go-coverprofile"
	FormatLCOV           = "lcov"
	FormatCoveragePyJSON = "coverage-py-json"
	FormatLLVMCovJSON    = "llvm-cov-json"
)

const (
	cacheAnalyzer                   = "coverage"
	reasonCoverageSourceUnavailable = "coverage_source_unavailable"
	reasonCoverageParserUnavailable = "coverage_parser_unavailable"
	reasonCoverageFactsEmpty        = "coverage_facts_empty"
	reasonInvalidCoverageFact       = "invalid_coverage_fact"
	reasonUnresolvedCoveragePaths   = "unresolved_coverage_paths"
)

// Source is one configured coverage artifact and its optional attestation.
// Paths are interpreted relative to the resolved scan root and must remain
// contained by it.
type Source struct {
	Path        string
	Format      string
	SidecarPath string
}

// Options is the config-projected supplied-coverage input. Enabled is false by
// default; Gate is consumed by acquisition's existing coverage-gap policy.
type Options struct {
	Enabled bool
	Gate    string
	Sources []Source
}

// Parser is the pure byte-to-fact seam concrete format parsers implement. Parse
// must not normalize paths or touch the filesystem; Ingestor owns both.
type Parser interface {
	Format() string
	Version() string
	Parse([]byte) ([]evidence.CoverageFact, error)
}

// Ingestor reads configured artifacts, normalizes parsed paths, resolves the
// sidecar attestation, and uses the shared content-addressed fact store.
type Ingestor struct {
	store   *factcache.Store
	parsers map[string]Parser
}

// New returns an ingestor with all built-in format parsers registered. A
// supplied parser with the same format replaces the built-in parser, which
// keeps the pure parser seam independently testable.
func New(store *factcache.Store, supplied ...Parser) *Ingestor {
	parsers := map[string]Parser{
		FormatAuto:           autoParser{},
		FormatGoCoverProfile: GoCoverProfileParser{},
		FormatLCOV:           LCOVParser{},
		FormatCoveragePyJSON: CoveragePyJSONParser{},
		FormatLLVMCovJSON:    LLVMCovJSONParser{},
	}
	for _, parser := range supplied {
		if parser != nil {
			parsers[parser.Format()] = parser
		}
	}
	return &Ingestor{store: store, parsers: parsers}
}

// SupportedFormats returns the closed config vocabulary in stable order.
func SupportedFormats() []string {
	return []string{FormatAuto, FormatGoCoverProfile, FormatLCOV, FormatCoveragePyJSON, FormatLLVMCovJSON}
}

// IngestAll processes every configured source and returns one normalized ingest
// per source plus a report-only analyzer health row. Disabled coverage returns
// no row, preserving the pre-feature output byte-for-byte.
func (i *Ingestor) IngestAll(scanRoot string, options Options) ([]evidence.CoverageIngest, evidence.Coverage) {
	if !options.Enabled {
		return nil, evidence.Coverage{}
	}
	normalizer, err := NewNormalizer(scanRoot)
	if err != nil {
		return nil, evidence.Coverage{Tool: ToolName, Status: evidence.StatusAbsent, Reason: reasonCoverageSourceUnavailable}
	}
	ingests := make([]evidence.CoverageIngest, 0, len(options.Sources))
	for _, source := range options.Sources {
		ingests = append(ingests, i.ingest(normalizer, source))
	}
	return ingests, coverageHealth(ingests, len(options.Sources))
}

func (i *Ingestor) ingest(normalizer *Normalizer, source Source) evidence.CoverageIngest {
	format := source.Format
	if format == "" {
		format = FormatAuto
	}
	out := evidence.CoverageIngest{Format: format, Freshness: evidence.FreshnessUnverified}
	parser, ok := i.parsers[format]
	if !ok || parser == nil {
		out.Reason = reasonCoverageParserUnavailable
		return out
	}

	artifactPath, err := resolveContainedFile(normalizer.root, source.Path)
	if err != nil {
		out.Reason = reasonCoverageSourceUnavailable
		return out
	}
	artifactRel, err := filepath.Rel(normalizer.root, artifactPath)
	if err != nil {
		out.Reason = reasonCoverageSourceUnavailable
		return out
	}
	artifactRel = filepath.ToSlash(artifactRel)
	data, err := os.ReadFile(artifactPath) //nolint:gosec // path is contained under the scan root
	if err != nil {
		out.Reason = reasonCoverageSourceUnavailable
		return out
	}

	if format == FormatAuto {
		detected, detectErr := detectFormat(source.Path, data)
		if detectErr != nil {
			out.Reason = joinReasons("coverage_parse_failed", detectErr.Error())
			return out
		}
		format = detected
		out.Format = format
		if detectedParser, found := i.parsers[format]; found && detectedParser != nil {
			parser = detectedParser
		}
	}
	out.ToolVersion = parser.Version()

	sidecarPath := source.SidecarPath
	if sidecarPath == "" {
		sidecarPath = source.Path + ".sidecar.json"
	}
	attestation := attest(normalizer, sidecarPath)
	out.Freshness = attestation.freshness

	key := coverageCacheKey(normalizer.root, artifactRel, format, parser.Version(), attestation.sourceRef, data)
	if blob, hit := i.store.Get(cacheAnalyzer, key); hit {
		var cached cachedFacts
		if json.Unmarshal(blob, &cached) == nil {
			out.Facts = cached.Facts
			out.Format = cached.Format
			out.ToolVersion = cached.ToolVersion
			stampFactMetadata(out.Facts, artifactRel, out.Format, out.ToolVersion, attestation.sourceRef)
			out.Reason = attestation.reason
			return out
		}
	}

	facts, err := parser.Parse(data)
	if err != nil {
		out.Reason = joinReasons("coverage_parse_failed", attestation.reason)
		return out
	}
	invalidFact := false
	for _, fact := range facts {
		rel, normalizeErr := normalizer.Normalize(fact.File)
		if normalizeErr != nil {
			out.UnresolvedPaths++
			continue
		}
		if fact.TotalUnits < 0 || fact.CoveredUnits < 0 || fact.CoveredUnits > fact.TotalUnits || fact.Unit == "" {
			invalidFact = true
			continue
		}
		fact.File = rel
		if fact.Format == "" {
			fact.Format = format
		}
		out.Facts = append(out.Facts, fact)
	}
	if format == FormatAuto && len(out.Facts) > 0 && out.Facts[0].Format != "" {
		out.Format = out.Facts[0].Format
	}
	stampFactMetadata(out.Facts, artifactRel, out.Format, parser.Version(), attestation.sourceRef)
	sortCoverageFacts(out.Facts)

	var invalidReason, unresolvedReason, emptyReason string
	if invalidFact {
		invalidReason = reasonInvalidCoverageFact
	}
	if out.UnresolvedPaths > 0 {
		unresolvedReason = reasonUnresolvedCoveragePaths
	}
	if len(out.Facts) == 0 {
		emptyReason = reasonCoverageFactsEmpty
	}
	factReason := joinReasons(invalidReason, unresolvedReason, emptyReason)
	out.Reason = joinReasons(factReason, attestation.reason)
	if out.Freshness == evidence.FreshnessMatched && out.UnresolvedPaths == 0 && factReason == "" {
		if blob, marshalErr := json.Marshal(cachedFacts{Facts: out.Facts, Format: out.Format, ToolVersion: out.ToolVersion}); marshalErr == nil {
			i.store.Put(cacheAnalyzer, key, blob)
		}
	}
	return out
}

type cachedFacts struct {
	Facts       []evidence.CoverageFact `json:"facts"`
	Format      string                  `json:"format"`
	ToolVersion string                  `json:"tool_version"`
}

type coverageKeyConfig struct {
	ScanRoot      string `json:"scan_root"`
	SourcePath    string `json:"source_path"`
	Format        string `json:"format"`
	ParserVersion string `json:"parser_version"`
	SourceRef     string `json:"source_ref"`
}

func coverageCacheKey(scanRoot, sourcePath, format, parserVersion, sourceRef string, data []byte) string {
	configHash, err := factcache.HashJSON(coverageKeyConfig{
		ScanRoot: scanRoot, SourcePath: sourcePath, Format: format, ParserVersion: parserVersion,
		// SourceRef is metadata, not a freshness gate. Including its value keeps
		// cached facts tied to the producer identity they report without caching
		// the sidecar decision or any scanned source hash.
		SourceRef: sourceRef,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return factcache.Key(cacheAnalyzer, parserVersion, configHash, hex.EncodeToString(sum[:]))
}

func stampFactMetadata(facts []evidence.CoverageFact, sourcePath, format, toolVersion, sourceRef string) {
	for idx := range facts {
		facts[idx].SourcePath = sourcePath
		if facts[idx].Format == "" || format != FormatAuto {
			facts[idx].Format = format
		}
		facts[idx].ToolVersion = toolVersion
		facts[idx].SourceRef = sourceRef
	}
}

func sortCoverageFacts(facts []evidence.CoverageFact) {
	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].File != facts[j].File {
			return facts[i].File < facts[j].File
		}
		if facts[i].Unit != facts[j].Unit {
			return facts[i].Unit < facts[j].Unit
		}
		if facts[i].TotalUnits != facts[j].TotalUnits {
			return facts[i].TotalUnits < facts[j].TotalUnits
		}
		return facts[i].CoveredUnits < facts[j].CoveredUnits
	})
}

func coverageHealth(ingests []evidence.CoverageIngest, configured int) evidence.Coverage {
	row := evidence.Coverage{Tool: ToolName}
	if configured == 0 {
		row.Status = evidence.StatusAbsent
		row.Reason = reasonCoverageSourceUnavailable
		return row
	}
	missing := 0
	versions := make(map[string]struct{})
	var reasons []string
	for idx, ingest := range ingests {
		row.FilesSeen += len(ingest.Facts)
		row.FilesApplicable += len(ingest.Facts)
		row.Unresolved += ingest.UnresolvedPaths
		if ingest.ToolVersion != "" {
			versions[ingest.ToolVersion] = struct{}{}
		}
		if ingest.Reason == reasonCoverageSourceUnavailable {
			missing++
		}
		if ingest.Reason != "" {
			reasons = append(reasons, fmt.Sprintf("source[%d]: %s", idx, ingest.Reason))
		}
	}
	versionList := make([]string, 0, len(versions))
	for version := range versions {
		versionList = append(versionList, version)
	}
	sort.Strings(versionList)
	row.Version = strings.Join(versionList, ",")
	row.Reason = strings.Join(reasons, "; ")
	switch {
	case missing == configured:
		row.Status = evidence.StatusAbsent
	case len(reasons) > 0:
		row.Status = evidence.StatusPartial
	default:
		row.Status = evidence.StatusOK
	}
	return row
}

func resolveContainedFile(root, configuredPath string) (string, error) {
	if root == "" || configuredPath == "" || strings.IndexByte(configuredPath, 0) >= 0 {
		return "", reasonError(reasonCoverageSourceUnavailable)
	}
	cleaned := strings.ReplaceAll(strings.TrimSpace(configuredPath), `\`, "/")
	if len(cleaned) >= 2 && cleaned[1] == ':' && !filepath.IsAbs(filepath.FromSlash(cleaned)) {
		return "", reasonError(reasonCoverageSourceUnavailable)
	}
	candidate := filepath.FromSlash(cleaned)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(candidate))
	if err != nil {
		return "", reasonError(reasonCoverageSourceUnavailable)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", reasonError(reasonCoverageSourceUnavailable)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", reasonError(reasonCoverageSourceUnavailable)
	}
	return resolved, nil
}

type reasonError string

func (e reasonError) Error() string { return string(e) }

func joinReasons(reasons ...string) string {
	var nonempty []string
	for _, reason := range reasons {
		if reason != "" {
			nonempty = append(nonempty, reason)
		}
	}
	return strings.Join(nonempty, "; ")
}

type unavailableParser struct{ format string }

func (p unavailableParser) Format() string { return p.format }
func (unavailableParser) Version() string  { return "coverage-parser.v1" }
func (unavailableParser) Parse([]byte) ([]evidence.CoverageFact, error) {
	return nil, errors.New(reasonCoverageParserUnavailable)
}
