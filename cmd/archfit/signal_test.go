//go:build unix

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const signalHelperEnv = "ARCHFIT_SIGNAL_TEST_ARGS"

// TestSignalHelperProcess is the archfit process TestSignal_StopsToolsAndRemovesTheSnapshot
// signals: the test binary re-executed to run Run, so the signal reaches a
// real process and not the test runner.
func TestSignalHelperProcess(t *testing.T) {
	args, ok := os.LookupEnv(signalHelperEnv)
	if !ok {
		t.Skip("helper process only")
	}
	os.Exit(Run(strings.Split(args, "\x1f"), os.Stdout))
}

// fakeDotnet answers the SDK and evaluation calls, and blocks the Roslyn
// helper run after writing its PID and snapshot path to $ARCHFIT_FAKE_MARK.
const fakeDotnet = `#!/bin/sh
case "$1" in
  --version) echo 10.0.402; exit 0;;
  msbuild) echo '{"Properties":{"TargetFramework":"net10.0"},"Items":{}}'; exit 0;;
esac
if [ "$2" = "--version" ]; then echo "archfit-roslyn 1.1.0; Microsoft.CodeAnalysis 5.6.0; SDK 10.0.402"; exit 0; fi
echo "$$ $2" > "$ARCHFIT_FAKE_MARK.tmp" && mv "$ARCHFIT_FAKE_MARK.tmp" "$ARCHFIT_FAKE_MARK"
exec sleep 60
`

func TestSignal_StopsToolsAndRemovesTheSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess test")
	}
	dir := t.TempDir()
	mustWrite := func(rel, content string, mode os.FileMode) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("bin/dotnet", fakeDotnet, 0o700)
	mustWrite("helper/archfit-roslyn.dll", "", 0o600)
	mustWrite("repo/A/A.csproj", `<Project Sdk="Microsoft.NET.Sdk"/>`, 0o600)
	mustWrite("repo/.archfit.yaml", "version: 2\nmodules:\n  a:\n    paths: [\"A/**\"]\n", 0o600)
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	mark := filepath.Join(dir, "mark")
	repo := filepath.Join(dir, "repo")

	cmd := exec.Command(os.Args[0], "-test.run=^TestSignalHelperProcess$") //nolint:gosec // re-executes this test binary
	cmd.Env = append(os.Environ(),
		signalHelperEnv+"="+strings.Join([]string{cmdAnalyze, "--config", filepath.Join(repo, ".archfit.yaml"), "--root", repo, "--format", "json"}, "\x1f"),
		"PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+tmp,
		"ARCHFIT_DOTNET_ROSLYN="+filepath.Join(dir, "helper"),
		"ARCHFIT_FAKE_MARK="+mark,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var helperPID int
	var snapshot string
	for deadline := time.Now().Add(30 * time.Second); helperPID == 0; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("the helper never ran; stderr:\n%s", stderr.String())
		}
		if data, err := os.ReadFile(mark); err == nil { //nolint:gosec // test file
			pid, snap, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
			helperPID, _ = strconv.Atoi(pid)
			snapshot = snap
		}
	}
	if _, err := os.Stat(snapshot); err != nil { //nolint:gosec // path the fake helper wrote, under the test temp dir
		t.Fatalf("snapshot %s missing while the helper runs: %v", snapshot, err)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 3 {
			t.Errorf("exit = %v, want code 3", err)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("archfit did not exit after SIGTERM")
	}

	if !strings.Contains(stderr.String(), "interrupted by a signal") {
		t.Errorf("stderr does not say the run was interrupted:\n%s", stderr.String())
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("temp dir keeps %v after SIGTERM, want the snapshot removed", entries)
	}
	if err := syscall.Kill(helperPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("helper process %d still exists (kill 0: %v)", helperPID, err)
	}
}
