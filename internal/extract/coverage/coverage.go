package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	cacheAnalyzer                        = "coverage"
	reasonCoverageSourceUnavailable      = "coverage_source_unavailable"
	reasonCoverageParserUnavailable      = "coverage_parser_unavailable"
	reasonCoverageArtifactTooLarge       = "coverage_artifact_too_large"
	reasonCoverageSidecarTooLarge        = "coverage_sidecar_too_large"
	reasonCoverageAttestedSourceTooLarge = "coverage_attested_source_too_large"
	reasonCoverageFactLimitExceeded      = "coverage_fact_limit_exceeded"
	reasonCoverageLimitsInvalid          = "coverage_limits_invalid"
	reasonCoverageFactsEmpty             = "coverage_facts_empty"
	reasonInvalidCoverageFact            = "invalid_coverage_fact"
	reasonUnresolvedCoveragePaths        = "unresolved_coverage_paths"
)

// Default input limits keep direct Options callers bounded even when they do
// not pass through config projection. Limits are per configured source.
const (
	DefaultMaxBytes int64 = 64 << 20
	DefaultMaxFacts       = 1_000_000
)

var errCoverageInputTooLarge = errors.New("coverage input exceeds configured maximum bytes")

// Source is one configured coverage artifact and its optional attestation.
// Paths are interpreted relative to the resolved scan root and must remain
// contained by it.
type Source struct {
	Path        string
	Format      string
	SidecarPath string
	MaxBytes    int64
	MaxFacts    int
}

func (s Source) withDefaults() Source {
	if s.MaxBytes == 0 {
		s.MaxBytes = DefaultMaxBytes
	}
	if s.MaxFacts == 0 {
		s.MaxFacts = DefaultMaxFacts
	}
	return s
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
		FormatGoCoverProfile: goCoverProfileParser{},
		FormatLCOV:           lcovParser{},
		FormatCoveragePyJSON: coveragePyJSONParser{},
		FormatLLVMCovJSON:    llvmCovJSONParser{},
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
		ingests = append(ingests, i.ingest(normalizer, source.withDefaults()))
	}
	return ingests, coverageHealth(ingests, len(options.Sources))
}

func (i *Ingestor) ingest(normalizer *Normalizer, source Source) evidence.CoverageIngest {
	format := source.Format
	if format == "" {
		format = FormatAuto
	}
	out := evidence.CoverageIngest{Format: format, Freshness: evidence.FreshnessUnverified}
	if source.MaxBytes <= 0 || source.MaxFacts <= 0 {
		out.Reason = reasonCoverageLimitsInvalid
		return out
	}
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
	data, err := readBoundedFile(artifactPath, source.MaxBytes)
	if err != nil {
		if errors.Is(err, errCoverageInputTooLarge) {
			out.Reason = reasonCoverageArtifactTooLarge
		} else {
			out.Reason = reasonCoverageSourceUnavailable
		}
		return out
	}

	format, parser, err = i.selectParser(format, source.Path, data, parser)
	if err != nil {
		out.Reason = joinReasons("coverage_parse_failed", err.Error())
		return out
	}
	out.Format = format
	out.ToolVersion = parser.Version()

	sidecarPath := source.SidecarPath
	if sidecarPath == "" {
		sidecarPath = source.Path + ".sidecar.json"
	}
	attestation := attest(normalizer, sidecarPath, source.MaxBytes)
	out.Freshness = attestation.freshness

	key := coverageCacheKey(normalizer.root, artifactRel, format, parser.Version(), attestation.sourceRef, data)
	if blob, hit := i.store.Get(cacheAnalyzer, key); hit {
		var cached cachedFacts
		if json.Unmarshal(blob, &cached) == nil {
			if len(cached.Facts) > source.MaxFacts {
				out.Reason = joinReasons(reasonCoverageFactLimitExceeded, attestation.reason)
				return out
			}
			out.Facts = cached.Facts
			out.Format = cached.Format
			out.ToolVersion = cached.ToolVersion
			stampFactMetadata(out.Facts, artifactRel, out.Format, out.ToolVersion, attestation.sourceRef)
			out.Reason = attestation.reason
			return out
		}
	}

	facts, err := parser.Parse(data)
	parseWarning := ""
	if err != nil {
		if errors.Is(err, ErrLCOVSummaryDiscrepancy) {
			parseWarning = joinReasons("coverage_parse_warning", err.Error())
		} else {
			out.Reason = joinReasons("coverage_parse_failed", err.Error(), attestation.reason)
			return out
		}
	}
	normalized := normalizeParsedFacts(normalizer, facts, format, source.MaxFacts)
	out.Facts = normalized.facts
	out.UnresolvedPaths = normalized.unresolvedPaths
	if normalized.limitExceeded {
		out.Reason = joinReasons(reasonCoverageFactLimitExceeded, parseWarning, attestation.reason)
		return out
	}
	if format == FormatAuto && len(out.Facts) > 0 && out.Facts[0].Format != "" {
		out.Format = out.Facts[0].Format
	}
	stampFactMetadata(out.Facts, artifactRel, out.Format, parser.Version(), attestation.sourceRef)
	sortCoverageFacts(out.Facts)

	var invalidReason, unresolvedReason, emptyReason string
	if normalized.invalidFact {
		invalidReason = reasonInvalidCoverageFact
	}
	if out.UnresolvedPaths > 0 {
		unresolvedReason = reasonUnresolvedCoveragePaths
	}
	if len(out.Facts) == 0 {
		emptyReason = reasonCoverageFactsEmpty
	}
	factReason := joinReasons(parseWarning, invalidReason, unresolvedReason, emptyReason)
	out.Reason = joinReasons(factReason, attestation.reason)
	if out.Freshness == evidence.FreshnessMatched && out.UnresolvedPaths == 0 && factReason == "" {
		if blob, marshalErr := json.Marshal(cachedFacts{Facts: out.Facts, Format: out.Format, ToolVersion: out.ToolVersion}); marshalErr == nil {
			i.store.Put(cacheAnalyzer, key, blob)
		}
	}
	return out
}

type normalizedFacts struct {
	facts           []evidence.CoverageFact
	unresolvedPaths int
	invalidFact     bool
	limitExceeded   bool
}

func normalizeParsedFacts(normalizer *Normalizer, facts []evidence.CoverageFact, format string, maxFacts int) normalizedFacts {
	var out normalizedFacts
	for _, fact := range facts {
		rel, err := normalizer.Normalize(fact.File)
		if err != nil {
			out.unresolvedPaths++
			continue
		}
		if fact.TotalUnits < 0 || fact.CoveredUnits < 0 || fact.CoveredUnits > fact.TotalUnits || fact.Unit == "" {
			out.invalidFact = true
			continue
		}
		if len(out.facts) == maxFacts {
			// Reject the whole artifact rather than returning a truncated set that
			// downstream attribution could mistake for complete evidence.
			out.facts = nil
			out.limitExceeded = true
			return out
		}
		fact.File = rel
		if fact.Format == "" {
			fact.Format = format
		}
		out.facts = append(out.facts, fact)
	}
	return out
}

func (i *Ingestor) selectParser(format, sourcePath string, data []byte, parser Parser) (string, Parser, error) {
	if format != FormatAuto {
		return format, parser, nil
	}
	detected, err := detectFormat(sourcePath, data)
	if err != nil {
		return "", nil, err
	}
	if detectedParser, found := i.parsers[detected]; found && detectedParser != nil {
		return detected, detectedParser, nil
	}
	return detected, parser, nil
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

func readBoundedFile(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, reasonError(reasonCoverageLimitsInvalid)
	}
	file, err := os.Open(path) //nolint:gosec // paths are either resolved within the scan root or produced by its contained walk
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // read errors are returned; close has no additional result to preserve
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxBytes {
		return nil, errCoverageInputTooLarge
	}

	// The stat avoids needless reads of already-oversized files; limit+1 keeps
	// the read bounded if the file grows or is replaced after that check.
	readLimit := maxBytes + 1
	if maxBytes == math.MaxInt64 {
		readLimit = math.MaxInt64
	}
	data, err := io.ReadAll(io.LimitReader(file, readLimit))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errCoverageInputTooLarge
	}
	return data, nil
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
