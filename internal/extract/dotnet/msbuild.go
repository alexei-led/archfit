package dotnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	toolDotnet      = "dotnet"
	evalTimeout     = 5 * time.Minute
	propTargetFW    = "TargetFramework"
	propTargetFWs   = "TargetFrameworks"
	propIsTest      = "IsTestProject"
	itemProjectRef  = "ProjectReference"
	itemPackageRef  = "PackageReference"
	tfmOverrideFlag = "-p:TargetFramework="
)

// msbuildEnv keeps the SDK quiet and its worker nodes from outliving the call.
var msbuildEnv = []string{
	"DOTNET_CLI_TELEMETRY_OPTOUT=1",
	"DOTNET_NOLOGO=1",
	"DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1",
	"MSBUILDDISABLENODEREUSE=1",
}

var (
	sdkNotFound      = regexp.MustCompile(`The SDK '([^']+)' specified could not be found`)
	globalJSONFile   = regexp.MustCompile(`(?m)^global\.json file:\s*(.+?)\s*$`)
	globalJSONSDK    = regexp.MustCompile(`(?m)^Requested SDK version:\s*(\S+)`)
	errTimedOut      = errors.New("evaluation timed out")
	errEvalNotParsed = errors.New("unparsable dotnet msbuild output")
)

// evalResult is one `dotnet msbuild` evaluation: either parsed properties and
// items, or a failure reason.
type evalResult struct {
	props map[string]string
	refs  []evalItem
	pkgs  []evalItem
	// failure is the reason the evaluation failed; empty on success.
	failure string
}

// evalItem is one MSBuild item as reported by -getItem.
type evalItem struct {
	Identity string `json:"Identity"`
	Version  string `json:"Version"`
	FullPath string `json:"FullPath"`
	// DefiningProjectFullPath is the file that declared the item: the project or
	// an imported props file.
	DefiningProjectFullPath string `json:"DefiningProjectFullPath"`
}

type msbuildJSON struct {
	Properties map[string]string     `json:"Properties"`
	Items      map[string][]evalItem `json:"Items"`
}

// evaluate runs one `dotnet msbuild` evaluation of project (an absolute path),
// optionally with TargetFramework forced to tfm. A non-zero exit or unparsable
// output is a failed result, not an error. A timeout returns errTimedOut. An
// exec-level failure is returned as an error.
func evaluate(ctx context.Context, runner toolrun.Runner, root, project, tfm string) (evalResult, error) {
	args := []string{
		"msbuild", filepath.Base(project),
		"-getItem:" + itemProjectRef,
		"-getItem:" + itemPackageRef,
		"-getProperty:" + propTargetFW,
		"-getProperty:" + propTargetFWs,
		"-getProperty:" + propIsTest,
	}
	if tfm != "" {
		args = append(args, tfmOverrideFlag+tfm)
	}
	out, err := runner.Run(ctx, toolrun.ToolCmd{
		Name:    toolDotnet,
		Args:    args,
		Env:     msbuildEnv,
		WorkDir: filepath.Dir(project),
		Timeout: evalTimeout,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return evalResult{}, errTimedOut
		}
		return evalResult{}, fmt.Errorf("run dotnet msbuild: %w", err)
	}
	if out.ExitCode != 0 {
		return evalResult{failure: failureReason(out, root)}, nil
	}
	res, perr := parseEvaluation(out.Stdout)
	if perr != nil {
		return evalResult{failure: perr.Error()}, nil
	}
	return res, nil
}

// parseEvaluation decodes the JSON that two or more -getProperty/-getItem flags
// print: {"Properties":{...},"Items":{...}}.
func parseEvaluation(data []byte) (evalResult, error) {
	var doc msbuildJSON
	if err := json.Unmarshal(bytes.TrimSpace(data), &doc); err != nil {
		return evalResult{}, fmt.Errorf("%w: %w", errEvalNotParsed, err)
	}
	return evalResult{props: doc.Properties, refs: doc.Items[itemProjectRef], pkgs: doc.Items[itemPackageRef]}, nil
}

// failureReason maps a failed evaluation to a reason that names the cause and
// the fix. MSB4236 names the SDK, a global.json mismatch names the file, and
// anything else keeps the first line that reports an error.
func failureReason(out toolrun.Output, root string) string {
	text := string(out.Stderr) + "\n" + string(out.Stdout)
	if m := sdkNotFound.FindStringSubmatch(text); m != nil {
		name, _, _ := strings.Cut(m[1], "/")
		return fmt.Sprintf("MSB4236: SDK '%s' not resolvable offline; run with NuGet access and a warm cache", name)
	}
	if f := globalJSONFile.FindStringSubmatch(text); f != nil {
		file := relativeTo(root, f[1])
		if v := globalJSONSDK.FindStringSubmatch(text); v != nil {
			return fmt.Sprintf("%s requests SDK %s, which is not installed; install a matching SDK or relax the pin", file, v[1])
		}
		return file + " cannot be satisfied by the installed SDKs; install a matching SDK or relax the pin"
	}
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(strings.ToLower(line), "error") {
			return strings.TrimSpace(strings.ReplaceAll(line, filepath.ToSlash(root)+"/", ""))
		}
	}
	return fmt.Sprintf("dotnet msbuild exited %d", out.ExitCode)
}

// relativeTo spells abs relative to root when it lies inside it, else as given.
func relativeTo(root, abs string) string {
	if rel, ok := relToRoot(root, abs); ok && rel != "" {
		return rel
	}
	return filepath.ToSlash(abs)
}

// relToRoot returns abs as a slash path relative to root, or false when abs lies
// outside it. Both the given and the symlink-resolved spelling of root are tried,
// since MSBuild reports real paths (macOS /tmp is /private/tmp).
func relToRoot(root, abs string) (string, bool) {
	spellings := []string{root}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		spellings = append(spellings, resolved)
	}
	for _, r := range spellings {
		rel, err := filepath.Rel(r, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if rel == "." {
			return "", true
		}
		return filepath.ToSlash(rel), true
	}
	return "", false
}

// splitTFMs splits an MSBuild `a;b` list, dropping blanks and duplicates.
func splitTFMs(list string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, t := range strings.Split(list, ";") {
		t = strings.TrimSpace(t)
		if _, dup := seen[t]; t == "" || dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
