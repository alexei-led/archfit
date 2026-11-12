package policy3

import (
	"bufio"
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// rootKey is the key of the files at the repository root.
const rootKey = "."

// testGlobs are the test paths init declares when a source file matches them.
// A Rust unit test lives inside its file and has no glob.
var testGlobs = []string{
	"**/*_test.go",
	"**/*.{test,spec}.{ts,tsx,mts,cts,js,jsx,mjs,cjs}",
	"**/__tests__/**",
	"**/test_*.py",
	"**/*_test.py",
	"**/conftest.py",
	"**/tests/**",
	"**/test/**",
	"**/testdata/**",
	"**/*.Tests/**",
}

// module is a module init found: a key directory and the files it owns.
type module struct {
	name  string
	key   string
	paths []string
	files []string
}

// tests returns the test globs that match a source file.
func tests(sources []string) []string {
	return slices.DeleteFunc(slices.Clone(testGlobs), func(g string) bool { return !matchesAny(g, sources) })
}

// generated returns the globs of generated sources: the sources the root
// .gitattributes marks linguist-generated, and the Go files whose header says
// "Code generated ... DO NOT EDIT." before the package clause (the go
// command's rule). A directory whose Go files are all generated is one glob.
// Every glob matches a source file.
func generated(root string, sources []string) ([]string, error) {
	attrs, err := readAttributes(root)
	if err != nil {
		return nil, err
	}
	attributes := attributeGlobs(attrs, sources)
	headers, err := goGenerated(root, sources, attributes)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, g := range slices.Concat(attributes, headers) {
		if !slices.Contains(out, g) && matchesAny(g, sources) {
			out = append(out, g)
		}
	}
	slices.Sort(out)
	return out, nil
}

// attribute is a line of .gitattributes that sets (on) or unsets the
// linguist-generated attribute, with its pattern as a glob.
type attribute struct {
	glob string
	on   bool
}

// readAttributes reads the lines of the root .gitattributes that name
// linguist-generated, in file order. A pattern without a slash matches at any
// depth; a quoted pattern is skipped.
func readAttributes(root string) ([]attribute, error) {
	data, err := os.ReadFile(filepath.Join(root, ".gitattributes")) //nolint:gosec // the repository's own attributes file
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var attrs []attribute
	lines := bufio.NewScanner(bytes.NewReader(data))
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], `"`) {
			continue
		}
		if on, named := generatedSetting(fields[1:]); named {
			attrs = append(attrs, attribute{glob: gitGlob(fields[0]), on: on})
		}
	}
	return attrs, lines.Err()
}

// generatedSetting reads the linguist-generated attribute of one line: whether
// the line names it, and whether it sets it. The last setting on the line wins.
func generatedSetting(fields []string) (on, named bool) {
	for _, f := range fields {
		switch f {
		case "linguist-generated", "linguist-generated=true":
			on, named = true, true
		case "-linguist-generated", "!linguist-generated", "linguist-generated=false":
			on, named = false, true
		}
	}
	return on, named
}

// attributeGlobs returns globs of the sources the attributes mark generated,
// read as git reads them: the last line that matches a path decides. A setting
// line that no later line unsets is kept as its glob; every other generated
// source is listed on its own.
func attributeGlobs(attrs []attribute, sources []string) []string {
	var globs []string
	for i, a := range attrs {
		if a.on && !slices.ContainsFunc(attrs[i+1:], func(b attribute) bool { return !b.on }) {
			globs = append(globs, a.glob)
		}
	}
	for _, f := range sources {
		if markedGenerated(attrs, f) && !matchesGlob(globs, f) {
			globs = append(globs, literal(f))
		}
	}
	return globs
}

// markedGenerated reports whether the last attribute line that matches file
// sets linguist-generated.
func markedGenerated(attrs []attribute, file string) bool {
	on := false
	for _, a := range attrs {
		if match(a.glob, file) {
			on = a.on
		}
	}
	return on
}

// gitGlob writes a .gitattributes pattern as a glob of repository paths.
func gitGlob(pattern string) string {
	if rest, anchored := strings.CutPrefix(pattern, "/"); anchored {
		return rest
	}
	if strings.Contains(pattern, "/") {
		return pattern
	}
	return "**/" + pattern
}

// goGenerated returns the Go sources with a generated header that no glob of
// covered matches: a directory whose Go files all have one as dir/*.go, the
// others file by file.
func goGenerated(root string, sources, covered []string) ([]string, error) {
	byDir := map[string][]string{}
	isGen := map[string]bool{}
	for _, file := range sources {
		if path.Ext(file) != ".go" {
			continue
		}
		dir := path.Dir(file)
		byDir[dir] = append(byDir[dir], file)
		gen, err := goHeaderGenerated(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		isGen[file] = gen && !slices.ContainsFunc(covered, func(g string) bool { return match(g, file) })
	}
	var out []string
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		files := byDir[dir]
		if !slices.ContainsFunc(files, func(f string) bool { return !isGen[f] }) {
			out = append(out, join(literal(dir), "*.go"))
			continue
		}
		for _, f := range files {
			if isGen[f] {
				out = append(out, literal(f))
			}
		}
	}
	return out, nil
}

// goHeaderGenerated reports whether a Go file says it is generated. A file
// that does not parse is not: the check facts report it.
func goHeaderGenerated(file string) (bool, error) {
	src, err := os.ReadFile(file) //nolint:gosec // a source file of the repository
	if err != nil {
		return false, err
	}
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return false, nil //nolint:nilerr // a file that does not parse is not generated
	}
	return ast.IsGenerated(f), nil
}

// discover groups files into modules. A file's key is its directory cut to
// two segments, after a leading src/ for Python; then each file goes to the
// longest key that holds it, so no file sits under a module it is not in.
func discover(files []string) []module {
	keys := map[string]bool{}
	for _, f := range files {
		keys[firstKey(f)] = true
	}
	byKey := map[string][]string{}
	for _, f := range files {
		k := longestKey(path.Dir(f), keys)
		byKey[k] = append(byKey[k], f)
	}
	sorted := slices.Sorted(maps.Keys(byKey))
	mods := make([]module, 0, len(sorted))
	for _, k := range sorted {
		mods = append(mods, module{key: k, files: byKey[k], paths: pathsOf(k, byKey[k], sorted)})
	}
	name(mods)
	return mods
}

// firstKey is the directory of file cut to two segments; for Python, after a
// leading src/, so src/shop/orders is a module and not src/shop.
func firstKey(file string) string {
	dir := path.Dir(file)
	prefix := ""
	if ext := path.Ext(file); (ext == ".py" || ext == ".pyi") && strings.HasPrefix(dir, "src/") {
		prefix, dir = "src/", strings.TrimPrefix(dir, "src/")
	}
	parts := strings.SplitN(dir, "/", 3)
	return prefix + strings.Join(parts[:min(len(parts), 2)], "/")
}

// longestKey is the longest key that is dir or holds it.
func longestKey(dir string, keys map[string]bool) string {
	for d := dir; ; d = path.Dir(d) {
		if keys[d] || d == rootKey || d == "/" {
			return d
		}
	}
}

// pathsOf returns the file globs of a module. A key with no key below it owns
// its whole tree. A key with keys below it, and the root, own their own files
// and directories only, so new code next to them is unowned, not theirs.
func pathsOf(key string, files, keys []string) []string {
	below := key == rootKey || slices.ContainsFunc(keys, func(k string) bool { return strings.HasPrefix(k, key+"/") })
	if !below {
		return []string{literal(key) + "/**"}
	}
	var globs []string
	for _, f := range files {
		rel := strings.TrimPrefix(f, key+"/")
		g := join(literal(key), "*"+literal(path.Ext(f)))
		if first, _, deeper := strings.Cut(rel, "/"); deeper {
			g = join(literal(key), literal(first)) + "/**"
		}
		if !slices.Contains(globs, g) {
			globs = append(globs, g)
		}
	}
	slices.Sort(globs)
	return globs
}

// name gives each module a name: its key without a leading internal/ or src/,
// in lower case, with / - and . as _; the root is "root". Names that collide
// take their whole key, then a number; the root keeps "root".
func name(mods []module) {
	count := map[string]int{}
	for i := range mods {
		mods[i].name = shortName(mods[i].key)
		count[mods[i].name]++
	}
	taken := map[string]bool{}
	for i := range mods {
		if count[mods[i].name] > 1 && mods[i].key != rootKey {
			mods[i].name = slug(mods[i].key)
		}
		n := mods[i].name
		for j := 2; taken[n]; j++ {
			n = mods[i].name + "_" + strconv.Itoa(j)
		}
		mods[i].name = n
		taken[n] = true
	}
}

func shortName(key string) string {
	if key == rootKey {
		return "root"
	}
	for _, prefix := range []string{"internal/", "src/"} {
		key = strings.TrimPrefix(key, prefix)
	}
	return slug(key)
}

func slug(key string) string {
	return strings.ToLower(strings.NewReplacer("/", "_", "-", "_", ".", "_").Replace(key))
}

// join joins a key and a relative glob; the root key adds nothing.
func join(key, rel string) string {
	if key == rootKey {
		return rel
	}
	return key + "/" + rel
}

// literal escapes the glob characters of a path, so the glob matches only the
// path: app/[slug] is a directory name, not a character class.
func literal(p string) string {
	var b strings.Builder
	for _, r := range p {
		if strings.ContainsRune(`*?[]{}\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func match(glob, p string) bool {
	ok, _ := doublestar.Match(glob, p)
	return ok
}

func matchesAny(glob string, paths []string) bool {
	return slices.ContainsFunc(paths, func(p string) bool { return match(glob, p) })
}
