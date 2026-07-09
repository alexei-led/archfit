package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvSearchDirs(t *testing.T) {
	t.Parallel()
	const repo = "/repo"
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"root space form", []string{cmdAnalyze, flagRoot, repo}, []string{repo}},
		{"root equals form", []string{cmdAnalyze, flagRoot + "=" + repo}, []string{repo}},
		{"root short", []string{"init", "-r", repo}, []string{repo}},
		{"config dir space form", []string{cmdAnalyze, "--config", "/x/cfg.yaml"}, []string{"/x"}},
		{"config short equals", []string{"-c=/x/y/cfg.yaml"}, []string{"/x/y"}},
		{"both root and config", []string{flagRoot, repo, "-c", "/cfg/.archfit.yaml"}, []string{repo, "/cfg"}},
		{"none", []string{cmdAnalyze, flagJSON}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := envSearchDirs(tc.args)
			if len(got) != len(tc.want) {
				t.Fatalf("args %v: got %v, want %v", tc.args, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("args %v: got[%d]=%q, want %q", tc.args, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# a comment\n\nexport ARCHFIT_TEST_A=plain\nARCHFIT_TEST_B=\"quoted value\"\nARCHFIT_TEST_C='single'\nARCHFIT_TEST_EQ=val=with=equals\nmalformed-no-equals\nARCHFIT_TEST_PRESET=from_file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// A real env var must win over the file value.
	t.Setenv("ARCHFIT_TEST_PRESET", "from_env")
	// The rest must be genuinely absent: t.Setenv registers restore-on-cleanup,
	// then Unsetenv makes them truly unset so the test exercises the real
	// "fill when missing" path (not the "" coincidence).
	for _, k := range []string{"ARCHFIT_TEST_A", "ARCHFIT_TEST_B", "ARCHFIT_TEST_C", "ARCHFIT_TEST_EQ"} {
		t.Setenv(k, "sentinel")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}

	loadDotEnv(path)

	cases := map[string]string{
		"ARCHFIT_TEST_A":      "plain",
		"ARCHFIT_TEST_B":      "quoted value",
		"ARCHFIT_TEST_C":      "single",
		"ARCHFIT_TEST_EQ":     "val=with=equals", // value keeps every '=' after the first
		"ARCHFIT_TEST_PRESET": "from_env",        // real env preserved
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	// A line with no '=' must be skipped entirely, not set as a bare key.
	if got, ok := os.LookupEnv("malformed-no-equals"); ok {
		t.Errorf("malformed line set env to %q, want unset", got)
	}
}

// TestLoadDotEnv_EmptyEnvVarWins verifies the LookupEnv fix: a key explicitly
// set to "" in the environment must be treated as "present" and never overwritten
// by the file value. The old os.Getenv(key) != "" check incorrectly treated ""
// as unset and would have clobbered it.
func TestLoadDotEnv_EmptyEnvVarWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("ARCHFIT_TEST_EMPTY=from_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Explicitly set to empty string — must win over the file value.
	t.Setenv("ARCHFIT_TEST_EMPTY", "")

	loadDotEnv(path)

	if got, ok := os.LookupEnv("ARCHFIT_TEST_EMPTY"); !ok || got != "" {
		t.Errorf("ARCHFIT_TEST_EMPTY = (%q, %v), want (\"\", true) — empty env var must not be overwritten", got, ok)
	}
}

func TestLoadDotEnv_MissingFileIsNoop(t *testing.T) {
	t.Parallel()
	// Must not panic or error on an absent file.
	loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env"))
}
