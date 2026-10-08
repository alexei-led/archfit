package main

import "testing"

func TestReleaseVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tagged go install", "v3.0.0", "v3.0.0"},
		{"pseudo version", "v3.0.1-0.20261008120000-abcdef123456", "v3.0.1-0.20261008120000-abcdef123456"},
		{"local build", "(devel)", devVersion},
		{"no build info", "", devVersion},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseVersion(tc.in); got != tc.want {
				t.Errorf("releaseVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
