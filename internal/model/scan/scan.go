// Package scan preserves the legacy scan contract during report migration.
package scan

import "github.com/alexei-led/archfit/internal/model/report"

// SchemaVersion identifies the external report document contract.
const SchemaVersion = report.SchemaVersion

// AgentTask is retained as a compatibility alias.
type AgentTask = report.AgentTask

// AdvisoryTask is retained as a compatibility alias.
type AdvisoryTask = report.AdvisoryTask

// Diagnostic is retained as a compatibility alias for report.Document.
type Diagnostic = report.Document

// New returns an empty report document.
func New() Diagnostic { return report.NewDocument() }
