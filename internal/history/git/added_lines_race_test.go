package git

import (
	"errors"
	"testing"
)

func TestCacheRace(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "vanished cache file with --ignore-errors",
			err:  errors.New(`git add --all --ignore-errors exited 1: error: unable to index file '.archfit-cache/last-1.json'`),
			want: true,
		},
		{
			name: "vanished cache file without --ignore-errors",
			err:  errors.New(`git add --all exited 128: fatal: unable to stat '.archfit-cache/last-1.json': No such file or directory`),
			want: true,
		},
		{
			name: "a source file failed, not the cache",
			err:  errors.New(`git add --all exited 128: fatal: unable to stat 'internal/a.go': No such file or directory`),
			want: false,
		},
		{
			name: "a source file failed beside a cache file",
			err: errors.New("git add --all --ignore-errors exited 1: " +
				"error: unable to index file '.archfit-cache/last-1.json'\n" +
				"error: unable to index file 'internal/a.go'"),
			want: false,
		},
		{
			name: "another failure",
			err:  errors.New(`git add --all exited 128: fatal: pathspec 'x' did not match any files`),
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cacheRace(tc.err); got != tc.want {
				t.Errorf("cacheRace(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
