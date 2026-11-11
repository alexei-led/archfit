// Package literals reads the string literals of a tree, with their line
// numbers, for the identical-literal check (design 7.3, rule R12). Go files are
// read with the standard parser. TypeScript, JavaScript, Python, Rust and C#
// files are read with embedded ast-grep rules, run only through toolrun.Runner.
//
// A literal's value is the text between its delimiters, with any prefix
// dropped and escapes kept as written: "abc" and `abc` are the same value.
// Not literals: Go import paths and struct tags; TypeScript and JavaScript
// import and export sources and the specifiers of require() and import();
// Python docstrings (any bare string statement). Text in comments is never
// read. Strings that are not plain text are skipped: template strings with
// ${…}, Python f-, t- and b-strings, Rust byte and C strings, C# interpolated and
// "…"u8 strings.
//
// Not read in milestone 1: multi-line literals (a raw or triple-quoted string
// that spans lines) and tables of values as one unit. A single-line string
// inside a table is a literal like any other.
//
// The index of a tree keeps values of at least MinIndexLength runes. The
// other R12 conditions (single words and struct tags, declared tests and
// generated paths, held by fewer than 3 modules) belong to the duplication
// rule. Shape ports shape() of scripts/eval/copies/copy_noise.py for the two
// shapes R12 reads.
package literals

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// MinIndexLength is the shortest value, in runes, the index keeps: R12's
// length bound.
const MinIndexLength = 12

// Literal is one single-line string literal of a file.
type Literal struct {
	File  string // root-relative, slash-separated
	Line  int    // 1-based
	Value string // the text between the delimiters, escapes as written
}

// Length is the number of runes of the value, as Python's len counts them in
// the copy-noise measurement.
func (l Literal) Length() int { return utf8.RuneCountInString(l.Value) }

// Shape classifies a value for rule R12.
type Shape string

const (
	// ShapeTag is struct tag syntax: key:"value" pairs.
	ShapeTag Shape = "tag"
	// ShapeWord is one token of letters, digits, '_', '.', ':', '@' and '-'.
	ShapeWord Shape = "word"
	// ShapeOther is any other value.
	ShapeOther Shape = "other"
)

// The prototype's GO_TAG and WORD patterns. Python's \w is Unicode, so \w is
// written [\p{L}\p{N}_].
var (
	tagPattern  = regexp.MustCompile(`^([\p{L}\p{N}_]+:"[^"]*"\s*)+$`)
	wordPattern = regexp.MustCompile(`^[\p{L}\p{N}_.:@-]+$`)
)

// Shape returns the shape of the value.
func (l Literal) Shape() Shape {
	switch {
	case tagPattern.MatchString(l.Value):
		return ShapeTag
	case wordPattern.MatchString(l.Value):
		return ShapeWord
	default:
		return ShapeOther
	}
}

// Index maps each value of at least MinIndexLength runes to its occurrences.
type Index map[string][]Literal

// BuildIndex returns the index of the literals of a tree. Occurrences keep
// their input order; Extract returns them sorted by file and line.
func BuildIndex(lits []Literal) Index {
	idx := Index{}
	for _, l := range lits {
		if l.Length() >= MinIndexLength {
			idx[l.Value] = append(idx[l.Value], l)
		}
	}
	return idx
}

// Config holds the policy settings the extractor reads.
type Config struct {
	Exclude []string // globs of paths no extractor reads
}

// ErrNoAstGrep reports that the tree has TypeScript, JavaScript, Python, Rust
// or C# files but ast-grep is not installed, so their literals are not read.
var ErrNoAstGrep = errors.New("literals: ast-grep is not installed; it reads TypeScript, JavaScript, Python, Rust and C# literals")

// skippedDirs hold no source of the tree: version control, installed
// packages and vendored code.
var skippedDirs = []string{".git", "node_modules", "vendor"}

const goExt = ".go"

// astGrepExts are the extensions of the files ast-grep reads: the script
// files tsimports parses, then Python, Rust and C#.
var astGrepExts = []string{".js", ".cjs", ".mjs", ".jsx", ".ts", ".cts", ".mts", ".tsx", ".py", ".rs", ".cs"}

// Extract reads the literals of every Go, TypeScript, JavaScript, Python,
// Rust and C# file under root that cfg does not exclude, sorted by file, line
// and value. A tree with only Go files needs no ast-grep.
func Extract(ctx context.Context, runner toolrun.Runner, root string, cfg Config) ([]Literal, error) {
	for _, pattern := range cfg.Exclude {
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("literals: bad exclude glob %q", pattern)
		}
	}
	fsys := os.DirFS(root)
	goFiles, sgFiles, err := sourceFiles(fsys, cfg.Exclude)
	if err != nil {
		return nil, err
	}
	var lits []Literal
	for _, file := range goFiles {
		found, err := readGo(fsys, file)
		if err != nil {
			return nil, err
		}
		lits = append(lits, found...)
	}
	if len(sgFiles) > 0 {
		if _, ok := runner.Detect(ctx, astGrep); !ok {
			return nil, ErrNoAstGrep
		}
		found, err := readAstGrep(ctx, runner, root, sgFiles)
		if err != nil {
			return nil, err
		}
		lits = append(lits, found...)
	}
	slices.SortFunc(lits, func(a, b Literal) int {
		return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Value, b.Value))
	})
	return lits, nil
}

// sourceFiles lists the root-relative Go files and the files for ast-grep,
// sorted. Only regular files count: a symlink is not followed out of the tree.
func sourceFiles(fsys fs.FS, exclude []string) (goFiles, sgFiles []string, err error) {
	err = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("literals: walk %s: %w", name, err)
		}
		if name == "." {
			return nil
		}
		if excluded(exclude, name) || d.IsDir() && slices.Contains(skippedDirs, d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		switch ext := path.Ext(name); {
		case ext == goExt:
			goFiles = append(goFiles, name)
		case slices.Contains(astGrepExts, ext):
			sgFiles = append(sgFiles, name)
		}
		return nil
	})
	return goFiles, sgFiles, err
}

// excluded reports whether an exclusion glob matches the path. The walk
// skips an excluded directory, so a file under it is never asked.
func excluded(exclude []string, name string) bool {
	for _, pattern := range exclude {
		if ok, _ := doublestar.Match(pattern, name); ok {
			return true
		}
	}
	return false
}
