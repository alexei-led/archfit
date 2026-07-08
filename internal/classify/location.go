package classify

import (
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/graph"
)

func couplingLocations(locs []graph.Location) []coupling.Location {
	if len(locs) == 0 {
		return nil
	}
	out := make([]coupling.Location, 0, len(locs))
	for _, loc := range locs {
		out = append(out, coupling.Location{File: loc.File, Line: loc.Line})
	}
	return out
}
