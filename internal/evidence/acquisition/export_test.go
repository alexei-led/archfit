package acquisition

// Test-only aliases for the acquisition internals. They are unexported in
// production because Acquire is their only caller; the behavior tests still
// address them directly so a coverage-gap or disclosure rule can be pinned
// without driving a whole run.
var (
	BuildCoverageGaps            = buildCoverageGaps
	MarkDisabledPrimaries        = markDisabledPrimaries
	ConfigToolGate               = configToolGate
	BuildConfigWarnings          = buildConfigWarnings
	BuildJudgmentDecisionTasks   = buildJudgmentDecisionTasks
	OutputInsideRootWarning      = outputInsideRootWarning
	OwnerDegradationWarning      = ownerDegradationWarning
	TSUnresolvedWarning          = tsUnresolvedWarning
	PyUnresolvedWarning          = pyUnresolvedWarning
	BuildVolatilityCorroboration = buildVolatilityCorroboration
	ConfigHash                   = configHash
)
