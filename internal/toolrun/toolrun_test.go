package toolrun

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDetect_Found(t *testing.T) {
	r := New()
	// "sh" is always present on macOS/Linux
	info, ok := r.Detect(context.Background(), "sh")
	if !ok {
		t.Fatal("Detect(sh) returned false, want true")
	}
	if info.Name != "sh" {
		t.Errorf("Name = %q, want %q", info.Name, "sh")
	}
	if info.Path == "" {
		t.Error("Path is empty")
	}
}

func TestDetect_NotFound(t *testing.T) {
	r := New()
	info, ok := r.Detect(context.Background(), "definitely-not-a-real-binary-xyz")
	if ok {
		t.Errorf("Detect returned true, want false; info=%+v", info)
	}
	if info != (ToolInfo{}) {
		t.Errorf("info = %+v, want zero value", info)
	}
}

func TestRun_CapturesOutput(t *testing.T) {
	r := New()
	out, err := r.Run(context.Background(), ToolCmd{
		Name: "sh",
		Args: []string{"-c", "echo hello-archfit"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", out.ExitCode)
	}
	if !strings.Contains(string(out.Stdout), "hello-archfit") {
		t.Errorf("Stdout = %q, want to contain %q", out.Stdout, "hello-archfit")
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	r := New()
	out, err := r.Run(context.Background(), ToolCmd{
		Name: "sh",
		Args: []string{"-c", "exit 1"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v (want nil, non-zero exit is not an error)", err)
	}
	if out.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", out.ExitCode)
	}
}

func TestRun_EnvPinned(t *testing.T) {
	r := New()
	out, err := r.Run(context.Background(), ToolCmd{
		Name: "sh",
		Args: []string{"-c", "echo $LC_ALL"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(string(out.Stdout), "C") {
		t.Errorf("Stdout = %q, want LC_ALL=C to be visible", out.Stdout)
	}
}

func TestRun_Timeout(t *testing.T) {
	r := New()
	out, err := r.Run(context.Background(), ToolCmd{
		Name:    "sh",
		Args:    []string{"-c", "sleep 10"},
		Timeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Errorf("Run returned nil error, want non-nil (timeout should cause context cancellation); out=%+v", out)
	}
}

func TestRun_GitWorkDirIgnoresHookRepoEnv(t *testing.T) {
	r := New()
	repoRootOut, err := exec.Command(gitCommand, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	gitDirOut, err := exec.Command(gitCommand, "rev-parse", "--git-dir").Output()
	if err != nil {
		t.Fatalf("git rev-parse --git-dir: %v", err)
	}
	t.Setenv("GIT_WORK_TREE", strings.TrimSpace(string(repoRootOut)))
	t.Setenv("GIT_DIR", strings.TrimSpace(string(gitDirOut)))

	out, err := r.Run(context.Background(), ToolCmd{
		Name:    gitCommand,
		Args:    []string{"rev-parse", "--show-toplevel"},
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if out.ExitCode == 0 {
		t.Fatalf("ExitCode = 0, want non-zero when WorkDir is not a git repo; stdout=%q stderr=%q", out.Stdout, out.Stderr)
	}
	if !strings.Contains(strings.ToLower(string(out.Stderr)), "not a git repository") {
		t.Fatalf("stderr = %q, want not-a-git-repository message", out.Stderr)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	out, err = r.Run(context.Background(), ToolCmd{
		Name:    gitCommand,
		Args:    []string{"rev-parse", "--show-toplevel"},
		WorkDir: cwd,
	})
	if err != nil {
		t.Fatalf("Run(repo): %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("ExitCode(repo) = %d, want 0; stderr=%q", out.ExitCode, out.Stderr)
	}
}
