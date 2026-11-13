package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// ObjectStore redirects git object writes to private storage. Use its runner
// for the entire command, including reads and temporary worktrees, and Close
// it only after all consumers of working-tree and snapshot ids have finished.
// The repository's object database is read through an absolute alternate.
type ObjectStore struct {
	toolrun.Runner
	dir     string
	mu      sync.Mutex
	objects map[string]string
}

// NewObjectStore creates command-scoped storage; the caller must call Close.
func NewObjectStore(runner toolrun.Runner) (*ObjectStore, error) {
	dir, err := os.MkdirTemp("", "archfit-objects-*")
	if err != nil {
		return nil, fmt.Errorf("private git objects: %w", err)
	}
	return &ObjectStore{Runner: runner, dir: dir, objects: make(map[string]string)}, nil
}

// Close removes the private objects after the command's readers have finished.
func (s *ObjectStore) Close() error { return os.RemoveAll(s.dir) }

// Run applies private object storage to git, leaving other tools unchanged.
func (s *ObjectStore) Run(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	cmd, err := s.command(ctx, cmd)
	if err != nil {
		return toolrun.Output{}, err
	}
	return s.Runner.Run(ctx, cmd)
}

// Stream applies the same storage to streaming git readers.
func (s *ObjectStore) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	cmd, err := s.command(ctx, cmd)
	if err != nil {
		return toolrun.Output{}, err
	}
	return s.Runner.Stream(ctx, cmd, consume)
}

func (s *ObjectStore) command(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.ToolCmd, error) {
	if cmd.Name != gitBinary {
		return cmd, nil
	}
	dir, err := filepath.Abs(cmd.WorkDir)
	if err != nil {
		return cmd, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	objects, ok := s.objects[dir]
	if !ok {
		out, err := s.Runner.Run(ctx, toolrun.ToolCmd{
			Name: gitBinary, Args: []string{"rev-parse", "--git-path", "objects"},
			WorkDir: dir, Env: CleanEnv(), Timeout: gitTimeout,
		})
		if err != nil {
			return cmd, err
		}
		// Commands such as git --version and git init also run outside a
		// repository. Let git itself handle them, and do not cache absence.
		if out.ExitCode != 0 {
			return cmd, nil
		}
		objects = strings.TrimSpace(string(out.Stdout))
		if !filepath.IsAbs(objects) {
			objects = filepath.Join(dir, objects)
		}
		s.objects[dir] = objects
	}
	// Quote the alternate so a path-list separator in the checkout's path
	// cannot be interpreted as another object directory.
	cmd.Env = append(append([]string(nil), cmd.Env...),
		"GIT_OBJECT_DIRECTORY="+s.dir,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES="+strconv.Quote(objects))
	return cmd, nil
}
