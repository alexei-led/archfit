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
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("%s: resolve target: %w", toolPythonEnv, err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return fmt.Errorf("%s: absolute target: %w", toolPythonEnv, err)
	}
	resolved, err := filepath.EvalSymlinks(python)
	if err != nil {
		return fmt.Errorf("%s: resolve interpreter: %w", toolPythonEnv, err)
	}
	// Resolve the parent separately: a target venv's python may itself be a
	// symlink to a trusted base executable while loading target site-packages.
	parent, err := filepath.EvalSymlinks(filepath.Dir(python))
	if err != nil {
		return fmt.Errorf("ARCHFIT_TOOL_PYTHON: resolve interpreter directory: %w", err)
	}
	for _, path := range []string{filepath.Join(parent, filepath.Base(python)), resolved} {
		if rel, err := filepath.Rel(resolvedRoot, path); err != nil || rel == "." || filepath.IsLocal(rel) {
			return fmt.Errorf("%s must not select a target interpreter", toolPythonEnv)
		}
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
