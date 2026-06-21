package initcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tsGlobSuffix is appended to a discovered subdirectory to form its paths glob.
const tsGlobSuffix = "/**"

// DiscoverTS reads src/ or lib/ subdirectories if package.json is present at root.
// Returns discovered module definitions, one per subdirectory.
func DiscoverTS(root string) ([]ModuleDef, error) {
	if !fileExists(filepath.Join(root, "package.json")) {
		return nil, nil
	}
	return discoverSubdirs(root, []string{"src", "lib"})
}

// discoverSubdirs reads subdirectories within dirNames inside root.
// Returns one ModuleDef per subdirectory found.
func discoverSubdirs(root string, dirNames []string) ([]ModuleDef, error) {
	var mods []ModuleDef
	for _, dir := range dirNames {
		full := filepath.Join(root, dir)
		entries, err := os.ReadDir(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("initcfg: read dir %s: %w", full, err)
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			name := e.Name()
			path := dir + "/" + name + tsGlobSuffix
			mods = append(mods, ModuleDef{
				Name:  name,
				Paths: []string{path},
				// TS/JS cross-file imports go through module exports (you cannot import
				// a non-exported binding), so they are contract coupling. Mark the
				// module's files public; SCIP-typescript can refine this when enabled.
				Public: []string{path},
				Layer:  layerCore,
			})
		}
	}
	return mods, nil
}
