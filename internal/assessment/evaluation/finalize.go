package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
)

// FinalizeInput carries the resolved values needed to score and build repair
// tasks. Assessment neither reads configuration nor touches the filesystem.
type FinalizeInput struct {
	RuleTypes          map[string]string
	ModulePublic       map[string][]string
	ValidationCommands []string
	KnownFiles         map[string]struct{}
	CrateRootDirs      map[string]string
	ModuleRootDirs     map[string]string
	OnDisk             func(string) bool
}

// Finalized is the assessment finalization outcome.
type Finalized struct {
	Score score.Scorecard
}

// finalize synthesises the diagnostic scorecard and attaches repair tasks.
// Scores and seam measurements never rewrite the verdict.
func finalize(diag *result.Result, in FinalizeInput) Finalized {
	card := score.Synthesize(*diag)
	resolver := agenttask.NewPathResolver(in.KnownFiles, in.CrateRootDirs, in.ModuleRootDirs, in.OnDisk)
	diag.AgentTasks = agenttask.Build(diag.Findings, in.RuleTypes, in.ModulePublic, in.ValidationCommands, diag.SyntaxFacts, resolver)
	diag.AdvisoryTasks = agenttask.BuildAdvisoryTasks(diag.Findings, in.ValidationCommands)
	return Finalized{Score: card}
}
