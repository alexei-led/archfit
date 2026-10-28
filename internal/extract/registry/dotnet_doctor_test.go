package registry

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestDotnetFailure(t *testing.T) {
	tests := []struct {
		name string
		out  toolrun.Output
		want string
	}{
		{
			name: "global.json under the checked directory is named relative to it",
			out:  toolrun.Output{Stderr: []byte("A compatible .NET SDK was not found.\n\nRequested SDK version: 10.0.999\nglobal.json file: /repo/global.json\n"), ExitCode: 155},
			want: "global.json requests SDK 10.0.999, which is not installed; install a matching SDK or relax the pin",
		},
		{
			name: "global.json above the checked directory keeps its path",
			out:  toolrun.Output{Stderr: []byte("Requested SDK version: 10.0.999\nglobal.json file: /global.json\n"), ExitCode: 155},
			want: "/global.json requests SDK 10.0.999, which is not installed; install a matching SDK or relax the pin",
		},
		{
			name: "helper error line",
			out:  toolrun.Output{Stderr: []byte("archfit-roslyn: MSBuild could not be located\n"), ExitCode: 1},
			want: "MSBuild could not be located",
		},
		{
			name: "anything else is its first line",
			out:  toolrun.Output{Stderr: []byte("\nboom\nmore\n"), ExitCode: 2},
			want: "boom",
		},
		{
			name: "no output",
			out:  toolrun.Output{ExitCode: 2},
			want: "exit 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dotnetFailure(tt.out, "/repo"); got != tt.want {
				t.Errorf("dotnetFailure = %q, want %q", got, tt.want)
			}
		})
	}
}
