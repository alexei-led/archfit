package ts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

// MeasurementConfigHash identifies the extractor's supported configuration
// inputs without including the checkout location. Unresolved configuration
// dependencies return an error rather than claiming a complete identity.
func MeasurementConfigHash(s scope.Scope, cfg evidenceports.ExtractConfig) (string, error) {
	workDir := s.Root
	if s.SubtreePrefix != "" {
		workDir = s.GitRoot
	}
	e := Extractor{cfg: cfg}
	compiler, err := tsConfigInputsHash(e.resolveTSConfig(s.Root, workDir), workDir)
	if err != nil {
		return "", fmt.Errorf("typescript compiler configuration: %w", err)
	}
	src := cfg.Src
	if src == "" {
		src = "src"
	}
	files, err := eligibleSources(filepath.Join(s.Root, src))
	if err != nil {
		return "", err
	}
	return factcache.HashJSON(struct {
		Compiler string
		Args     []string
		Env      []string
	}{compiler, scanArgs(files), producerEnv})
}

// tsConfigInputsHash includes the complete explicit local extends chain, even
// outside the scan root. Unsupported TypeScript resolution or JSONC syntax
// vetoes caching rather than guessing which config files the compiler reads.
func tsConfigInputsHash(path, workDir string) (string, error) {
	if path == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	base, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	inputs := make(map[string]string)
	visiting := make(map[string]bool)
	var visit func(string) error
	visit = func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if visiting[abs] {
			return fmt.Errorf("cyclic tsconfig extends: %s", abs)
		}
		rel, err := filepath.Rel(base, abs)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if _, ok := inputs[key]; ok {
			return nil
		}
		data, err := os.ReadFile(abs) //nolint:gosec // compiler config paths intentionally include parents outside the scan root
		if err != nil {
			return err
		}
		var config map[string]json.RawMessage
		if err := json.Unmarshal(data, &config); err != nil {
			return err
		}
		var parents []string
		if raw, ok := config["extends"]; ok {
			var parent string
			if err := json.Unmarshal(raw, &parent); err == nil && parent != "" {
				parents = []string{parent}
			} else if err := json.Unmarshal(raw, &parents); err != nil || len(parents) == 0 {
				return fmt.Errorf("unsupported tsconfig extends: %s", abs)
			}
		}
		visiting[abs] = true
		for _, parent := range parents {
			if filepath.Ext(parent) != ".json" || (!filepath.IsAbs(parent) && !strings.HasPrefix(parent, "./") && !strings.HasPrefix(parent, "../")) {
				return fmt.Errorf("unsupported tsconfig extends resolution: %s", parent)
			}
			if !filepath.IsAbs(parent) {
				parent = filepath.Join(filepath.Dir(abs), parent)
			}
			if err := visit(parent); err != nil {
				return err
			}
		}
		visiting[abs] = false
		inputs[key] = string(data)
		return nil
	}
	if err := visit(path); err != nil {
		return "", err
	}
	return factcache.HashJSON(inputs)
}
