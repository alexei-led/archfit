package initcfg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// EvidenceKind identifies the source class for an architecture evidence item.
type EvidenceKind string

const (
	// EvidenceKindDoc marks README, architecture, design, and ADR documents.
	EvidenceKindDoc EvidenceKind = "doc"
	// EvidenceKindAPI marks exported names and configured public API globs.
	EvidenceKindAPI EvidenceKind = "api"
	// EvidenceKindComment marks package-level source comments.
	EvidenceKindComment EvidenceKind = "comment"
	// EvidenceKindConfig marks bounded .archfit.yaml snippets.
	EvidenceKindConfig EvidenceKind = "config"
	// EvidenceKindDiagnostic marks deterministic tool and command summaries.
	EvidenceKindDiagnostic EvidenceKind = "diag"
)

// EvidenceItem is one bounded, citeable architecture-intent fact for LLM prompts.
type EvidenceItem struct {
	ID     string
	Kind   EvidenceKind
	Source string
	Text   string
}

// EvidenceDiagnostic carries deterministic command facts into the semantic pack.
type EvidenceDiagnostic struct {
	Source  string
	Summary string
}

// EvidenceBudget bounds how much evidence each source type can contribute.
type EvidenceBudget struct {
	Docs         int
	Comments     int
	APIs         int
	Configs      int
	Diagnostics  int
	MaxTextBytes int
}

// EvidencePackOptions controls architecture evidence collection.
type EvidencePackOptions struct {
	Root        string
	Modules     []ModuleDef
	ConfigPath  string
	Config      []byte
	Diagnostics []EvidenceDiagnostic
	Budget      EvidenceBudget
}

const (
	defaultEvidenceDocs        = 12
	defaultEvidenceComments    = 12
	defaultEvidenceAPIs        = 20
	defaultEvidenceConfigs     = 2
	defaultEvidenceDiagnostics = 20
	defaultEvidenceTextBytes   = 700
	maxEvidenceReadBytes       = 64 * 1024
)

// BuildArchitectureEvidencePack returns a deterministic, bounded evidence pack
// for off-gate LLM prompts. It is best-effort: unreadable sources are skipped.
func BuildArchitectureEvidencePack(opts EvidencePackOptions) []EvidenceItem {
	root := opts.Root
	if root == "" {
		root = "."
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	budget := normalizeEvidenceBudget(opts.Budget)

	docs := boundEvidenceItems(collectDocEvidence(root), budget.Docs, budget.MaxTextBytes)
	comments, apis := collectCodeEvidence(root, opts.Modules)
	comments = boundEvidenceItems(comments, budget.Comments, budget.MaxTextBytes)
	apis = boundEvidenceItems(apis, budget.APIs, budget.MaxTextBytes)
	configs := boundEvidenceItems(collectConfigEvidence(root, opts.ConfigPath, opts.Config), budget.Configs, budget.MaxTextBytes)
	diagnostics := boundEvidenceItems(collectDiagnosticEvidence(opts.Diagnostics), budget.Diagnostics, budget.MaxTextBytes)

	items := make([]EvidenceItem, 0, len(docs)+len(comments)+len(apis)+len(configs)+len(diagnostics))
	items = append(items, docs...)
	items = append(items, comments...)
	items = append(items, apis...)
	items = append(items, configs...)
	items = append(items, diagnostics...)
	return items
}

// FormatEvidencePack renders items as compact prompt lines with stable IDs.
func FormatEvidencePack(items []EvidenceItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		out = append(out, item.ID+" ("+string(item.Kind)+") "+item.Source+": "+item.Text)
	}
	return out
}

func normalizeEvidenceBudget(b EvidenceBudget) EvidenceBudget {
	if b.Docs <= 0 {
		b.Docs = defaultEvidenceDocs
	}
	if b.Comments <= 0 {
		b.Comments = defaultEvidenceComments
	}
	if b.APIs <= 0 {
		b.APIs = defaultEvidenceAPIs
	}
	if b.Configs <= 0 {
		b.Configs = defaultEvidenceConfigs
	}
	if b.Diagnostics <= 0 {
		b.Diagnostics = defaultEvidenceDiagnostics
	}
	if b.MaxTextBytes <= 0 {
		b.MaxTextBytes = defaultEvidenceTextBytes
	}
	return b
}

func boundEvidenceItems(items []EvidenceItem, limit, maxTextBytes int) []EvidenceItem {
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		return items[i].Source < items[j].Source
	})
	if len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		items[i].Text = boundEvidenceText(items[i].Text, maxTextBytes)
	}
	return items
}

func boundEvidenceText(s string, maxBytes int) string {
	s = oneLine(s)
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 3 {
		return s[:maxBytes]
	}
	return strings.TrimSpace(s[:maxBytes]) + "..."
}

func collectDocEvidence(root string) []EvidenceItem {
	candidates := map[string]struct{}{}
	for _, rel := range []string{"README.md", "README.markdown", "README.rst", "README.txt", "README"} {
		if regularFile(filepath.Join(root, rel)) && !secretishEvidencePath(rel) {
			candidates[filepath.ToSlash(rel)] = struct{}{}
		}
	}

	docsRoot := filepath.Join(root, "docs")
	_ = filepath.WalkDir(docsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if p != docsRoot && skipEvidenceDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !architectureDocRel(rel) || secretishEvidencePath(rel) {
			return nil
		}
		candidates[rel] = struct{}{}
		return nil
	})

	rels := make([]string, 0, len(candidates))
	for rel := range candidates {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	items := make([]EvidenceItem, 0, len(rels))
	for _, rel := range rels {
		data, err := readEvidenceFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		text := summarizeFreeText(string(data))
		if text == "" {
			continue
		}
		items = append(items, EvidenceItem{ID: "doc:" + rel, Kind: EvidenceKindDoc, Source: rel, Text: text})
	}
	return items
}

func architectureDocRel(rel string) bool {
	lower := strings.ToLower(filepath.ToSlash(rel))
	if !evidenceTextExt(path.Ext(lower)) {
		return false
	}
	for _, prefix := range []string{"docs/design/", "docs/architecture/", "docs/adr/", "docs/adrs/", "docs/decisions/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	base := path.Base(lower)
	return strings.Contains(base, "adr") || strings.Contains(base, "architecture") || strings.Contains(base, "design")
}

func evidenceTextExt(ext string) bool {
	switch ext {
	case ".md", ".markdown", ".mdown", ".txt", ".rst", ".adoc":
		return true
	default:
		return false
	}
}

func collectCodeEvidence(root string, modules []ModuleDef) ([]EvidenceItem, []EvidenceItem) {
	commentsByID := map[string]EvidenceItem{}
	apiByModule := map[string]map[string]struct{}{}
	publicByModule := map[string][]string{}

	mods := append([]ModuleDef(nil), modules...)
	sort.SliceStable(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
	for _, mod := range mods {
		if len(mod.Public) > 0 {
			publicByModule[mod.Name] = append([]string(nil), mod.Public...)
			sort.Strings(publicByModule[mod.Name])
		}
		for _, dir := range moduleEvidenceDirs(root, mod.Paths) {
			files, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
			for _, entry := range files {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				filePath := filepath.Join(dir, entry.Name())
				rel := relEvidencePath(root, filePath)
				if secretishEvidencePath(rel) {
					continue
				}
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
				if err != nil {
					continue
				}
				if file.Doc != nil {
					text := summarizeFreeText(file.Doc.Text())
					if text != "" {
						commentsByID["comment:"+rel] = EvidenceItem{ID: "comment:" + rel, Kind: EvidenceKindComment, Source: rel, Text: text}
					}
				}
				for _, name := range exportedNames(file) {
					if apiByModule[mod.Name] == nil {
						apiByModule[mod.Name] = map[string]struct{}{}
					}
					apiByModule[mod.Name][name] = struct{}{}
				}
			}
		}
	}

	comments := make([]EvidenceItem, 0, len(commentsByID))
	for _, item := range commentsByID {
		comments = append(comments, item)
	}
	apis := make([]EvidenceItem, 0, len(apiByModule)+len(publicByModule))
	moduleNames := make(map[string]struct{}, len(apiByModule)+len(publicByModule))
	for module := range apiByModule {
		moduleNames[module] = struct{}{}
	}
	for module := range publicByModule {
		moduleNames[module] = struct{}{}
	}
	for module := range moduleNames {
		var parts []string
		if public := publicByModule[module]; len(public) > 0 {
			parts = append(parts, "public globs: "+strings.Join(public, ", "))
		}
		names := sortedKeys(apiByModule[module])
		if len(names) > 0 {
			parts = append(parts, "exported names: "+strings.Join(names, ", "))
		}
		text := strings.Join(parts, "; ")
		if text == "" {
			continue
		}
		apis = append(apis, EvidenceItem{ID: "api:" + slugID(module), Kind: EvidenceKindAPI, Source: module, Text: text})
	}
	return comments, apis
}

func moduleEvidenceDirs(root string, paths []string) []string {
	seen := map[string]struct{}{}
	for _, p := range paths {
		dir, ok := resolveDir(root, p)
		if !ok || !regularDir(dir) {
			continue
		}
		seen[dir] = struct{}{}
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

func exportedNames(file *ast.File) []string {
	seen := map[string]struct{}{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && ast.IsExported(d.Name.Name) {
				seen["func "+d.Name.Name] = struct{}{}
			}
		case *ast.GenDecl:
			kind := strings.ToLower(d.Tok.String())
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(s.Name.Name) {
						seen[kind+" "+s.Name.Name] = struct{}{}
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if ast.IsExported(name.Name) {
							seen[kind+" "+name.Name] = struct{}{}
						}
					}
				}
			}
		}
	}
	return sortedKeys(seen)
}

func collectConfigEvidence(root, configPath string, configData []byte) []EvidenceItem {
	source := ".archfit.yaml"
	if configPath != "" {
		source = relEvidencePath(root, configPath)
	}
	if len(configData) == 0 {
		pathToRead := configPath
		if pathToRead == "" {
			pathToRead = filepath.Join(root, ".archfit.yaml")
		}
		if secretishEvidencePath(relEvidencePath(root, pathToRead)) {
			return nil
		}
		data, err := readEvidenceFile(pathToRead)
		if err != nil {
			return nil
		}
		configData = data
	}
	text := summarizeConfig(string(configData))
	if text == "" {
		return nil
	}
	return []EvidenceItem{{ID: "config:" + source, Kind: EvidenceKindConfig, Source: source, Text: text}}
}

func collectDiagnosticEvidence(diags []EvidenceDiagnostic) []EvidenceItem {
	if len(diags) == 0 {
		return nil
	}
	counts := map[string]int{}
	items := make([]EvidenceItem, 0, len(diags))
	for _, diag := range diags {
		summary := summarizeFreeText(diag.Summary)
		if summary == "" {
			continue
		}
		source := strings.TrimSpace(diag.Source)
		if source == "" {
			source = "diagnostic"
		}
		slug := slugID(source)
		counts[slug]++
		items = append(items, EvidenceItem{
			ID:     "diag:" + slug + "#" + strconvItoa(counts[slug]),
			Kind:   EvidenceKindDiagnostic,
			Source: source,
			Text:   summary,
		})
	}
	return items
}

func summarizeFreeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var parts []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || line == "" || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, "[![") {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line == "" {
			continue
		}
		parts = append(parts, line)
		if len(parts) >= 24 {
			break
		}
	}
	return oneLine(strings.Join(parts, " "))
}

func summarizeConfig(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if secretishConfigLine(trimmed) {
			line = redactConfigLine(line)
		}
		lines = append(lines, strings.TrimSpace(line))
		if len(lines) >= 80 {
			break
		}
	}
	return oneLine(strings.Join(lines, " | "))
}

func redactConfigLine(line string) string {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "[redacted]"
	}
	return line[:idx+1] + " [redacted]"
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func readEvidenceFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- caller restricts to local repo evidence files
	if err != nil {
		return nil, err
	}
	if len(data) > maxEvidenceReadBytes {
		data = data[:maxEvidenceReadBytes]
	}
	return data, nil
}

func relEvidencePath(root, p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return filepath.ToSlash(filepath.Clean(p))
	}
	return filepath.ToSlash(rel)
}

func regularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

func regularDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func skipEvidenceDir(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case ".git", ".hg", ".svn", ".archfit-cache", ".gitnexus", ".codegraph", ".pi", "node_modules", "vendor", "target", "dist", "build", "coverage":
		return true
	default:
		return strings.HasPrefix(lower, ".") || secretishName(lower)
	}
}

func secretishEvidencePath(rel string) bool {
	lower := strings.ToLower(filepath.ToSlash(rel))
	for _, part := range strings.Split(lower, "/") {
		if part == ".env" || strings.HasPrefix(part, ".env.") || secretishName(part) {
			return true
		}
	}
	switch path.Ext(lower) {
	case ".pem", ".key", ".p12", ".pfx":
		return true
	default:
		return false
	}
}

func secretishName(name string) bool {
	for _, needle := range []string{"secret", "credential", "password", "token", "private-key", "private_key", "apikey", "api_key"} {
		if strings.Contains(name, needle) {
			return true
		}
	}
	return false
}

func secretishConfigLine(line string) bool {
	lower := strings.ToLower(line)
	return secretishName(lower) || strings.Contains(lower, "authorization:") || strings.Contains(lower, "bearer ")
}

func slugID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "evidence"
	}
	return out
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
