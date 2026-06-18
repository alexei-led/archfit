package initcfg

import (
	"fmt"
	"time"
)

// SubdomainPin is one approved subdomain assignment for a module.
type SubdomainPin struct {
	Module     string
	Subdomain  string // core|supporting|generic
	Volatility string // low|medium|high (may be empty)
	ReviewedAt time.Time
	ReviewedBy string
}

// PinSubdomains applies approved subdomain pins to the .archfit.yaml source bytes.
// currentSubdomains maps module name → current subdomain value (empty string if unset).
// Only modules whose subdomain field is absent (or whose current value already matches
// the pin — idempotent) are updated.
// Returns (patched bytes, count of modules actually patched, error).
func PinSubdomains(src []byte, currentSubdomains map[string]string, pins []SubdomainPin) ([]byte, int, error) {
	var edits []Edit
	patched := 0

	for _, pin := range pins {
		if pin.Subdomain == "" {
			continue
		}
		cur := currentSubdomains[pin.Module]
		if cur != "" && cur != pin.Subdomain {
			// Already set to a different value — skip to avoid overwriting human edits.
			continue
		}
		if cur == pin.Subdomain {
			// Already set to the same value — idempotent skip, don't count as patched.
			continue
		}

		fields := map[ModuleField]string{
			FieldSubdomain:  pin.Subdomain,
			FieldReviewedAt: pin.ReviewedAt.UTC().Format(time.RFC3339),
			FieldReviewedBy: pin.ReviewedBy,
		}
		if pin.Volatility != "" {
			fields[FieldVolatility] = pin.Volatility
		}

		edits = append(edits, SetModuleFieldsEdit{
			Module: pin.Module,
			Fields: fields,
		})
		patched++
	}

	if len(edits) == 0 {
		return src, 0, nil
	}

	result, err := ApplyEdits(src, edits)
	if err != nil {
		return nil, 0, fmt.Errorf("initcfg: pin subdomains: %w", err)
	}
	return result, patched, nil
}
