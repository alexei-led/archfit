// Package checkfacts reads the facts the check uses (design 7.2, 11.2, 11.8):
// the imports, source files and literals of the base and of the head, and the
// unknown items, what the check needs and cannot read. It observes and never
// judges.
//
// Both trees take one path. A tree is named by its git tree id: the merge base
// by its commit's tree, the head by the tree a commit of every change in the
// working tree would hold (git.WorkingTreeID: untracked files in, ignored files
// out). Facts read from a tree are cached under that id, so a tree read once is
// never read again. On a miss the tree's files are written to a temporary
// directory from git objects and read there, so the head sees exactly the
// files the base and CI see, and the working tree is never touched.
//
// The extractors run behind internal/extract/registry: Go with the standard
// parser; TypeScript, JavaScript and Python with ast-grep and built-in
// resolvers; Rust and .NET with cargo and MSBuild. No optional analyzer runs,
// and never dependency-cruiser, grimp, the typed Go loader, SCIP or jscpd.
package checkfacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// factsFormat is part of every cache key. Change it whenever what a tree's
// facts hold changes without an engine version change.
const factsFormat = "checkfacts.v2"

// fixFetch is the fix of a base that cannot be read.
const fixFetch = "run git fetch, then archfit check again"

// versionTimeout bounds one tool --version probe.
const versionTimeout = 30 * time.Second

// Acquirer reads the check facts of a repository.
type Acquirer struct {
	Runner toolrun.Runner
	// Store caches the facts of each tree. Nil turns the cache off.
	Store *factcache.Store
	// Version is the engine version, part of every cache key. Empty turns the
	// cache off: facts read by one build are never served to another.
	Version string
	Config  Config

	keyOnce     sync.Once
	keyTools    string // the tool part of every key
	keySettings string // the settings part of every key; "" turns the cache off
	// versions holds the version toolVersion found for each tool.
	versions map[string]string
}

// Head returns the facts of the working tree of the repository at root, as a
// commit of every change would hold it. An error is a working tree git cannot
// stage, or a cancelled context.
func (a *Acquirer) Head(ctx context.Context, root string) (evidence.CheckFacts, error) {
	tree, err := git.WorkingTreeID(ctx, root, a.Runner)
	if err != nil {
		return evidence.CheckFacts{}, fmt.Errorf("checkfacts: the working tree: %w", err)
	}
	return a.tree(ctx, root, tree)
}

// Base returns the facts of the tree of commit, the merge base the check
// compares with. A base that cannot be read gives one unknown item of cause
// base, not an error: missing evidence is not zero. Only a cancelled context
// is an error.
func (a *Acquirer) Base(ctx context.Context, root, commit string) (evidence.CheckFacts, error) {
	unreadable := func(err error) evidence.CheckFacts {
		return evidence.CheckFacts{Unknown: []evidence.UnknownItem{{
			Specifier: commit, Reason: "the base " + commit + " cannot be read: " + err.Error(),
			Cause: evidence.CauseBase, Fix: fixFetch,
		}}}
	}
	tree, err := git.TreeOf(ctx, root, commit, a.Runner)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return evidence.CheckFacts{}, ctxErr
		}
		return unreadable(err), nil
	}
	f, err := a.tree(ctx, root, tree)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return evidence.CheckFacts{}, ctxErr
		}
		return unreadable(err), nil
	}
	return f, nil
}

// tree returns the facts of the git tree id tree: from the cache, else read
// from a snapshot of its files and stored.
func (a *Acquirer) tree(ctx context.Context, root, tree string) (evidence.CheckFacts, error) {
	key := a.key(ctx, tree)
	if f, ok := a.cached(ctx, key); ok {
		return f, nil
	}
	dir, err := os.MkdirTemp("", "archfit-check-tree-*")
	if err != nil {
		return evidence.CheckFacts{}, fmt.Errorf("checkfacts: snapshot directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := git.SnapshotTree(ctx, root, tree, dir, a.Runner); err != nil {
		return evidence.CheckFacts{}, fmt.Errorf("checkfacts: snapshot of %s: %w", tree, err)
	}
	f, err := registry.ReadCheck(ctx, a.Runner, dir, registry.CheckRequest{
		Configs: a.Config.Languages, Exclude: a.Config.Exclude, Literals: a.Config.Literals,
	})
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	withoutSnapshotPath(&f, dir)
	if key != "" && cacheable(f) {
		entry := cacheEntry{Tools: a.projectTools(ctx, dir), Facts: f}
		if data, err := json.Marshal(entry); err == nil {
			a.Store.Put(factcache.CheckFactsAnalyzer, key, data)
		}
	}
	return f, nil
}

// cacheEntry is a tree's facts with the version of each project tool that read
// them: cargo and dotnet run only where their language has a project, so their
// versions are checked against the tree the facts came from, not keyed from
// the working tree.
type cacheEntry struct {
	Tools map[string]string   `json:"tools,omitempty"`
	Facts evidence.CheckFacts `json:"facts"`
}

// cached returns the facts stored under key when every project tool that read
// them still has the same version.
func (a *Acquirer) cached(ctx context.Context, key string) (evidence.CheckFacts, bool) {
	if key == "" {
		return evidence.CheckFacts{}, false
	}
	data, ok := a.Store.Get(factcache.CheckFactsAnalyzer, key)
	if !ok {
		return evidence.CheckFacts{}, false
	}
	var entry cacheEntry
	if json.Unmarshal(data, &entry) != nil {
		return evidence.CheckFacts{}, false
	}
	for tool, version := range entry.Tools {
		if a.toolVersion(ctx, tool) != version {
			return evidence.CheckFacts{}, false
		}
	}
	return entry.Facts, true
}

// cacheable reports whether facts may be stored: a tool that is missing or
// failed, and a base that cannot be read, can be fixed without changing the
// tree, so facts that hold such an item are read again on the next run.
func cacheable(f evidence.CheckFacts) bool {
	for _, u := range f.Unknown {
		if u.Cause == evidence.CauseTool || u.Cause == evidence.CauseBase {
			return false
		}
	}
	return true
}

// projectTools returns the version of cargo and dotnet for each of their
// languages that has a project in the snapshot dir.
func (a *Acquirer) projectTools(ctx context.Context, dir string) map[string]string {
	tools := map[string]string{}
	for _, probe := range []struct{ lang, tool string }{{"rust", registry.ToolCargo}, {"dotnet", "dotnet"}} {
		cfg, ok := a.Config.Languages[probe.lang]
		if ok && cfg.Mode != evidenceports.ModeOff && registry.ProjectPresent(probe.lang, dir, cfg) {
			tools[probe.tool] = a.toolVersion(ctx, probe.tool)
		}
	}
	return tools
}

// withoutSnapshotPath writes the snapshot directory as "." in the unknown
// reasons, so a tool that prints absolute paths gives the same reason on every
// run and names no directory that is already gone.
func withoutSnapshotPath(f *evidence.CheckFacts, dir string) {
	paths := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
		paths = append([]string{resolved}, paths...)
	}
	for i := range f.Unknown {
		for _, p := range paths {
			f.Unknown[i].Reason = strings.ReplaceAll(f.Unknown[i].Reason, p, ".")
		}
	}
}

// key returns the cache key of tree, or "" when the cache is off.
func (a *Acquirer) key(ctx context.Context, tree string) string {
	if a.Store == nil || a.Version == "" {
		return ""
	}
	a.keyOnce.Do(func() { a.keyTools, a.keySettings = a.keyParts(ctx) })
	if a.keySettings == "" {
		return ""
	}
	return factcache.TreeFactsKey(a.keyTools, a.keySettings, tree)
}

// keyParts returns the tool part and the settings part of the cache key. The
// tools: the engine version, the facts format, and the version of ast-grep,
// which any tree with TypeScript, Python or non-Go literals needs. The
// settings: a hash of the Config and of the Go build context the parser reads;
// "" when it cannot be hashed.
func (a *Acquirer) keyParts(ctx context.Context) (tools, settings string) {
	parts := []string{a.Version, factsFormat, "ast-grep=" + a.toolVersion(ctx, registry.ToolAstGrep)}
	settings, err := factcache.HashJSON(struct {
		Config Config
		GoEnv  map[string]string
	}{a.Config, goBuildContext()})
	if err != nil {
		return "", ""
	}
	return strings.Join(parts, "\n"), settings
}

// toolVersion returns the last line `tool --version` prints, or "" when the
// tool is not on PATH or fails. Each tool is probed once per Acquirer.
func (a *Acquirer) toolVersion(ctx context.Context, tool string) string {
	if v, ok := a.versions[tool]; ok {
		return v
	}
	v := ""
	if _, ok := a.Runner.Detect(ctx, tool); ok {
		out, err := a.Runner.Run(ctx, toolrun.ToolCmd{Name: tool, Args: []string{"--version"}, Timeout: versionTimeout})
		if err == nil && out.ExitCode == 0 {
			lines := strings.Split(strings.TrimSpace(string(out.Stdout)), "\n")
			v = strings.TrimSpace(lines[len(lines)-1])
		}
	}
	if a.versions == nil {
		a.versions = map[string]string{}
	}
	a.versions[tool] = v
	return v
}

// goBuildContext returns what the Go parser reads to pick the files the go
// command builds: the environment variables, and a hash of the go env file
// that holds their defaults.
func goBuildContext() map[string]string {
	env := map[string]string{}
	for _, key := range []string{"GOOS", "GOARCH", "GOFLAGS", "CGO_ENABLED", "GOENV"} {
		if v, ok := os.LookupEnv(key); ok {
			env[key] = v
		}
	}
	if file := goEnvFile(); file != "" {
		if data, err := os.ReadFile(file); err == nil { //nolint:gosec // the go env file the go command reads
			sum := sha256.Sum256(data)
			env["goenv-file"] = hex.EncodeToString(sum[:])
		}
	}
	return env
}

// goEnvFile is the path of the go env file, as the go command finds it.
func goEnvFile() string {
	file := os.Getenv("GOENV")
	if file == "off" {
		return ""
	}
	if file != "" {
		return file
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "go", "env")
}
