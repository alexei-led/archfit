package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	suppliedcoverage "github.com/alexei-led/archfit/internal/extract/coverage"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// measurementProfile builds the measurement identity of this run (profile v2).
//
// The settings hash covers the global settings plus ONE slice per language that
// contributes a producer. A language whose primary row is not applicable —
// absent, no coverage gap, and no source file of that language in the
// inventory — contributes no row and no slice, so a release that registers a new
// language leaves the profile of every tree without that language unchanged.
func (s *Service) measurementProfile(ctx context.Context, sc scope.Scope, rows []evidence.Coverage, gaps []evidence.CoverageGap, fileIndex map[string]fileclass.FileClass, history *evidence.VolatilityCorroboration) *evidence.MeasurementProfile {
	p := &evidence.MeasurementProfile{Version: evidence.MeasurementProfileVersion, Producers: []evidence.MeasurementProducer{}, Unknowns: []string{}}
	global := map[string]any{
		"exclusions": s.Options.Exclusions,
		"file_class": s.Options.Acquisition.FileClass, "syntax": s.Options.Syntax,
		"patterns":          sortedPatterns(s.Options.Patterns),
		"supplied_coverage": suppliedCoverageSettings(s.Options.SuppliedCoverage),
	}
	byLanguage := map[string]map[string]any{}
	slice := func(lang string) map[string]any {
		if byLanguage[lang] == nil {
			byLanguage[lang] = map[string]any{"extractor": s.Options.Extractors[lang]}
		}
		return byLanguage[lang]
	}
	inventory := inventoryExtensions(fileIndex)
	gapped := make(map[string]bool, len(gaps))
	for _, gap := range gaps {
		gapped[gap.Tool] = true
	}
	for _, row := range rows {
		if row.Tool == "" {
			continue
		}
		lang := toolLanguage[row.Tool]
		if lang != "" && profileNotApplicable(row, lang, gapped, inventory) {
			continue
		}
		semantics, _ := evidence.MeasurementContract(row.Tool)
		version := normalizeToolVersion(row.Version)
		if row.Status == evidence.StatusAbsent || row.Status == evidence.StatusDisabled {
			version = ""
		}
		p.Producers = append(p.Producers, evidence.MeasurementProducer{Tool: row.Tool, SemanticsVersion: semantics, ToolVersion: version, Status: row.Status, PartialBasis: measurementPartialBasis(row)})
		if lang == "" {
			continue
		}
		sl := slice(lang)
		ran := row.Status != evidence.StatusAbsent && row.Status != evidence.StatusDisabled
		if row.Tool == registry.ToolGoPackages && ran {
			env, ok := s.measurementGoEnv(ctx, sc.Root)
			if !ok {
				p.Unknowns = append(p.Unknowns, "go/packages build environment is unknown")
			} else {
				sl["go_environment"] = env
			}
		}
		if row.Tool == registry.ToolDepCruiser && ran {
			if row.MeasurementSettingsHash != "" {
				sl["typescript_config"] = row.MeasurementSettingsHash
				continue
			}
			hash, err := ts.MeasurementConfigHash(sc, s.Options.Extractors["typescript"])
			if err != nil {
				p.Unknowns = append(p.Unknowns, "dependency-cruiser measurement configuration is unknown")
			} else {
				sl["typescript_config"] = hash
			}
		}
	}
	historySemantics, _ := evidence.MeasurementContract("git-history")
	historyProducer := evidence.MeasurementProducer{Tool: "git-history", SemanticsVersion: historySemantics, Status: evidence.StatusAbsent}
	if history != nil {
		historyProducer.Status = history.Status
		historyProducer.ToolVersion = normalizeToolVersion(s.measurementToolVersion(ctx, sc.GitRoot, "git"))
	}
	p.Producers = append(p.Producers, historyProducer)
	sort.Slice(p.Producers, func(i, j int) bool { return p.Producers[i].Tool < p.Producers[j].Tool })
	data, err := json.Marshal(map[string]any{"global": global, "languages": byLanguage})
	if err != nil {
		p.Unknowns = append(p.Unknowns, "measurement settings could not be encoded")
	} else {
		hash := sha256.Sum256(data)
		p.SettingsHash = hex.EncodeToString(hash[:])
	}
	return p
}

// toolLanguage maps a language-bound producer to its language ID. Cross-language
// producers (SCIP, ast-grep, jscpd, git history) are not in it: they always
// belong to the profile.
var toolLanguage = buildToolLanguage()

func buildToolLanguage() map[string]string {
	out := map[string]string{registry.ToolCargoModules: "rust"}
	for _, lang := range registry.All() {
		out[lang.PrimaryTool] = lang.ID
	}
	return out
}

// profileNotApplicable reports that a language-bound row says nothing about this
// tree: the extractor found no project, no coverage gap asks for the tool, and
// no file of that language sits in the source inventory. Every other absent row
// stays in the profile, so a missing analyzer over present source fails closed.
func profileNotApplicable(row evidence.Coverage, lang string, gapped, inventory map[string]bool) bool {
	if row.Status != evidence.StatusAbsent || gapped[row.Tool] {
		return false
	}
	for _, ext := range graph.BuiltinConventions[lang].FileExtensions {
		if inventory[ext] {
			return false
		}
	}
	return true
}

func inventoryExtensions(index map[string]fileclass.FileClass) map[string]bool {
	out := map[string]bool{}
	for file := range index {
		out[strings.ToLower(path.Ext(file))] = true
	}
	return out
}

// sortedPatterns makes rule order irrelevant: reordering two rules moves no fact.
func sortedPatterns(patterns pattern.Config) []string {
	out := make([]string, 0, len(patterns))
	for _, def := range patterns {
		data, err := json.Marshal(def)
		if err != nil {
			data = []byte(fmt.Sprint(def))
		}
		out = append(out, string(data))
	}
	sort.Strings(out)
	return out
}

// maxToolVersionLen is the consumer's bound for a tool version string.
const maxToolVersionLen = 128

// normalizeToolVersion reduces a tool's --version output to one printable line
// of at most maxToolVersionLen runes. Text beyond the bound is replaced by a
// digest of the whole string, so two different long versions stay different.
func normalizeToolVersion(raw string) string {
	line := ""
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	line = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, line)
	line = strings.Join(strings.Fields(line), " ")
	if utf8.RuneCountInString(line) <= maxToolVersionLen {
		return line
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	const suffix = 13 // "~" plus 12 hex
	return string([]rune(line)[:maxToolVersionLen-suffix]) + "~" + hex.EncodeToString(sum[:])[:12]
}

func measurementPartialBasis(row evidence.Coverage) evidence.MeasurementPartialBasis {
	if row.UnresolvedInputsMissing > 0 {
		return ""
	}
	switch {
	case evidence.PartialFromUnresolvedSpecifiers(row):
		return evidence.PartialUnresolvedSpecifiers
	case evidence.PartialFromDegradedPrecision(row):
		return evidence.PartialDegradedPrecision
	default:
		return ""
	}
}

func (s *Service) measurementToolVersion(ctx context.Context, root, tool string) string {
	out, err := s.Runner.Run(ctx, toolrun.ToolCmd{Name: tool, Args: []string{"--version"}, WorkDir: root, Timeout: 10 * time.Second})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

func (s *Service) measurementGoEnv(ctx context.Context, root string) (map[string]string, bool) {
	keys := []string{"GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS", "GOEXPERIMENT", "GO111MODULE", "GOTOOLCHAIN",
		"GO386", "GOAMD64", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM"}
	out, err := s.Runner.Run(ctx, toolrun.ToolCmd{Name: "go", Args: goEnvJSONArgs(keys), WorkDir: root, Timeout: 10 * time.Second})
	if err != nil || out.ExitCode != 0 {
		return nil, false
	}
	var env map[string]string
	if json.Unmarshal(out.Stdout, &env) != nil {
		return nil, false
	}
	for _, key := range keys {
		if _, ok := env[key]; !ok {
			return nil, false
		}
	}
	if env["GOOS"] == "" || env["GOARCH"] == "" || env["CGO_ENABLED"] == "" {
		return nil, false
	}
	return env, true
}

// suppliedCoverageSettings is the supplied-coverage config that changes what is
// measured: whether it is on and which artifacts it reads. The gate decides what
// blocks, so it never enters the hash. Artifact bytes are not hashed either:
// they change on every CI run, and freshness is checked per run.
func suppliedCoverageSettings(o suppliedcoverage.Options) suppliedcoverage.Options {
	o.Gate = ""
	return o
}
