package brief

// category places a step in the NEXT STEPS order.
type category int

const (
	// categoryTools steps restore analyzer evidence.
	categoryTools category = iota + 1
	// categoryReference steps record or review the gate reference; nextSteps
	// decides them from the gate reference itself.
	categoryReference
	// categoryModules steps are module decisions in the config.
	categoryModules
	// categoryEvidence steps supply coverage or deploy-unit evidence.
	categoryEvidence
	// categoryOther steps close a fact but are not architect next steps
	// (commit history, strength labels); NOT MEASURED still names them.
	categoryOther
	// categoryOutOfClaim facts are outside what the dimension claims to
	// measure; nothing closes them.
	categoryOutOfClaim
)

// OutOfClaim is the NOT MEASURED mark for a fact the dimension does not claim.
const OutOfClaim = "(out of claim — no action)"

// fallbackStep closes a fact this table does not know: a newer engine than
// the renderer, or no collector reached the dimension at all.
const fallbackStep = "report it as an archfit defect: no step is known for this fact"

// Steps several facts share, so NEXT STEPS lists each once.
const (
	stepRestoreDependencies = "restore the dependency evidence: archfit doctor --fix"
	stepSupplyCoverage      = "supply a test coverage report for the current tree in the coverage: section"
)

type factStep struct {
	step     string
	category category
}

// factSteps maps every unknown-fact name the architecture state publishes to
// the step that closes it. The names are the wire vocabulary of
// unknown[].fact; a test pins this table to the assessment's required-fact
// contract, so a new fact cannot ship without a step.
var factSteps = map[string]factStep{
	// intent
	"declared intent inventory": {"declare modules and rules: archfit config init", categoryModules},
	"active rule conformance":   {"restore the analyzers the rules read: archfit doctor --fix", categoryTools},
	"disabled rule conformance": {"", categoryOutOfClaim},
	// structure
	"primary dependency inventory":  {"install or fix the language analyzers: archfit doctor --fix", categoryTools},
	"internal edge classification":  {"declare modules for the imported source: archfit config update", categoryModules},
	"external dependency structure": {"", categoryOutOfClaim},
	// modularity
	"declared module inventory":   {"declare modules: archfit config init", categoryModules},
	"module boundary attribution": {"give every package a module: archfit config update", categoryModules},
	"module graph shape":          {stepRestoreDependencies, categoryTools},
	"inferred public surface":     {"", categoryOutOfClaim},
	// coupling
	"coupling candidate inventory":           {"declare modules that import each other: archfit config update", categoryModules},
	"coupling strength":                      {"pin strength labels (archfit config enrich) or enable analyzers.scip", categoryOther},
	"coupling distance":                      {"declare owner and deploy_unit for each module", categoryModules},
	"extractor resolution within ceiling":    {"fix the unresolved imports the analyzer reports (see Evidence coverage)", categoryTools},
	"local and undeclared-external coupling": {"", categoryOutOfClaim},
	// change_locality
	"eligible commit sample":             {"run on a full checkout with commit history (in CI, fetch-depth: 0)", categoryOther},
	"commit-to-module attribution":       {"run on a full checkout with commit history (in CI, fetch-depth: 0)", categoryOther},
	"essential vs accidental volatility": {"", categoryOutOfClaim},
	// complexity
	"declared module graph":        {"declare modules that import each other: archfit config update", categoryModules},
	"dependency chain depth":       {stepRestoreDependencies, categoryTools},
	"module fan-in distribution":   {stepRestoreDependencies, categoryTools},
	"module fan-out distribution":  {stepRestoreDependencies, categoryTools},
	"code size tail":               {"", categoryOutOfClaim},
	"function length distribution": {"", categoryOutOfClaim},
	"cognitive complexity":         {"", categoryOutOfClaim},
	// testability
	"production source inventory": {"run where the source walk sees production files (check --root)", categoryOther},
	"supplied coverage units":     {stepSupplyCoverage, categoryEvidence},
	"coverage path resolution":    {stepSupplyCoverage, categoryEvidence},
	"coverage module attribution": {stepSupplyCoverage, categoryEvidence},
	"coverage freshness":          {stepSupplyCoverage, categoryEvidence},
	"assertion quality":           {"", categoryOutOfClaim},
	"boundary test semantics":     {"", categoryOutOfClaim},
	// operations
	"declared operational topology":                    {"declare owner and deploy_unit for each module", categoryModules},
	"corroborated deploy unit":                         {"commit a deploy manifest for each deploy_unit", categoryEvidence},
	"owner provenance":                                 {"declare owner for each module or add CODEOWNERS", categoryModules},
	"declared-to-corroborated topology reconciliation": {"make each deploy_unit match its deploy manifest", categoryEvidence},
	"observed runtime topology":                        {"", categoryOutOfClaim},
	"supply-chain inventory":                           {"", categoryOutOfClaim},
	"analyzer health":                                  {"", categoryOutOfClaim},
	// drift
	"admissible persisted reference":   {"once the findings are reviewed, record a gate reference: archfit baseline", categoryReference},
	"complete two-sided seam identity": {"once the findings are reviewed, record a gate reference: archfit baseline", categoryReference},
	"base comparison":                  {"", categoryOutOfClaim},
}

// Step returns the NOT MEASURED suffix for one unknown fact: "→ <step>", or
// the out-of-claim mark.
func Step(fact string) string {
	fs, ok := factSteps[fact]
	switch {
	case !ok:
		return "→ " + fallbackStep
	case fs.category == categoryOutOfClaim:
		return OutOfClaim
	default:
		return "→ " + fs.step
	}
}
