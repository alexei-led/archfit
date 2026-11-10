// gofacts emits the file-level import facts the extractor equivalence test
// compares (Archfit 4 design, sections 11.8 and 15.2). It is a measurement
// prototype, not product code.
//
// Usage:
//
//	gofacts reference -lang go|typescript|python -root DIR [-config FILE]
//	gofacts goparse -root DIR [-config FILE]
//	gofacts modules -lang LANG [-config FILE] < node-paths.json
//	gofacts config -lang LANG [-config FILE]
//
// reference runs today's extractor for one language. goparse is the new Go
// extractor: go/parser imports, go/build constraints, go.mod module paths, no
// Go toolchain. modules maps node paths, a JSON array on stdin, to declared
// modules with the engine's own module map. config prints the extractor configuration
// the engine projects for one language, so the Python prototypes read the same
// exclusions and package settings. Each command writes JSON to stdout.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

// facts is the one output shape both sides of the comparison write. Node
// paths are spelled as the engine spells them: a file path and a package dir
// for Go, file paths for TypeScript, dotted modules for Python.
type facts struct {
	Language   string       `json:"language"`
	Side       string       `json:"side"`
	Sources    []string     `json:"sources"`
	Edges      []edge       `json:"edges"`
	Unknown    []unresolved `json:"unknown"`
	External   int          `json:"external"`
	Coverage   *coverage    `json:"coverage,omitempty"`
	ConfigNote string       `json:"config_note,omitempty"`
}

// edge is one first-party import: a source node uses a target node.
type edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// unresolved is a first-party import the side could not place on a node.
// Go spells the specifier as the package dir the import names, as the
// engine spells package nodes; TypeScript and Python as written.
type unresolved struct {
	From      string `json:"from"`
	Specifier string `json:"specifier"`
}

// coverage is today's extractor's own account of the run.
type coverage struct {
	Tool       string `json:"tool"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Unresolved int    `json:"unresolved"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gofacts:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("want a command: reference, goparse, or modules")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	lang := fs.String("lang", config.LangGo, "language: go, typescript, or python")
	root := fs.String("root", "", "repository root")
	cfgPath := fs.String("config", "", "the repository's .archfit.yaml (optional)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, note, err := loadConfig(ctx, *cfgPath)
	if err != nil {
		return err
	}
	var out any
	switch args[0] {
	case "reference":
		absRoot, rootErr := filepath.Abs(*root)
		if rootErr != nil {
			return rootErr
		}
		f, refErr := reference(ctx, *lang, absRoot, cfg)
		f.ConfigNote = note
		out, err = f, refErr
	case "goparse":
		absRoot, rootErr := filepath.Abs(*root)
		if rootErr != nil {
			return rootErr
		}
		f, parseErr := goparse(absRoot, cfg)
		f.ConfigNote = note
		out, err = f, parseErr
	case "modules":
		out, err = modules(cfg.ModuleMapView(), *lang, stdin)
	case "config":
		out = mergedExclusions(cfg).ForExtract(*lang)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", " ")
	return enc.Encode(out)
}

// loadConfig reads the repository's config as the engine does. A config in an
// older schema, which today's engine refuses, is read for its exclusions and
// module paths only, and the note says so. No path means no config.
func loadConfig(ctx context.Context, path string) (config.Config, string, error) {
	if path == "" {
		return config.Config{}, "no config: no module map", nil
	}
	cfg, err := config.Load(ctx, path)
	if err == nil {
		return cfg, "", nil
	}
	data, readErr := os.ReadFile(path) // #nosec G304 G703 -- the operator names the config
	if readErr != nil {
		return config.Config{}, "", readErr
	}
	var subset struct {
		Exclude []string                    `yaml:"exclude"`
		Modules map[string]policy.ModuleDef `yaml:"modules"`
	}
	if yamlErr := yaml.Unmarshal(data, &subset); yamlErr != nil {
		return config.Config{}, "", fmt.Errorf("%w; reading exclude and modules only: %w", err, yamlErr)
	}
	return config.Config{Exclude: subset.Exclude, Modules: subset.Modules},
		"engine refuses the config (" + err.Error() + "); read exclude and modules only", nil
}

// mergedExclusions is the exclusion set every extractor of a run sees.
func mergedExclusions(cfg config.Config) config.Config {
	cfg.Exclude = scope.MergeExclusions(cfg.Exclude)
	return cfg
}

// modules maps each node path in the JSON array read from in to its declared
// module, "" for none. JSON keeps "", the Go root package, a node.
func modules(mm policy.ModuleMap, lang string, in io.Reader) (map[string]string, error) {
	var paths []string
	if err := json.NewDecoder(in).Decode(&paths); err != nil {
		return nil, fmt.Errorf("modules: want a JSON array of node paths: %w", err)
	}
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		mod, _ := mm.ModuleForNode(p, lang)
		out[p] = mod
	}
	return out, nil
}

// finish sorts and de-duplicates the lists so two runs compare byte for byte.
func (f *facts) finish() {
	slices.Sort(f.Sources)
	f.Sources = slices.Compact(f.Sources)
	slices.SortFunc(f.Edges, func(a, b edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	f.Edges = slices.Compact(f.Edges)
	slices.SortFunc(f.Unknown, func(a, b unresolved) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.Specifier, b.Specifier))
	})
	f.Unknown = slices.Compact(f.Unknown)
	if f.Sources == nil {
		f.Sources = []string{}
	}
	if f.Edges == nil {
		f.Edges = []edge{}
	}
	if f.Unknown == nil {
		f.Unknown = []unresolved{}
	}
}
