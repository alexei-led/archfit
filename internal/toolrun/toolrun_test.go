package toolrun

import (
	"context"
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
