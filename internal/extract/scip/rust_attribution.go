package scip

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// rustAttribution is mandatory on mapped Rust runs, including empty output.
// A pointer count and non-nil diagnostics distinguish omission from explicit zero.
type rustAttribution struct {
	Status            string           `json:"status"`
	UnresolvedSymbols *int             `json:"unresolved_symbols"`
	Diagnostics       []rustDiagnostic `json:"diagnostics"`
}

type rustDiagnostic struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// rustReaderCoverage validates the reader contract before facts can be consumed
// or cached. Both cold output and durable cache hits pass through this check.
func rustReaderCoverage(raw []byte) (evidence.Coverage, error) {
	var out readerOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return evidence.Coverage{}, fmt.Errorf("rust attribution: invalid reader JSON: %w", err)
	}
	if out.Error != "" {
		return evidence.Coverage{}, fmt.Errorf("rust attribution: reader failed: %s", out.Error)
	}
	a := out.RustAttribution
	if a == nil || a.UnresolvedSymbols == nil || a.Diagnostics == nil {
		return evidence.Coverage{}, errors.New("rust attribution: missing required status/count/diagnostics")
	}
	count := *a.UnresolvedSymbols
	if a.Status == evidence.StatusOK && count == 0 && len(a.Diagnostics) == 0 {
		return evidence.Coverage{Status: evidence.StatusOK}, nil
	}
	if a.Status != evidence.StatusPartial || count <= 0 || len(a.Diagnostics) == 0 {
		return evidence.Coverage{}, errors.New("rust attribution: inconsistent status/count/diagnostics")
	}
	total, previous := 0, ""
	details := make([]string, 0, len(a.Diagnostics))
	for _, d := range a.Diagnostics {
		if (d.Reason != "ambiguous_target" && d.Reason != "missing_target") || d.Reason <= previous || d.Count <= 0 || d.Count > count-total {
			return evidence.Coverage{}, errors.New("rust attribution: invalid diagnostic reason/count/order")
		}
		total += d.Count
		previous = d.Reason
		details = append(details, fmt.Sprintf("%s=%d", d.Reason, d.Count))
	}
	if total != count {
		return evidence.Coverage{}, errors.New("rust attribution: diagnostic counts do not match unresolved symbols")
	}
	return evidence.Coverage{
		Status: evidence.StatusPartial,
		Reason: fmt.Sprintf(
			"Rust target attribution partial: %d unresolved symbols (%s) — rust-analyzer saw symbols it cannot pin to one Cargo target; pin languages.rust.manifest to the workspace you analyse, or set analyzers.scip.enabled: false to keep crate-level coupling",
			count, strings.Join(details, ", ")),
	}, nil
}
