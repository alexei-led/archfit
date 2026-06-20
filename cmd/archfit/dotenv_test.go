package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# a comment\n\nexport ARCHFIT_TEST_A=plain\nARCHFIT_TEST_B=\"quoted value\"\nARCHFIT_TEST_C='single'\nmalformed-no-equals\nARCHFIT_TEST_PRESET=from_file\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// A real env var must win over the file value.
	t.Setenv("ARCHFIT_TEST_PRESET", "from_env")
	// Ensure the others start unset.
	for _, k := range []string{"ARCHFIT_TEST_A", "ARCHFIT_TEST_B", "ARCHFIT_TEST_C"} {
		t.Setenv(k, "") // registers cleanup; loadDotEnv treats "" as unset and fills it
	}

	loadDotEnv(path)

	cases := map[string]string{
		"ARCHFIT_TEST_A":      "plain",
		"ARCHFIT_TEST_B":      "quoted value",
		"ARCHFIT_TEST_C":      "single",
		"ARCHFIT_TEST_PRESET": "from_env", // real env preserved
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadDotEnv_MissingFileIsNoop(t *testing.T) {
	// Must not panic or error on an absent file.
	loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env"))
}
