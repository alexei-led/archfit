package scip

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

func TestRustPackageEdgesAggregateObservedReferences(t *testing.T) {
	for _, attribution := range []string{rustAttributionOK, rustAttributionPartial} {
		t.Run(attribution, func(t *testing.T) {
			h := newRustPipelineHarness(t)
			sourceCrate, _, _ := strings.Cut("custom_lib::api", "::")
			h.readerJSON = `{"edges":[
				{"from":"custom_lib::api","to":"custom_bin::types","strength":"model","data_strength":"model","connascence":["type"]},
				{"from":"custom_lib::run","to":"custom_bin::run","strength":"functional","data_strength":"contract","connascence":["name","algorithm"]},
				{"from":"custom_lib::local","to":"custom_lib::api","strength":"intrusive","connascence":["name"]}
			],"symbols":[{"symbol":"root","module":"custom_lib","path":"src/lib.rs"}],"rust_attribution":` + attribution + `}`
			store := factcache.NewStore(t.TempDir())
			var cold []evidence.Coverage
			for run := range 2 {
				a := h.adapter(t, store)
				s := scope.Scope{Root: h.root}
				strengths, sc, _ := a.Strengths(t.Context(), s)
				data, dc, _ := a.DataStrengths(t.Context(), s)
				conn, cc, _ := a.Connascence(t.Context(), s)
				syms, sy, _ := a.Symbols(t.Context(), s)
				key := "z-root\x00a-member"
				if strengths[key] != strengthFunctional || data[key] != strengthModel || len(conn[key]) != 3 {
					t.Fatalf("primary package alias lost observed aggregate: %v %v %v", strengths, data, conn)
				}
				if strengths["custom_lib::api\x00custom_bin::types"] != strengthModel || strengths["custom_lib::local\x00custom_lib::api"] != strengthIntrusive || syms.Module["root"] != sourceCrate {
					t.Fatal("deep module facts/ownership changed")
				}
				if _, ok := strengths["z-root\x00z-root"]; ok {
					t.Fatal("same-package modules collapsed into a dependency")
				}
				if sc.FilesSeen != 3 || sc.FilesApplicable != 3 || sy.FilesSeen != 1 {
					t.Fatalf("aliases inflated observed counts: %+v %+v", sc, sy)
				}
				cov := []evidence.Coverage{sc, dc, cc, sy}
				if run == 0 {
					cold = cov
				} else if !reflect.DeepEqual(cold, cov) {
					t.Fatalf("cold/warm coverage differs: %+v %+v", cold, cov)
				}
				want := evidence.StatusOK
				if attribution == rustAttributionPartial {
					want = evidence.StatusPartial
				}
				for _, c := range cov {
					if c.Status != want {
						t.Fatalf("reader attribution overwritten: %+v", c)
					}
				}
			}
			if h.metadataRuns != 2 || h.indexRuns != 1 || h.readerRuns != 1 {
				t.Fatal("mapped cache not shared")
			}
		})
	}
}

func TestRustPackageEdgesDoNotInferUnusedDependencies(t *testing.T) {
	h := newRustPipelineHarness(t)
	h.readerJSON = `{"edges":[{"from":"custom_lib::one","to":"custom_lib::two","strength":"functional"}],"symbols":[{"symbol":"root","module":"custom_lib"}],"rust_attribution":` + rustAttributionOK + `}`
	m, cov, _ := h.adapter(t, nil).Strengths(t.Context(), scope.Scope{Root: h.root})
	if len(m) != 1 || m["z-root\x00a-member"] != "" || cov.Status != evidence.StatusOK {
		t.Fatalf("unused package gained inferred strength: %v %+v", m, cov)
	}
}
func TestRustPackageEdgesAbstainNamespaceCollisions(t *testing.T) {
	const overlapDeepRoot = "aggregate overlaps deep root"
	for _, collision := range []string{"shared target", "package overlaps target", overlapDeepRoot} {
		t.Run(collision, func(t *testing.T) {
			h := newRustPipelineHarness(t)
			from, to := "custom_lib::one", "custom_bin::two"
			switch collision {
			case "shared target":
				h.metadata = strings.ReplaceAll(h.metadata, `"custom-bin"`, `"custom_lib"`)
				to = "custom_lib::two"
			case "package overlaps target":
				h.metadata = strings.ReplaceAll(h.metadata, `"custom-bin"`, `"z_root"`)
				// A literal package name may itself be a crate identifier.
				h.metadata = strings.ReplaceAll(h.metadata, `"name":"z-root"`, `"name":"z_root"`)
				// Keep the harness's expected --package contract unchanged: collision
				// is exercised directly below, without tool invocation.
				to = "z_root::two"
			case overlapDeepRoot:
				h.metadata = strings.ReplaceAll(h.metadata, `"name":"z-root"`, `"name":"custom_lib"`)
				h.metadata = strings.ReplaceAll(h.metadata, `"name":"a-member"`, `"name":"custom_bin"`)
			}
			h.readerJSON = `{"edges":[{"from":"` + from + `","to":"` + to + `","strength":"functional"}],"symbols":[],"rust_attribution":` + rustAttributionOK + `}`
			if collision == overlapDeepRoot {
				h.readerJSON = `{"edges":[{"from":"custom_lib","to":"custom_bin","strength":"model","data_strength":"model","connascence":["type"]},{"from":"custom_lib::one","to":"custom_bin::two","strength":"functional","connascence":["algorithm"]}],"symbols":[],"rust_attribution":` + rustAttributionOK + `}`
			}
			ro, cov, ok := rustPackageResult(pipelineResult{raw: []byte(h.readerJSON), indexer: indexerRust}, evidence.Coverage{Status: evidence.StatusOK}, []byte(h.metadata))
			if !ok || len(ro.packageEdges) != 0 || cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "Rust package aliases") {
				t.Fatalf("collision guessed or hidden: %+v %+v %v", ro, cov, ok)
			}
		})
	}
}
