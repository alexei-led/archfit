package golang

import "testing"

// TestGoToolchainToken pins the reduction of `go version` output to the
// toolchain token. The platform half is deliberately dropped: it would put the
// host into the published report and into every byte-identical golden.
func TestGoToolchainToken(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   string
	}{
		{"darwin arm64", "go version go1.27.1 darwin/arm64\n", "go1.27.1"},
		{"linux amd64 is the same toolchain", "go version go1.27.1 linux/amd64\n", "go1.27.1"},
		{"different toolchain is a different identity", "go version go1.26.0 linux/amd64\n", "go1.26.0"},
		{"devel toolchain keeps its version", "go version devel go1.28-abc123 linux/amd64\n", "devel go1.28-abc123"},
		{"unrecognised shape is kept, not dropped", "weird output", "weird output"},
		{"empty probe yields empty identity", "", ""},
		{"whitespace only yields empty identity", "  \n\t", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := goToolchainToken(tc.stdout); got != tc.want {
				t.Errorf("goToolchainToken(%q) = %q, want %q", tc.stdout, got, tc.want)
			}
		})
	}
}
