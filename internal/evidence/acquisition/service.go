package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/facts"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// LabelLoader is the approved-label file port. Acquisition reads labels; it
// never decides which of them are fresh — that is relationship analysis.
type LabelLoader interface {
	Load(path string) ([]labels.Label, error)
}

// Service is the concrete evidence-acquisition stage. It resolves the analysis
// boundary, runs every extractor and fact collector, resolves repository
// ownership exactly once, and returns neutral facts plus the run context they
// were acquired under.
//
// One Service value describes one tree and one policy. It keeps no result from
// a previous call: every Acquire returns everything the later stages need, so a
// base-tree sub-run is a second Service, never a second call with hidden state.
type Service struct {
	// ConfigPath and Root are the defaults a request may override.
	ConfigPath string
	Root       string
	// Options and Policy are the config projections this run analyses under.
	Options RunOptions
	Policy  policy.PolicySnapshot

	Runner   toolrun.Runner
	Labels   LabelLoader
	Stderr   io.Writer
	Progress func(stage string)
	// WarnLabel prefixes every stderr warning. The base-tree sub-run sets it to
	// "[base] " so a base-side degradation is never misread as a head regression.
	WarnLabel string
	Refresh   bool
}

var _ application.EvidenceStage = (*Service)(nil)

// Acquire implements application.EvidenceStage.
func (s *Service) Acquire(ctx context.Context, req application.AnalysisRequest) (application.Acquired, error) {
	if s == nil || s.Runner == nil {
		return application.Acquired{}, errors.New("analysis runner is required")
	}
	configPath := s.ConfigPath
	if req.ConfigSource != "" {
		configPath = req.ConfigSource
	}
	bundleDir := filepath.Dir(configPath)
	if req.BundleDir != "" {
		bundleDir = req.BundleDir
	}
	root := s.Root
	if req.Root != "" {
		root = req.Root
	}
	now := req.EvaluatedAt
	if now.IsZero() {
		now = time.Now()
	}

	sc := s.Options.Scope
	sc.WorkDir, sc.Root, sc.Base, sc.Full = scanDir(root, bundleDir), root, req.BaseRef, true
	s.reportPhase("Discovering project")
	resolved, err := scope.Resolve(ctx, sc, gitResolver{workDir: sc.WorkDir, runner: s.Runner})
	if err != nil {
		return application.Acquired{}, err
	}

	runPolicy := s.Policy.Clone()
	store := factcache.NewStore(factsCacheDir(bundleDir))
	store.RefreshMode = s.Refresh
	extractors := registry.Build(s.Runner, s.Options.Extractors, store)

	var warnings []string
	warnLabel := s.WarnLabel
	if req.WarnLabel != "" {
		warnLabel = req.WarnLabel
	}
	note := func(msg string) {
		warnings = append(warnings, msg)
		_, _ = fmt.Fprintln(s.stderr(), "warning: "+warnLabel+msg)
	}
	noteToolError := func(tool string, e error) {
		if e != nil {
			note(tool + ": " + e.Error())
		}
	}
	if abs, absErr := filepath.Abs(bundleDir); absErr == nil {
		if warning := outputInsideRootWarning(resolved.Root, abs); warning != "" {
			note(warning)
		}
	}

	s.reportPhase("Collecting facts")
	collected := acquire.Collect(ctx, resolved.Root, s.Options.Acquisition, s.Runner, store)
	noteToolError(toolLoc, collected.LOCError)
	// Ownership and deploy-unit resolution runs exactly once per run. The
	// resolved snapshot travels on the analysis context; relationship analysis
	// and assessment read it instead of re-walking repository history.
	ownerSource := ownerSourceConfig
	var ownerWarnings []string
	var resolvedOwners map[string]string
	if runPolicy.NeedsOwnerResolution() {
		owners, source := ownership.Resolve(ctx, resolved.Root, resolved.GitRoot, resolved.SubtreePrefix, runPolicy.Topology.ModuleMap, s.Runner)
		resolvedOwners = owners
		ownerSource = string(source)
		if warning := ownerDegradationWarning(source); warning != "" {
			ownerWarnings = append(ownerWarnings, warning)
			note(warning)
		}
	}
	runPolicy = runPolicy.WithResolvedTopology(resolvedOwners, collected.DeployUnitsByModule)
	noteToolError(toolJscpd, collected.CloneError)
	pinned, err := s.loadLabels(bundleDir)
	if err != nil {
		return application.Acquired{}, err
	}

	graphResult, err := Collect(ctx, Input{
		Scope: resolved, Extractors: extractors, Resolver: collected.Resolver,
		ExtraCoverage: collected.ExtraCoverage,
	})
	if err != nil {
		return application.Acquired{}, err
	}
	patternMatches, syntaxFacts, ruleCoverage, err := s.collectRuleEvidence(ctx, resolved, runPolicy, collected)
	if err != nil {
		return application.Acquired{}, err
	}
	coverage := append(append([]evidence.Coverage(nil), graphResult.Coverages...), ruleCoverage...)
	crateRootDirs := map[string]string{}
	// cargo-modules is an opt-in module-graph analyzer, not a file extractor: its
	// row counts crates, not files. It is report evidence only (ToolCoverage, the
	// coverage-gap block, the tool gate, the partial-module-graph confidence cap),
	// so it must stay out of the raw rows rule and metric evaluation read — the
	// `coverage` metric would otherwise divide crate counts by file counts and can
	// exceed the 1.0 ceiling its own contract calls impossible.
	var reportOnlyCoverage []evidence.Coverage
	if ex := registry.RustExtractor(extractors); ex != nil {
		reportOnlyCoverage = append(reportOnlyCoverage, ex.LastModuleGraphCoverage())
		for _, cr := range ex.LastCrateRoots() {
			crateRootDirs[cr.Name] = cr.Dir
		}
	}
	// Unresolved-specifier disclosure. Both analyzers complete but drop edges
	// into the external bucket, so the gap must not be stderr-silent.
	if warning := tsUnresolvedWarning(coverage); warning != "" {
		note(warning)
	}
	if warning := pyUnresolvedWarning(coverage); warning != "" {
		note(warning)
	}
	// Rule and metric evaluation reads the RAW coverage rows; the marked copy is
	// report evidence only, so a config opt-out can never move a measured metric.
	marked := markDisabledPrimaries(append(append([]evidence.Coverage(nil), coverage...), reportOnlyCoverage...), s.Options.Coverage, resolved.Root)

	return application.Acquired{
		Facts: evidencecontract.Facts{
			Graph: graphResult.Graph, Coverage: coverage,
			Symbols: graphResult.SCIPSymbols, PatternMatches: patternMatches,
			SyntaxFacts: syntaxFacts, FileLOC: collected.FileLOC,
			FileClassIndex: collected.FileClassIndex,
			FileFacts:      facts.Build(graphResult.SCIPSymbols, collected.FileLOC),
			Clones:         collected.DuplicationClusters,
			DynamicImports: collected.DynamicImports, RuntimeAsyncSites: collected.RuntimeAsyncSites,
			RuntimeConfidence: collected.RuntimeConfidence, DeprecatedDeps: collected.DeprecatedDeps,
			SemanticStrengthOverlay: graphResult.SemanticStrengthOverlay,
		},
		Context: application.AnalysisContext{
			Scope: resolved, BaseRef: req.BaseRef, Full: true,
			Now: now, ConfigHash: configHash(configPath), PrimaryExtractorTools: registry.PrimaryTools(),
			ConfigSource: configPath, BundleDir: bundleDir, ScanRoot: root,
			PinnedLabels: pinned, Policy: runPolicy,
			OwnerSource: ownerSource, OwnerWarnings: ownerWarnings,
			ConfigWarnings:          configWarnings(s.Options.LintWarnings, warnings, runPolicy.Topology.Modules, pinned, configPath),
			MarkedCoverage:          marked,
			CoverageGaps:            buildCoverageGaps(marked, s.Options.Coverage, resolved.Root),
			CrateRootDirs:           crateRootDirs,
			VolatilityCorroboration: buildVolatilityCorroboration(ctx, resolved.GitRoot, resolved.SubtreePrefix, runPolicy, s.Runner),
		},
	}, nil
}

// configWarnings assembles the advisory config-warning block: config lint,
// then every degradation this run disclosed on stderr, then the judgment
// decisions the scorer needs a human to make.
func configWarnings(lint, runWarnings []string, modules map[string]policy.ModuleDef, pinned []labels.Label, configPath string) []string {
	out := buildConfigWarnings(lint, runWarnings)
	return append(out, buildJudgmentDecisionTasks(modules, pinned, configPath)...)
}

// ownerSourceConfig is the owner_source reported when policy already declares
// every owner and no repository walk is needed.
const ownerSourceConfig = "config"

// collectRuleEvidence acquires the pattern and syntax evidence rule evaluation
// reads. Syntax facts are stamped with their module here because the module map
// is a policy input the later stages must not re-derive.
func (s *Service) collectRuleEvidence(ctx context.Context, sc scope.Scope, p policy.PolicySnapshot, collected acquire.Result) ([]pattern.Match, []evidence.SyntaxFact, []evidence.Coverage, error) {
	patternMatches, patternCoverage, err := collected.Patterns.Find(ctx, sc, s.Options.Patterns)
	if err != nil {
		return nil, nil, nil, err
	}
	coverage := make([]evidence.Coverage, 0, 2)
	coverage = append(coverage, patternCoverage)
	if !s.Options.Syntax.Enabled {
		return patternMatches, nil, coverage, nil
	}
	if collected.Syntax == nil {
		return nil, nil, nil, errors.New("acquisition: SyntaxCfg.Enabled=true but no Syntax provider")
	}
	syntaxFacts, syntaxCoverage, err := collected.Syntax.Syntax(ctx, sc, s.Options.Syntax.Languages)
	if err != nil {
		return nil, nil, nil, err
	}
	for i := range syntaxFacts {
		syntaxFacts[i].Module, _ = p.Relationship.Topology.ModuleMap.ModuleForFile(syntaxFacts[i].File)
	}
	return patternMatches, syntaxFacts, append(coverage, syntaxCoverage), nil
}

// loadLabels reads the pinned labels from the RUN's bundle directory, not from
// the service's own config path. `config compare` measures two configs against
// one bundle: both sides must classify with the same approved labels, or the
// comparison would attribute a label difference to the candidate config.
func (s *Service) loadLabels(bundleDir string) ([]labels.Label, error) {
	if s == nil || s.Labels == nil {
		return nil, nil
	}
	return s.Labels.Load(filepath.Join(bundleDir, labelsFileName))
}

// labelsFileName is the pinned-label file every bundle carries.
const labelsFileName = ".archfit-labels.yaml"

func (s *Service) stderr() io.Writer {
	if s != nil && s.Stderr != nil {
		return s.Stderr
	}
	return os.Stderr
}

func (s *Service) reportPhase(stage string) {
	if s != nil && s.Progress != nil {
		s.Progress(stage)
	}
}

// scanDir returns the scope/git resolution anchor for a run.
func scanDir(root, bundleDir string) string {
	if root != "" {
		return root
	}
	return bundleDir
}

// factsCacheDir returns the extractor fact-cache directory under baseDir.
func factsCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "facts")
}

// gitResolver adapts internal/history/git to scope. The concrete git dependency
// lives with the acquisition adapter; scope itself stays free of process and
// tool dependencies.
type gitResolver struct {
	workDir string
	runner  toolrun.Runner
}

func (g gitResolver) RepoRoot(ctx context.Context) (string, error) {
	return git.RepoRoot(ctx, g.workDir, g.runner)
}
func (g gitResolver) HeadRef(ctx context.Context) (string, error) {
	return git.HeadRef(ctx, g.workDir, g.runner)
}
func (g gitResolver) Changed(ctx context.Context, base, head string) ([]string, error) {
	cs, err := git.Changed(ctx, g.workDir, base, head, "", g.runner)
	if err != nil {
		return nil, err
	}
	return cs.Files, nil
}

// configHash returns the sha256 hex digest of the raw config file bytes at
// path, or "" when the file cannot be read. It is the run's config identity:
// two runs with the same hash analysed the same policy text, so the git-origin
// delta can tell a code change from a policy change.
func configHash(path string) string {
	b, err := os.ReadFile(path) //#nosec G304 -- path comes from the --config CLI flag, not arbitrary user input
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
