package registry

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// Doctor runs the SDK and the helper the way analysis does: in the checked
// directory, where a global.json selects the SDK. The helper file name and the
// SDK resolver's global.json lines mirror internal/extract/dotnet, which owns
// running them during analysis.
const (
	roslynHelperDLL      = "archfit-roslyn.dll"
	roslynHelperPrefix   = "archfit-roslyn "
	roslynHelperErrorTag = "archfit-roslyn: "
	dotnetVersionTimeout = 30 * time.Second
)

var (
	globalJSONFileLine = regexp.MustCompile(`(?m)^global\.json file:\s*(.+?)\s*$`)
	requestedSDKLine   = regexp.MustCompile(`(?m)^Requested SDK version:\s*(\S+)`)
	dotnetDoctorEnv    = []string{"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1"}
)

// dotnetSDKVersion is the SDK `dotnet --version` selects in dir.
func dotnetSDKVersion(ctx context.Context, r toolrun.Runner, _, dir string) (string, error) {
	out, err := runDotnet(ctx, r, dir, "dotnet --version", "--version")
	if err != nil {
		return "", err
	}
	return "SDK " + strings.TrimSpace(firstLine(string(out.Stdout))), nil
}

// roslynHelperVersion is the helper's identity line ("archfit-roslyn <v>;
// Microsoft.CodeAnalysis <v>; SDK <v>").
func roslynHelperVersion(ctx context.Context, r toolrun.Runner, helperDir, dir string) (string, error) {
	out, err := runDotnet(ctx, r, dir, "archfit-roslyn --version", filepath.Join(helperDir, roslynHelperDLL), "--version")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(out.Stdout), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, roslynHelperPrefix) {
			return line, nil
		}
	}
	return "", fmt.Errorf("archfit-roslyn --version printed no identity line; rebuild it with `make dotnet-roslyn`")
}

// runDotnet runs dotnet with args in dir; label names the command in errors.
func runDotnet(ctx context.Context, r toolrun.Runner, dir, label string, args ...string) (toolrun.Output, error) {
	out, err := r.Run(ctx, toolrun.ToolCmd{Name: cmdDotNet, Args: args, Env: dotnetDoctorEnv, WorkDir: dir, Timeout: dotnetVersionTimeout})
	if err != nil {
		return out, fmt.Errorf("%s in %s: %w", label, dir, err)
	}
	if out.ExitCode != 0 {
		return out, fmt.Errorf("%s fails in %s: %s", label, dir, dotnetFailure(out, dir))
	}
	return out, nil
}

// dotnetFailure names why a dotnet command failed: an unsatisfiable global.json
// pin by its file and requested SDK, a helper error by its own message, else
// the first line of output. A global.json under dir is named relative to it.
func dotnetFailure(out toolrun.Output, dir string) string {
	text := string(out.Stderr) + "\n" + string(out.Stdout)
	if match := globalJSONFileLine.FindStringSubmatch(text); match != nil {
		file := match[1]
		if rel, err := filepath.Rel(dir, file); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			file = filepath.ToSlash(rel)
		}
		if sdk := requestedSDKLine.FindStringSubmatch(text); sdk != nil {
			return fmt.Sprintf("%s requests SDK %s, which is not installed; install a matching SDK or relax the pin", file, sdk[1])
		}
		return file + " cannot be satisfied by the installed SDKs; install a matching SDK or relax the pin"
	}
	for line := range strings.SplitSeq(text, "\n") {
		if msg, ok := strings.CutPrefix(strings.TrimSpace(line), roslynHelperErrorTag); ok {
			return msg
		}
	}
	if line := strings.TrimSpace(firstLine(strings.TrimSpace(text))); line != "" {
		return line
	}
	return fmt.Sprintf("exit %d", out.ExitCode)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
