package initcfg

import (
	"path"
	"sort"
	"strings"
)

// discoverDotNet returns one module per .NET project directory, so the default
// policy matches project boundaries. projects are the repo-relative .csproj
// paths the .NET extractor evaluates (Presence.DotNetProjects): discovery reads
// the extractor's answer and runs no tool. A module owns its project directory
// and names its project file as the source, because the graph names a project
// node by that path. Two projects in one directory share one module, named
// after the first project file.
func discoverDotNet(projects []string) []ModuleDef {
	byDir := map[string][]string{}
	for _, p := range projects {
		dir := path.Dir(p)
		byDir[dir] = append(byDir[dir], p)
	}
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	mods := make([]ModuleDef, 0, len(dirs))
	for _, dir := range dirs {
		files := byDir[dir]
		sort.Strings(files)
		mods = append(mods, ModuleDef{
			Name:    strings.TrimSuffix(path.Base(files[0]), path.Ext(files[0])),
			Paths:   []string{path.Join(dir, "**")},
			Sources: files,
		})
	}
	return mods
}
