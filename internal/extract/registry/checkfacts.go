package registry

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/dotnet"
	"github.com/alexei-led/archfit/v3/internal/extract/golang"
	"github.com/alexei-led/archfit/v3/internal/extract/literals"
	"github.com/alexei-led/archfit/v3/internal/extract/pyimports"
	"github.com/alexei-led/archfit/v3/internal/extract/rust"
	"github.com/alexei-led/archfit/v3/internal/extract/tsimports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// ToolAstGrep is the parser the check needs for TypeScript, JavaScript and
// Python imports, and for the literals of every language but Go.
const ToolAstGrep = "ast-grep"

// The fix of each unknown item the check facts give: the step that restores
// the evidence, as the record prints it after "next:".
const (
	fixImport  = "fix the import"
	fixSyntax  = "fix the syntax error"
	fixCgo     = "set CGO_ENABLED to 0 or 1, then run archfit check again"
	fixExtends = "make the extends a relative path inside the repository"
	fixAstGrep = "install ast-grep: " + analyzerSetupURL
	fixCargo   = "install Rust: https://rustup.rs"
	fixDotNet  = "install the .NET SDK: " + dotnetSDKURL
	fixRetry   = "fix the error, then run archfit check again"
)

// scriptExts are the extensions of the TypeScript and JavaScript files
// tsimports reads.
var scriptExts = []string{".js", ".cjs", ".mjs", ".jsx", ".ts", ".cts", ".mts", ".tsx"}

// goFileExt is the extension of a Go source file.
const goFileExt = ".go"

// builtinExclusions are the directories no extractor of the check reads, at
// any depth, besides the policy's exclude: version control, installed
// packages and vendored code.
var builtinExclusions = []string{".git", "node_modules", "vendor"}

// CheckExclusions returns the one exclusion rule of the check as globs: the
// policy's exclude, then the built-in directories at any depth.
func CheckExclusions(exclude []string) []string {
	out := slices.Clone(exclude)
	for _, dir := range builtinExclusions {
		out = append(out, "**/"+dir, "**/"+dir+"/**")
	}
	return out
}

// CheckRequest says what ReadCheck reads.
type CheckRequest struct {
	// Configs are the settings of each language by id. A language whose Mode is
	// off is not read; auto and on read it where its files exist.
	Configs Configs
	// Exclude is the policy's exclude.
	Exclude []string
	// Literals turns on the literals, which only the duplication rule reads.
	Literals bool
}

// ReadCheck reads the facts the check uses from the tree at root, through each
// language's CheckFacts and then the literals. It runs no optional analyzer.
// The exclusion rule of CheckExclusions applies to every extractor and once
// more to the merged facts. What a language cannot read is an unknown item, so
// only a cancelled context is an error. The facts are sorted and unique.
func ReadCheck(ctx context.Context, runner toolrun.Runner, root string, req CheckRequest) (evidence.CheckFacts, error) {
	exclusions := CheckExclusions(req.Exclude)
	var all evidence.CheckFacts
	var present []string
	for _, lang := range languages {
		cfg, ok := req.Configs[lang.ID]
		if !ok || cfg.Mode == evidenceports.ModeOff || lang.CheckFacts == nil {
			continue
		}
		cfg.Exclusions = exclusions
		f, err := lang.CheckFacts(ctx, runner, root, cfg)
		if err := ctx.Err(); err != nil {
			return evidence.CheckFacts{}, err
		}
		if err != nil {
			f = evidence.CheckFacts{Unknown: []evidence.UnknownItem{{
				Reason: lang.ID + " facts not read: " + err.Error(), Cause: evidence.CauseTool, Fix: fixRetry,
			}}}
		}
		f = withoutExcluded(f, exclusions)
		if len(f.Sources) > 0 || len(f.Unknown) > 0 {
			present = append(present, lang.ID)
		}
		merge(&all, f)
	}
	if req.Literals && len(present) > 0 {
		merge(&all, readLiterals(ctx, runner, root, exclusions, present))
		if err := ctx.Err(); err != nil {
			return evidence.CheckFacts{}, err
		}
	}
	return tidy(all, exclusions), nil
}

func merge(into *evidence.CheckFacts, f evidence.CheckFacts) {
	into.Imports = append(into.Imports, f.Imports...)
	into.Literals = append(into.Literals, f.Literals...)
	into.Unknown = append(into.Unknown, f.Unknown...)
	into.Sources = append(into.Sources, f.Sources...)
}

// readLiterals reads the literals of the present languages. Without ast-grep
// it reads the Go literals and gives one unknown item for the other present
// languages. A file that does not parse is an unknown item unless the import
// extractor already named the whole file.
func readLiterals(ctx context.Context, runner toolrun.Runner, root string, exclusions, present []string) evidence.CheckFacts {
	langs := present
	var out evidence.CheckFacts
	onlyGo := !slices.ContainsFunc(present, func(id string) bool { return id != graph.LangGo })
	if !onlyGo && !detected(ctx, runner, ToolAstGrep) {
		langs = nil
		var missed []string
		for _, id := range present {
			if id == graph.LangGo {
				langs = append(langs, id)
			} else {
				missed = append(missed, id)
			}
		}
		if len(missed) > 0 {
			out.Unknown = append(out.Unknown, evidence.UnknownItem{
				Reason: "ast-grep is not installed; it reads the literals of " + strings.Join(missed, ", "),
				Cause:  evidence.CauseTool, Fix: fixAstGrep,
			})
		}
		if len(langs) == 0 {
			return out
		}
	}
	facts, err := literals.Extract(ctx, runner, root, literals.Config{Exclude: exclusions, Languages: langs})
	if err != nil {
		out.Unknown = append(out.Unknown, evidence.UnknownItem{
			Reason: "literals not read: " + err.Error(), Cause: evidence.CauseTool, Fix: fixRetry,
		})
		return out
	}
	for _, l := range facts.Literals {
		out.Literals = append(out.Literals, evidence.LiteralOccurrence{File: l.File, Line: l.Line, Value: l.Value, Shape: shapeOf(l)})
	}
	for _, u := range facts.Unread {
		if goNeverBuilds(u.File) {
			continue
		}
		out.Unknown = append(out.Unknown, evidence.UnknownItem{File: u.File, Reason: u.Reason, Cause: evidence.CauseImport, Fix: fixSyntax})
	}
	return out
}

// goNeverBuilds reports whether the go command never builds the Go file: it is
// under testdata, or a directory or file name starts with "_" or ".". Such a
// file may be broken on purpose (a parser's fixture), so a file of it that does
// not parse is no unknown; its literals are read when it parses.
func goNeverBuilds(file string) bool {
	if path.Ext(file) != goFileExt {
		return false
	}
	for _, segment := range strings.Split(file, "/") {
		if segment == "testdata" || strings.HasPrefix(segment, "_") || strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

func detected(ctx context.Context, runner toolrun.Runner, tool string) bool {
	_, ok := runner.Detect(ctx, tool)
	return ok
}

func shapeOf(l literals.Literal) evidence.LiteralShape {
	switch l.Shape() {
	case literals.ShapeTag:
		return evidence.ShapeTag
	case literals.ShapeWord:
		return evidence.ShapeWord
	default:
		return evidence.ShapeOther
	}
}

// withoutExcluded drops the facts of the files the exclusions match. An item
// about the whole tree has no file and stays.
func withoutExcluded(f evidence.CheckFacts, exclusions []string) evidence.CheckFacts {
	out := func(file string) bool { return excludedPath(exclusions, file) }
	f.Sources = slices.DeleteFunc(f.Sources, out)
	f.Imports = slices.DeleteFunc(f.Imports, func(i evidence.ImportFact) bool { return out(i.File) })
	f.Literals = slices.DeleteFunc(f.Literals, func(l evidence.LiteralOccurrence) bool { return out(l.File) })
	f.Unknown = slices.DeleteFunc(f.Unknown, func(u evidence.UnknownItem) bool { return u.File != "" && out(u.File) })
	return f
}

// tidy drops the facts of excluded files, sorts every list, removes
// duplicates, and keeps one whole-file "does not parse" item per file.
func tidy(f evidence.CheckFacts, exclusions []string) evidence.CheckFacts {
	f = withoutExcluded(f, exclusions)
	slices.Sort(f.Sources)
	f.Sources = slices.Compact(f.Sources)
	slices.SortFunc(f.Imports, func(a, b evidence.ImportFact) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Target, b.Target), compareBool(a.External, b.External))
	})
	f.Imports = slices.Compact(f.Imports)
	slices.SortFunc(f.Literals, func(a, b evidence.LiteralOccurrence) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Value, b.Value))
	})
	f.Literals = slices.Compact(f.Literals)
	slices.SortStableFunc(f.Unknown, func(a, b evidence.UnknownItem) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Specifier, b.Specifier))
	})
	f.Unknown = slices.CompactFunc(f.Unknown, func(a, b evidence.UnknownItem) bool {
		if a.File != "" && a.File == b.File && a.Line == 0 && b.Line == 0 && a.Specifier == "" && b.Specifier == "" &&
			doesNotParse(a) && doesNotParse(b) {
			return true
		}
		return a == b
	})
	return f
}

func doesNotParse(u evidence.UnknownItem) bool { return u.Fix == fixSyntax }

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// excludedPath reports whether the exclusion globs match the repository
// relative file or one of its directories.
func excludedPath(globs []string, file string) bool {
	for p := file; p != "." && p != "/" && p != ""; p = path.Dir(p) {
		for _, g := range globs {
			if ok, _ := doublestar.Match(g, p); ok {
				return true
			}
		}
	}
	return false
}

// goCheckFacts reads Go through the standard parser: the files the go command
// reads, with their imports. A first-party import targets its package
// directory, "." for the root package.
func goCheckFacts(_ context.Context, _ toolrun.Runner, root string, cfg evidenceports.ExtractConfig) (evidence.CheckFacts, error) {
	parsed, err := golang.ParseImports(root, cfg)
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	f := evidence.CheckFacts{Sources: parsed.Sources}
	for _, imp := range parsed.Imports {
		switch imp.Kind {
		case golang.ImportFirstParty:
			f.Imports = append(f.Imports, evidence.ImportFact{File: imp.File, Line: imp.Line, Target: dirOrDot(imp.Package)})
		case golang.ImportExternal:
			f.Imports = append(f.Imports, evidence.ImportFact{File: imp.File, Line: imp.Line, Target: imp.Path, External: true})
		default:
			f.Unknown = append(f.Unknown, evidence.UnknownItem{
				File: imp.File, Line: imp.Line, Specifier: imp.Path,
				Reason: "no Go package is read in " + dirOrDot(imp.Package), Cause: evidence.CauseImport, Fix: fixImport,
			})
		}
	}
	for _, u := range parsed.Unknown {
		fix := fixSyntax
		if strings.Contains(u.Reason, "CGO_ENABLED") {
			fix = fixCgo
		}
		f.Unknown = append(f.Unknown, evidence.UnknownItem{File: u.File, Reason: u.Reason, Cause: evidence.CauseImport, Fix: fix})
	}
	return f, nil
}

func dirOrDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// tsCheckFacts reads TypeScript and JavaScript through ast-grep and the
// built-in resolver. tsimports takes no exclusions: a tree whose script files
// are all excluded is not read at all, so it needs no ast-grep, and ReadCheck
// drops the facts of excluded files after it.
func tsCheckFacts(ctx context.Context, runner toolrun.Runner, root string, cfg evidenceports.ExtractConfig) (evidence.CheckFacts, error) {
	kept, err := walkSources(root, scriptExts, cfg.Exclusions)
	if err != nil || len(kept) == 0 {
		return evidence.CheckFacts{}, err
	}
	facts, err := tsimports.Extract(ctx, runner, root)
	if errors.Is(err, tsimports.ErrNoAstGrep) {
		return noAstGrep("TypeScript and JavaScript imports"), nil
	}
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	f := evidence.CheckFacts{Sources: facts.Sources}
	for _, imp := range facts.Imports {
		switch imp.Kind {
		case tsimports.KindFile:
			f.Imports = append(f.Imports, evidence.ImportFact{File: imp.File, Line: imp.Line, Target: imp.Target})
		case tsimports.KindExternal:
			f.Imports = append(f.Imports, evidence.ImportFact{File: imp.File, Line: imp.Line, Target: imp.Specifier, External: true})
		default:
			f.Unknown = append(f.Unknown, evidence.UnknownItem{
				File: imp.File, Line: imp.Line, Specifier: imp.Specifier,
				Reason: "the import names no file", Cause: evidence.CauseImport, Fix: fixImport,
			})
		}
	}
	for _, u := range facts.Unknown {
		fix := fixExtends
		if u.Specifier == "" {
			fix = fixSyntax
		}
		f.Unknown = append(f.Unknown, evidence.UnknownItem{File: u.File, Specifier: u.Specifier, Reason: u.Reason, Cause: evidence.CauseImport, Fix: fix})
	}
	return f, nil
}

// pyCheckFacts reads Python through ast-grep and the built-in resolver. A tree
// with Python files but no package gives no facts: loose scripts are not a
// Python project.
func pyCheckFacts(ctx context.Context, runner toolrun.Runner, root string, cfg evidenceports.ExtractConfig) (evidence.CheckFacts, error) {
	facts, err := pyimports.Extract(ctx, runner, root, pyimports.Config{Package: cfg.PyPackage, Exclude: cfg.Exclusions})
	if errors.Is(err, pyimports.ErrNoAstGrep) {
		return noAstGrep("Python imports"), nil
	}
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	f := evidence.CheckFacts{Sources: facts.Sources}
	for _, imp := range facts.Imports {
		switch imp.Kind {
		case pyimports.KindFile:
			f.Imports = append(f.Imports, evidence.ImportFact{File: imp.File, Line: imp.Line, Target: imp.Target})
		case pyimports.KindExternal:
			f.Imports = append(f.Imports, evidence.ImportFact{File: imp.File, Line: imp.Line, Target: imp.Module, External: true})
		default:
			f.Unknown = append(f.Unknown, evidence.UnknownItem{
				File: imp.File, Line: imp.Line, Specifier: imp.Module,
				Reason: "the import names no module file", Cause: evidence.CauseImport, Fix: fixImport,
			})
		}
	}
	return f, nil
}

func noAstGrep(what string) evidence.CheckFacts {
	return evidence.CheckFacts{Unknown: []evidence.UnknownItem{{
		Reason: "ast-grep is not installed; it reads " + what, Cause: evidence.CauseTool, Fix: fixAstGrep,
	}}}
}

// rustCheckFacts reads the crate dependencies of Cargo.toml with cargo
// metadata, without cargo-modules. A dependency on a workspace crate targets
// the crate's directory; the site is its line in Cargo.toml.
func rustCheckFacts(ctx context.Context, runner toolrun.Runner, root string, cfg evidenceports.ExtractConfig) (evidence.CheckFacts, error) {
	if !rust.Applicable(root, cfg.CargoManifest) {
		return evidence.CheckFacts{}, nil
	}
	sources, err := walkSources(root, []string{".rs"}, cfg.Exclusions)
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	if _, ok := runner.Detect(ctx, ToolCargo); !ok {
		return toolMissing(sources, "cargo is not installed; it reads the Rust crates", fixCargo), nil
	}
	cfg.Mode, cfg.ModuleGraph = evidenceports.ModeAuto, false
	facts, cov, err := rust.New(runner, cfg).Extract(ctx, scope.Scope{Root: root})
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	f := evidence.CheckFacts{Sources: sources}
	if cov.Status != evidence.StatusOK {
		f.Unknown = append(f.Unknown, evidence.UnknownItem{Reason: "Rust crates not read: " + cov.Reason, Cause: evidence.CauseTool, Fix: fixRetry})
		return f, nil
	}
	dirs := map[string]string{} // crate name to its directory
	for _, cr := range facts.CrateRoots {
		dirs[cr.Name] = dirOrDot(cr.Dir)
	}
	f.Imports = edgeImports(facts.Edges, func(id string) (string, bool) {
		name := graph.NodePath(id)
		dir, ok := dirs[name]
		if !ok {
			return name, false
		}
		return dir, true
	}, func(id string) string {
		return path.Join(dirs[graph.NodePath(id)], "Cargo.toml")
	})
	return f, nil
}

// dotnetCheckFacts reads the project references of the .NET projects with
// MSBuild evaluation, without the Roslyn type graph. A reference targets the
// referenced project's directory; the site is its line in the project file.
func dotnetCheckFacts(ctx context.Context, runner toolrun.Runner, root string, cfg evidenceports.ExtractConfig) (evidence.CheckFacts, error) {
	if !dotnet.Applicable(root, cfg.Exclusions, cfg.DotNetProjects) {
		return evidence.CheckFacts{}, nil
	}
	sources, err := walkSources(root, []string{".cs"}, cfg.Exclusions)
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	if _, ok := runner.Detect(ctx, cmdDotNet); !ok {
		return toolMissing(sources, "dotnet is not installed; it reads the .NET project references", fixDotNet), nil
	}
	cfg.Mode, cfg.TypeGraphDisabled = evidenceports.ModeAuto, true
	ex := dotnet.New(runner, cfg, cfg.DotNetProjects)
	ex.TypeGraph = dotnet.TypeGraphOptions{Disabled: true}
	facts, cov, err := ex.Extract(ctx, scope.Scope{Root: root})
	if err != nil {
		return evidence.CheckFacts{}, err
	}
	f := evidence.CheckFacts{Sources: sources}
	if cov.Status != evidence.StatusOK {
		f.Unknown = append(f.Unknown, evidence.UnknownItem{Reason: ".NET projects not read: " + cov.Reason, Cause: evidence.CauseTool, Fix: fixRetry})
	}
	f.Imports = edgeImports(facts.Edges, func(id string) (string, bool) {
		return dirOrDot(path.Dir(graph.NodePath(id))), true
	}, graph.NodePath)
	return f, nil
}

func toolMissing(sources []string, reason, fix string) evidence.CheckFacts {
	return evidence.CheckFacts{Sources: sources, Unknown: []evidence.UnknownItem{{Reason: reason, Cause: evidence.CauseTool, Fix: fix}}}
}

// edgeImports turns depends_on edges into imports: target places an edge's
// To node (its directory, or the library when not first-party), and file names
// the From node's manifest for an edge without a location.
func edgeImports(edges []graph.Edge, target func(id string) (string, bool), file func(id string) string) []evidence.ImportFact {
	var out []evidence.ImportFact
	for _, e := range edges {
		if e.Kind != graph.EdgeKindDependsOn {
			continue
		}
		to, firstParty := target(e.To)
		locations := e.Locations
		if len(locations) == 0 {
			locations = []graph.Location{{File: file(e.From)}}
		}
		for _, loc := range locations {
			out = append(out, evidence.ImportFact{File: loc.File, Line: loc.Line, Target: to, External: !firstParty})
		}
	}
	return out
}

// walkSources lists the repository-relative files under root with one of the
// extensions exts, outside the exclusions, sorted.
func walkSources(root string, exts, exclusions []string) ([]string, error) {
	var files []string
	err := fs.WalkDir(os.DirFS(root), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", name, err)
		}
		if name == "." {
			return nil
		}
		if excludedPath(exclusions, name) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && slices.Contains(exts, path.Ext(name)) {
			files = append(files, name)
		}
		return nil
	})
	return files, err
}
