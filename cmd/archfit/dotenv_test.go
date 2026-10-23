package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDotEnv covers the startup .env autoload: which directories are probed for
// a .env file, how lines are parsed, and the precedence rules against the real
// environment.
//
// One exported test function by design — cmd/archfit sits at its public_api_max
// ceiling, so new coverage arrives as subtests, never as new exported names.
// The parent must NOT be parallel: t.Setenv panics when any ancestor is.
func TestDotEnv(t *testing.T) {
	t.Run("search_dirs", testEnvSearchDirs)
	t.Run("load", testLoadDotEnv)
	t.Run("empty_env_var_wins", testLoadDotEnvEmptyEnvVarWins)
	t.Run("missing_file_is_noop", testLoadDotEnvMissingFileIsNoop)
	t.Run("target_cannot_configure_analyzers", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".env")
		for _, key := range []string{"UV_INDEX_URL", "UV_EXTRA_INDEX_URL", "PYTHONPATH", "LD_PRELOAD", "OPENAI_BASE_URL", "ANTHROPIC_BASE_URL"} {
			t.Setenv(key, "sentinel")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(key+"=target-controlled\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			loadDotEnv(path)
			if _, set := os.LookupEnv(key); set {
				t.Errorf("target .env configured analyzer variable %s", key)
			}
		}
	})
}

func testEnvSearchDirs(t *testing.T) {
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

func testLoadDotEnv(t *testing.T) {
	cases := []struct{ name, key, content, want string }{
		{"anthropic export", anthropicKeyEnv, "# comment\n\nexport ANTHROPIC_API_KEY=plain\n", "plain"},
		{"deepseek export", deepSeekKeyEnv, "export DEEPSEEK_API_KEY=plain\n", "plain"},
		{"double quotes", openAIKeyEnv, "OPENAI_API_KEY=\"quoted value\"\n", "quoted value"},
		{"single quotes", openAIKeyEnv, "OPENAI_API_KEY='single'\n", "single"},
		{"embedded equals", openAIKeyEnv, "OPENAI_API_KEY=val=with=equals\n", "val=with=equals"},
		{"malformed lines", openAIKeyEnv, "malformed-no-equals\n=empty-key\nOPENAI_API_KEY=valid\n", "valid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, "sentinel")
			if err := os.Unsetenv(tc.key); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			loadDotEnv(path)
			if got := os.Getenv(tc.key); got != tc.want {
				t.Fatalf("value=%q want=%q", got, tc.want)
			}
		})
	}
	t.Run("real environment wins", func(t *testing.T) {
		t.Setenv(openAIKeyEnv, "from_env")
		path := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(path, []byte("OPENAI_API_KEY=from_file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		loadDotEnv(path)
		if got := os.Getenv(openAIKeyEnv); got != "from_env" {
			t.Fatalf("real environment was overwritten: %q", got)
		}
	})
}

func testLoadDotEnvEmptyEnvVarWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("OPENAI_API_KEY=from_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Explicitly set to empty string — must win over the file value.
	t.Setenv(openAIKeyEnv, "")

	loadDotEnv(path)

	if got, ok := os.LookupEnv(openAIKeyEnv); !ok || got != "" {
		t.Errorf("OPENAI_API_KEY = (%q, %v), want (\"\", true) — empty env var must not be overwritten", got, ok)
	}
}

func testLoadDotEnvMissingFileIsNoop(t *testing.T) {
	// Must not panic or error on an absent file.
	loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env"))
}
