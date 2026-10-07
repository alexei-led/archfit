package main

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/alexei-led/archfit/internal/output/archmap"
)

// MapCmd renders the architecture map of one run: the declared modules in
// layer order and the observed seams with their policy status and strength.
// It reads the same report check produces and adds no wire document.
type MapCmd struct {
	Config string   `short:"c" help:"Path to config file." default:".archfit.yaml"`
	Root   string   `help:"Repository root to analyze (default: directory of --config)." type:"path"`
	Format string   `name:"format" help:"Output format: mermaid or text." enum:"mermaid,text" default:"mermaid"`
	Focus  []string `name:"focus" help:"Keep this module, its direct neighbours, and the seams that touch it. Repeatable. The output counts what it omits."`

	Refresh  bool   `name:"refresh" help:"Re-run all extractors and refresh the cache."`
	Progress string `name:"progress" help:"Progress reporting on stderr: auto, plain, none." enum:"auto,plain,none," default:""`
	Quiet    bool   `short:"q" help:"Suppress progress output."`
}

func (*MapCmd) Help() string {
	return `Draw the modules and the seams between them, from the same run check makes.

Layers are drawn outermost first, so a permitted dependency points down. Each
seam shows its policy status and its integration strength:
  violation  an active gate finding names the pair (thick arrow)
  accepted   a gate finding on the pair is baselined or waived (dotted arrow)
  advisory   another active finding names the pair
  allowed    a fail-gated allowlist or layer rule permits the pair
  observed   no finding names the pair and no rule decides it

The exit code is 0, or 3 on an error or an unknown --focus module. The verdict
never changes it: use check for the gate.

Common runs:
  archfit map -c .archfit.yaml > architecture.mmd
  archfit map --format text --focus billing`
}

func (c *MapCmd) Run(deps *appDeps) error {
	rep := newProgressReporter(deps.stderr(), analyzePhaseTotal(false, false), c.Progress, c.Quiet, time.Now())
	rep.banner("Archfit mapping " + analyzeTarget(c.Config, c.Root))
	resp, cfg, err := executeScan(context.Background(), deps, scanRequest{
		configPath: c.Config, root: c.Root, refresh: c.Refresh, progress: c.Progress, quiet: c.Quiet, reportOnly: true,
	}, rep.advance)
	rep.finish()
	if err != nil {
		return err
	}
	modules := make([]archmap.Module, 0, len(cfg.Modules))
	for name, def := range cfg.Modules {
		modules = append(modules, archmap.Module{Name: name, Layer: def.Layer})
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Name < modules[j].Name })
	err = archmap.Render(archmap.Input{
		Layers: cfg.Layers, Modules: modules, Seams: resp.Document.State.Seams, Focus: c.Focus,
	}, c.Format, deps.Stdout)
	if errors.Is(err, archmap.ErrUnknownFocus) {
		return &exitError{code: 3, msg: "error: " + err.Error()}
	}
	return err
}
