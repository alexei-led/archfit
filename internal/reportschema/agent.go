package reportschema

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/invopop/jsonschema"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/output/agentout"
)

const agentSchemaID = "https://raw.githubusercontent.com/alexei-led/archfit/main/archfit.agent-result.schema.json"

// GenerateAgentResult produces the JSON Schema bytes for agentout.Result, the
// archfit.agent-result.v1 document that `--format agent` emits.
//
// srcDir is the filesystem path to internal/output/agentout so AddGoComments
// can parse doc-comments into schema descriptions; tests pass
// "../output/agentout". The required/optional split follows the state schema:
// a field is required unless it carries `omitempty`.
func GenerateAgentResult(srcDir string) ([]byte, error) {
	r := &jsonschema.Reflector{ExpandedStruct: true}
	if err := r.AddGoComments("github.com/alexei-led/archfit/v3/internal/output/agentout", srcDir); err != nil {
		return nil, fmt.Errorf("reportschema: AddGoComments: %w", err)
	}
	// ResultIdentity and Location live in the report contract.
	if err := r.AddGoComments("github.com/alexei-led/archfit/v3/internal/model/report", filepath.Join(srcDir, "..", "..", "model", "report")); err != nil {
		return nil, fmt.Errorf("reportschema: AddGoComments: %w", err)
	}

	schema := r.Reflect(&agentout.Result{})
	schema.ID = jsonschema.ID(agentSchemaID)
	schema.Version = schemaDraft
	patchAgentDefinitions(schema)

	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("reportschema: marshal: %w", err)
	}
	return append(buf, '\n'), nil
}

// patchAgentDefinitions pins the closed vocabularies of the agent result. Each
// list mirrors the constants in internal/output/agentout and
// internal/model/report.
func patchAgentDefinitions(schema *jsonschema.Schema) {
	if schema.Properties != nil {
		if version, ok := schema.Properties.Get("schema_version"); ok {
			version.Enum = []any{agentout.SchemaVersion}
			version.Description = "The only agent-result contract this binary emits."
		}
		if verdict, ok := schema.Properties.Get("verdict"); ok {
			verdict.Enum = []any{
				string(report.StateHealthy),
				string(report.StateNeedsAttention),
				string(report.StateBlocked),
			}
		}
		if action, ok := schema.Properties.Get("next_action"); ok {
			action.Enum = []any{
				string(agentout.ActionRepair),
				string(agentout.ActionAskOwner),
				string(agentout.ActionRestoreEvidence),
				string(agentout.ActionNone),
			}
		}
	}
	if def, ok := schema.Definitions["ResultIdentity"]; ok && def.Properties != nil {
		pinIdentityVersion(def)
	}
	if change, ok := schema.Definitions["Change"]; ok && change.Properties != nil {
		if status, ok := change.Properties.Get("status"); ok {
			status.Enum = []any{string(agentout.ChangePass), string(agentout.ChangeFail), string(agentout.ChangeUnknown)}
		}
	}
	if attention, ok := schema.Definitions["Attention"]; ok && attention.Properties != nil {
		if kind, ok := attention.Properties.Get("kind"); ok {
			kind.Enum = []any{
				agentout.AttentionUnmeasuredRatchet, agentout.AttentionDeadSelector,
				agentout.AttentionExistingViolation, agentout.AttentionCouplingGate, agentout.AttentionBlockedWithoutFinding,
			}
		}
	}
	if repair, ok := schema.Definitions["Repair"]; ok && repair.Properties != nil {
		if kind, ok := repair.Properties.Get("repair_kind"); ok {
			kind.Enum = []any{"code_change", "needs_owner_decision"}
		}
		if origin, ok := repair.Properties.Get("origin"); ok {
			origin.Enum = originEnum
		}
	}
}
