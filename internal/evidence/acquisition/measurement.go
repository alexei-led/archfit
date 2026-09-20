package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func (s *Service) measurementProfile(ctx context.Context, sc scope.Scope, rows []evidence.Coverage, history *evidence.VolatilityCorroboration) *evidence.MeasurementProfile {
	p := &evidence.MeasurementProfile{Version: evidence.MeasurementProfileVersion, Producers: []evidence.MeasurementProducer{}, Unknowns: []string{}}
	settings := map[string]any{
		"extractors": s.Options.Extractors, "exclusions": s.Options.Exclusions,
		"file_class": s.Options.Acquisition.FileClass, "syntax": s.Options.Syntax,
		"patterns": s.Options.Patterns,
	}
	for _, row := range rows {
		if row.Tool == "" {
			continue
		}
		semantics, _ := evidence.MeasurementContract(row.Tool)
		version := strings.TrimSpace(row.Version)
		if row.Status == evidence.StatusAbsent || row.Status == evidence.StatusDisabled {
			version = ""
		}
		p.Producers = append(p.Producers, evidence.MeasurementProducer{Tool: row.Tool, SemanticsVersion: semantics, ToolVersion: version, Status: row.Status, PartialBasis: measurementPartialBasis(row)})
		if row.Tool == registry.ToolGoPackages && row.Status != evidence.StatusAbsent && row.Status != evidence.StatusDisabled {
			env, ok := s.measurementGoEnv(ctx, sc.Root)
			if !ok {
				p.Unknowns = append(p.Unknowns, "go/packages build environment is unknown")
			} else {
				settings["go_environment"] = env
			}
		}
		if row.Tool == registry.ToolDepCruiser && row.Status != evidence.StatusAbsent && row.Status != evidence.StatusDisabled {
			if row.MeasurementSettingsHash != "" {
				settings["typescript_config"] = row.MeasurementSettingsHash
				continue
			}
			hash, err := ts.MeasurementConfigHash(sc, s.Options.Extractors["typescript"])
			if err != nil {
				p.Unknowns = append(p.Unknowns, "dependency-cruiser measurement configuration is unknown")
			} else {
				settings["typescript_config"] = hash
			}
		}
	}
	historySemantics, _ := evidence.MeasurementContract("git-history")
	historyProducer := evidence.MeasurementProducer{Tool: "git-history", SemanticsVersion: historySemantics, Status: evidence.StatusAbsent}
	if history != nil {
		historyProducer.Status = history.Status
		historyProducer.ToolVersion = s.measurementToolVersion(ctx, sc.GitRoot, "git")
	}
	p.Producers = append(p.Producers, historyProducer)
	sort.Slice(p.Producers, func(i, j int) bool { return p.Producers[i].Tool < p.Producers[j].Tool })
	data, err := json.Marshal(settings)
	if err != nil {
		p.Unknowns = append(p.Unknowns, "measurement settings could not be encoded")
	} else {
		hash := sha256.Sum256(data)
		p.SettingsHash = hex.EncodeToString(hash[:])
	}
	return p
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
	out, err := s.Runner.Run(ctx, toolrun.ToolCmd{Name: "go", Args: append([]string{"env", "-json"}, keys...), WorkDir: root, Timeout: 10 * time.Second})
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
