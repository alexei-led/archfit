package py

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const toolPythonEnv = "ARCHFIT_TOOL_PYTHON"

// Python's isolated mode still loads the owned environment's installed packages.
// Clear import/venv selectors as well, including for the uv parent process.
func isolatedPythonEnv() []string {
	return []string{"PYTHONPATH=", "PYTHONHOME=", "VIRTUAL_ENV=", "PYTHONUSERBASE=", "PYTHONNOUSERSITE=1", "__PYVENV_LAUNCHER__="}
}

func validateToolPython(root, python string) error {
	if !filepath.IsAbs(python) {
		return fmt.Errorf("%s must name an absolute interpreter outside the target", toolPythonEnv)
	}
	inside, err := toolrun.ContainsPath(root, python)
	if err != nil {
		return fmt.Errorf("%s: validate interpreter location: %w", toolPythonEnv, err)
	}
	if inside {
		return fmt.Errorf("%s must not select a target interpreter", toolPythonEnv)
	}
	info, err := os.Stat(python)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s must name an executable Python interpreter", toolPythonEnv)
	}
	return nil
}

func (e *Extractor) provisionedPython(ctx context.Context, root, python string) (string, error) {
	if err := validateToolPython(root, python); err != nil {
		return "", err
	}
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    python,
		Args:    []string{"-I", "-c", "import sys, grimp; from importlib.metadata import version; assert sys.version_info >= (3, 14), 'Python 3.14+ required'; assert version('grimp') == '3.17', 'grimp==3.17 required'; print(sys.version.split()[0])"},
		WorkDir: os.TempDir(), Env: isolatedPythonEnv(), Timeout: 30 * time.Second,
	})
	if err != nil {
		return "", fmt.Errorf("%s: validate Python/grimp: %w", toolPythonEnv, err)
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("%s: validate Python/grimp: %s", toolPythonEnv, strings.TrimSpace(string(out.Stderr)))
	}
	return strings.TrimSpace(string(out.Stdout)), nil
}
