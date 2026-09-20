package ts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
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
	for _, name := range tsManifestNames {
		if !strings.HasPrefix(name, ".dependency-cruiser.") || filepath.Ext(name) == ".json" {
			continue
		}
		if _, err := os.Stat(filepath.Join(workDir, name)); !os.IsNotExist(err) {
			return "", fmt.Errorf("dependency-cruiser dynamic configuration inputs unresolved: %s", name)
		}
	}
	inputs := map[string]string{"tsconfig": compiler}
	const configName = ".dependency-cruiser.json"
	data, err := os.ReadFile(filepath.Join(workDir, configName)) //nolint:gosec // extractor's configuration file
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("dependency-cruiser configuration: %w", err)
	}
	if err == nil {
		var config struct {
			Extends json.RawMessage `json:"extends"`
			Options struct {
				TSConfig               json.RawMessage `json:"tsConfig"`
				WebpackConfig          json.RawMessage `json:"webpackConfig"`
				BabelConfig            json.RawMessage `json:"babelConfig"`
				EnhancedResolveOptions struct {
					Plugins json.RawMessage `json:"plugins"`
				} `json:"enhancedResolveOptions"`
				ExternalModuleResolutionStrategy string `json:"externalModuleResolutionStrategy"`
			} `json:"options"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return "", fmt.Errorf("dependency-cruiser configuration: %w", err)
		}
		if len(config.Extends) != 0 || len(config.Options.TSConfig) != 0 ||
			len(config.Options.WebpackConfig) != 0 || len(config.Options.BabelConfig) != 0 ||
			len(config.Options.EnhancedResolveOptions.Plugins) != 0 || config.Options.ExternalModuleResolutionStrategy == "yarn-pnp" {
			return "", fmt.Errorf("dependency-cruiser configuration inputs unresolved: %s", configName)
		}
		inputs[configName] = string(data)
	}
	return factcache.HashJSON(inputs)
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
