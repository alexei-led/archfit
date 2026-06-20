package initcfg

import (
	"fmt"
	"time"
)

// ValuePin is one approved single-scalar assignment (owner or volatility) for a
// module, stamped with the human reviewer identity.
type ValuePin struct {
	Module     string
	Value      string
	ReviewedAt time.Time
	ReviewedBy string
}

// PinModuleValues applies approved pins for one module field to the .archfit.yaml
// source bytes. field selects which scalar to write (FieldOwner or
// FieldVolatility). current maps module name → current value for that field
// (empty string if unset). Only modules whose field is absent are updated — a
// field already set to a different value is left alone (never overwrite human
// edits); a field already set to the same value is an idempotent no-op.
// Returns (patched bytes, count of modules actually patched, error).
func PinModuleValues(src []byte, field ModuleField, current map[string]string, pins []ValuePin) ([]byte, int, error) {
	var edits []Edit
	patched := 0

	for _, pin := range pins {
		if pin.Value == "" {
			continue
		}
		cur := current[pin.Module]
		if cur != "" {
			// Already set (same or different) — never overwrite a live field.
			continue
		}

		edits = append(edits, SetModuleFieldsEdit{
			Module: pin.Module,
			Fields: map[ModuleField]string{
				field:           pin.Value,
				FieldReviewedAt: pin.ReviewedAt.UTC().Format(time.RFC3339),
				FieldReviewedBy: pin.ReviewedBy,
			},
		})
		patched++
	}

	if len(edits) == 0 {
		return src, 0, nil
	}

	result, err := ApplyEdits(src, edits)
	if err != nil {
		return nil, 0, fmt.Errorf("initcfg: pin module values: %w", err)
	}
	return result, patched, nil
}
