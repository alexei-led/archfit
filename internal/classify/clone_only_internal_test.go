package classify

import "testing"

// TestCloneLang locks the extension→language mapping cloneLang derives from
// graph.BuiltinConventions: each language's FileExtensions list claims exactly
// one extension, and an unclaimed (or absent) extension falls back to "" so
// moduleSegments uses its slash default.
func TestCloneLang(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "python extension", file: "services/a/dup.py", want: "python"},
		{name: "go extension", file: "services/a/dup.go", want: "go"},
		{name: "extensionless file", file: "services/a/dup", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cloneLang(tt.file); got != tt.want {
				t.Errorf("cloneLang(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}
