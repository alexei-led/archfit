package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsRejectVersion2(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{".archfit.yaml": "version: 2\nmodules: {}\n", "go.mod": "module example.com/app\n\ngo 1.22\n", "app.go": "package app\n"})
	for _, command := range []string{"config", "modules", "goparse", "reference"} {
		t.Run(command, func(t *testing.T) {
			var out bytes.Buffer
			err := run(t.Context(), []string{command, "-root", root, "-config", filepath.Join(root, ".archfit.yaml")}, strings.NewReader("[]"), &out)
			if err == nil || !strings.Contains(err.Error(), "archfit init") || out.Len() != 0 {
				t.Fatalf("%s accepted version 2 or omitted migration guidance: error %v, output %q", command, err, out.String())
			}
		})
	}
}

func TestConfigUsesSchema3WithoutFallback(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, body, want string }{
		{"version 1", "version: 1\n", "version"},
		{"unknown key", "version: 3\nowners: [owner]\nmodules: {app: {paths: ['**']}}\nunknown: true\n", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := loadConfig(t.Context(), path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadConfig error = %v, want %s", err, tc.want)
			}
		})
	}
}
