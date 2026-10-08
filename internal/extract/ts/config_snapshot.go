package ts

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

//go:embed config_snapshot.cjs
var configSnapshotSource []byte

const (
	snapshotJSONConfig      = ".dependency-cruiser.json"
	snapshotJSConfig        = ".dependency-cruiser.js"
	snapshotCJSConfig       = ".dependency-cruiser.cjs"
	snapshotMJSConfig       = ".dependency-cruiser.mjs"
	snapshotJSONExtension   = ".json"
	snapshotDefaultTSConfig = "tsconfig.json"
)

type nativeConfigSnapshot struct {
	Version  string          `json:"version"`
	Config   json.RawMessage `json:"config"`
	Unknown  bool            `json:"unknown"`
	Fallback bool            `json:"fallback"`
}

func (e *Extractor) runWithConfigSnapshot(ctx context.Context, sc scope.Scope, version, tsConfig, workDir string, cmd toolrun.ToolCmd) (toolrun.Output, string, string, error) {
	config := dynamicConfigPath(workDir)
	if config == "" {
		out, err := e.cachedRunner(sc, version, tsConfig, workDir).Run(ctx, cmd)
		return out, "", version, err
	}
	dir, err := os.MkdirTemp("", "archfit-depcruise-config-")
	if err != nil {
		out, runErr := e.runner.Run(ctx, cmd)
		return out, "", version, runErr
	}
	defer os.RemoveAll(dir) //nolint:errcheck // private temporary snapshot only
	outputPath := filepath.Join(dir, "snapshot.json")
	input, err := json.Marshal(struct {
		Config string `json:"config"`
		Output string `json:"output"`
	}{config, outputPath})
	if err != nil {
		return toolrun.Output{}, "", version, err
	}
	source := append(append([]byte("const archfitSnapshot = "), input...), []byte(";\n")...)
	source = append(source, configSnapshotSource...)
	wrapper := filepath.Join(dir, "config.cjs")
	if err := os.WriteFile(wrapper, source, 0o600); err != nil {
		out, runErr := e.runner.Run(ctx, cmd)
		return out, "", version, runErr
	}
	frozenCmd := cmd
	frozenCmd.Args = append(append([]string(nil), cmd.Args...), "--config", wrapper)
	out, runErr := e.runner.Run(ctx, frozenCmd)
	data, readErr := os.ReadFile(outputPath) //nolint:gosec // private tempfile created above
	var snapshot nativeConfigSnapshot
	if readErr != nil || json.Unmarshal(data, &snapshot) != nil {
		return out, "", version, runErr
	}
	if snapshot.Fallback {
		out, runErr = e.runner.Run(ctx, cmd)
		return out, "", version, runErr
	}
	if snapshot.Version != "" {
		version = snapshot.Version
	}
	if snapshot.Unknown || len(snapshot.Config) == 0 {
		return out, "", version, runErr
	}
	hash := consumedConfigHash(snapshot.Config, tsConfig, workDir)
	return out, hash, version, runErr
}

func dynamicConfigPath(root string) string {
	for _, name := range []string{snapshotJSONConfig, snapshotJSConfig, snapshotCJSConfig, snapshotMJSConfig} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			if filepath.Ext(path) == snapshotJSONExtension {
				return ""
			}
			absolute, err := filepath.Abs(path)
			if err == nil {
				return absolute
			}
			return ""
		}
	}
	return ""
}

func consumedConfigHash(raw json.RawMessage, tsConfig, workDir string) string {
	if canonical, err := filepath.EvalSymlinks(workDir); err == nil {
		workDir = canonical
	}
	var config map[string]any
	if json.Unmarshal(raw, &config) != nil {
		return ""
	}
	options, _ := config["options"].(map[string]any)
	if tsConfig == "" {
		if value, ok := options["tsConfig"].(map[string]any); ok {
			tsConfig, _ = value["fileName"].(string)
			if tsConfig == "" {
				tsConfig = snapshotDefaultTSConfig
			}
		}
	}
	compilerHash, err := tsConfigInputsHash(tsConfig, workDir)
	if err != nil {
		return ""
	}
	normalizeConfigPaths(options, workDir)
	hash, err := factcache.HashJSON(struct {
		Config   map[string]any
		Compiler string
	}{config, compilerHash})
	if err != nil {
		return ""
	}
	return hash
}

func normalizeConfigPaths(options map[string]any, root string) {
	if value, ok := options["baseDir"].(string); ok {
		options["baseDir"] = relativeConfigPath(value, root)
	}
	if tsConfig, ok := options["tsConfig"].(map[string]any); ok {
		if value, ok := tsConfig["fileName"].(string); ok {
			tsConfig["fileName"] = relativeConfigPath(value, root)
		}
	}
}

func relativeConfigPath(value, root string) string {
	if !filepath.IsAbs(value) {
		return value
	}
	if relative, err := filepath.Rel(root, value); err == nil {
		return filepath.ToSlash(relative)
	}
	return value
}
