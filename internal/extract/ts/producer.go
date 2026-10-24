package ts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Target package managers, runtime preload hooks and depcruise configuration
// are not part of the producer. PATH is used only to detect the two owned
// executables; the validated Node path runs the producer without its shebang.
var producerEnv = []string{"NODE_OPTIONS=", "NODE_PATH=", "NO_COLOR=1", "FORCE_COLOR=0"}

var producerArgs = []string{"--no-config", "--no-cache", "--output-type", "json", "--exclude", thirdPartyExclude}

func scanArgs(files []string) []string {
	args := append([]string(nil), producerArgs...)
	if needsTypeScript(files) {
		args = append(args, "--ts-pre-compilation-deps")
	}
	return args
}

type producerInfo struct {
	Version    string
	Node       string
	JavaScript string
	TypeScript string
	SupportsTS bool
}

func (e *Extractor) producerPath(ctx context.Context, roots ...string) (string, error) {
	info, ok := e.runner.Detect(ctx, toolName)
	if !ok || !filepath.IsAbs(info.Path) {
		return "", errors.New("owned depcruise executable not found on PATH; install dependency-cruiser with a compatible TypeScript compiler outside the target project")
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		inside, err := toolrun.ContainsPath(root, info.Path)
		if err != nil {
			return "", fmt.Errorf("validate dependency-cruiser location: %w", err)
		}
		if inside {
			return "", fmt.Errorf("refusing target-project dependency-cruiser executable: %s", info.Path)
		}
	}
	return info.Path, nil
}

// nodeInterpreterPath rejects project-owned interpreters before any execution,
// including both lexical paths and symlink destinations. Never fall back to an
// interpreter selected by the producer's /usr/bin/env shebang.
func (e *Extractor) nodeInterpreterPath(ctx context.Context, roots ...string) (string, error) {
	info, ok := e.runner.Detect(ctx, "node")
	if !ok || !filepath.IsAbs(info.Path) {
		return "", errors.New("owned Node interpreter not found on PATH")
	}
	interpreter, err := filepath.EvalSymlinks(info.Path)
	if err != nil {
		return "", fmt.Errorf("resolve owned Node interpreter: %w", err)
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		inside, err := toolrun.ContainsPath(root, info.Path)
		if err != nil {
			return "", fmt.Errorf("validate Node location: %w", err)
		}
		if inside {
			return "", fmt.Errorf("refusing target-project Node interpreter: %s", info.Path)
		}
	}
	return interpreter, nil
}

// Both supported depcruise generations expose compiler availability via --info;
// v17 does not include environment metadata in the JSON graph. Probe the exact
// executable, without a target working directory or runtime module injection.
func (e *Extractor) inspectProducer(ctx context.Context, binary, interpreter string) (producerInfo, error) {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name: interpreter, Args: []string{binary, "--no-config", "--info"}, Env: producerEnv, Timeout: 30 * time.Second,
	})
	if err != nil {
		return producerInfo{}, err
	}
	if out.ExitCode != 0 {
		return producerInfo{}, fmt.Errorf("dependency-cruiser environment probe exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	var info producerInfo
	for _, line := range strings.Split(string(out.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "dependency-cruiser@") {
			info.Version = strings.TrimPrefix(line, "dependency-cruiser@")
		}
		if strings.HasPrefix(line, "node version found") {
			_, info.Node, _ = strings.Cut(line, ":")
			info.Node = strings.TrimSpace(info.Node)
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		switch fields[1] {
		case "javascript":
			if fields[0] == "✔" {
				info.JavaScript = fields[len(fields)-1]
			}
		case "typescript":
			info.TypeScript = fields[len(fields)-1]
			info.SupportsTS = fields[0] == "✔" && strings.HasPrefix(info.TypeScript, "typescript@")
		}
	}
	if info.Version == "" || info.Node == "" || info.JavaScript == "" || info.TypeScript == "" {
		return info, errors.New("dependency-cruiser compiler/runtime environment could not be identified")
	}
	return info, nil
}

func eligibleSources(root string) ([]string, error) {
	files := factcache.ListInputs(root, isSource, []string{"node_modules/**", "**/node_modules/**"})
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file == "." {
			return nil, errors.New("cannot inventory TypeScript source inputs: unreadable, cyclic, or too large")
		}
		physical, err := filepath.EvalSymlinks(filepath.Join(root, file))
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(base, physical)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("TypeScript source symlink leaves input scope: %s", file)
		}
	}
	return files, nil
}

func needsTypeScript(files []string) bool {
	for _, file := range files {
		switch filepath.Ext(file) {
		case ".ts", ".tsx", ".mts", ".cts":
			return true
		}
	}
	return false
}

func incompleteCoverage(version, reason string, applicable int) evidence.Coverage {
	return evidence.Coverage{
		Tool: coverageTool, Version: version, Status: statusPartial, Reason: reason,
		FilesApplicable: applicable, UnresolvedInputsMissing: max(1, applicable),
	}
}

func missingSourceCount(dc dcOutput, files []string, srcArg string) int {
	seen := make(map[string]bool, len(dc.Modules))
	for _, mod := range dc.Modules {
		if !mod.CoreModule && !mod.CouldNotResolve {
			seen[filepath.ToSlash(filepath.Clean(mod.Source))] = true
		}
	}
	missing := 0
	for _, file := range files {
		if !seen[filepath.ToSlash(filepath.Join(srcArg, file))] {
			missing++
		}
	}
	return missing
}

func environmentIssue(dc dcOutput) string {
	if len(dc.Summary.Environment.Issues) == 0 {
		return ""
	}
	issue := dc.Summary.Environment.Issues[0]
	return fmt.Sprintf("dependency-cruiser environment incomplete (%s): %s", issue.Name, issue.Description)
}
