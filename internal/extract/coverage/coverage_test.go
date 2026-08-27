package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

const fixtureCoveragePath = "coverage.info"

type fixtureParser struct {
	format string
	facts  []evidence.CoverageFact
	calls  *int
}

func (p fixtureParser) Format() string { return p.format }
func (fixtureParser) Version() string  { return "fixture-parser.v1" }
func (p fixtureParser) Parse([]byte) ([]evidence.CoverageFact, error) {
	*p.calls++
	out := make([]evidence.CoverageFact, len(p.facts))
	copy(out, p.facts)
	return out, nil
}

func TestIngest_DisabledIsByteIdenticalToAbsentOnFixedInput(t *testing.T) {
	root := t.TempDir()
	ingestor := New(nil)
	absentFacts, absentRow := ingestor.IngestAll(root, Options{})
	disabledFacts, disabledRow := ingestor.IngestAll(root, Options{
		Enabled: false,
		Gate:    "fail",
		Sources: []Source{{Path: "must-not-be-read.info", Format: FormatLCOV}},
	})
	absent, err := json.Marshal(struct {
		Facts []evidence.CoverageIngest `json:"facts,omitempty"`
		Row   evidence.Coverage         `json:"row"`
	}{absentFacts, absentRow})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := json.Marshal(struct {
		Facts []evidence.CoverageIngest `json:"facts,omitempty"`
		Row   evidence.Coverage         `json:"row"`
	}{disabledFacts, disabledRow})
	if err != nil {
		t.Fatal(err)
	}
	if string(absent) != string(disabled) {
		t.Fatalf("absent = %s, explicit disabled = %s", absent, disabled)
	}
}

func TestIngest_DefaultLimitsApplyToDirectOptions(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	calls := 0
	ingests, _ := New(nil, fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
	}, calls: &calls}).IngestAll(root, Options{
		Enabled: true,
		Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV}},
	})
	if calls != 1 || len(ingests) != 1 || len(ingests[0].Facts) != 1 {
		t.Fatalf("direct Options ingest calls/results = %d/%+v, want default-bounded parsed fact", calls, ingests)
	}
	got := (Source{}).withDefaults()
	if got.MaxBytes != DefaultMaxBytes || got.MaxFacts != DefaultMaxFacts {
		t.Fatalf("direct Options defaults = %d bytes/%d facts, want %d/%d", got.MaxBytes, got.MaxFacts, DefaultMaxBytes, DefaultMaxFacts)
	}
}

func TestIngest_ArtifactByteLimitBoundary(t *testing.T) {
	const artifact = "coverage"
	for _, tc := range []struct {
		name       string
		maxBytes   int64
		wantParsed bool
	}{
		{name: "at limit", maxBytes: int64(len(artifact)), wantParsed: true},
		{name: "limit plus one", maxBytes: int64(len(artifact) - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, sourcePath := coverageFixture(t)
			writeCoverageFile(t, root, fixtureCoveragePath, artifact)
			calls := 0
			ingests, _ := New(nil, fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
				{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
			}, calls: &calls}).IngestAll(root, Options{Enabled: true, Sources: []Source{{
				Path: fixtureCoveragePath, Format: FormatLCOV, MaxBytes: tc.maxBytes,
			}}})
			got := ingests[0]
			if tc.wantParsed {
				if calls != 1 || len(got.Facts) != 1 || strings.Contains(got.Reason, reasonCoverageArtifactTooLarge) {
					t.Fatalf("at-limit ingest calls/result = %d/%+v", calls, got)
				}
				return
			}
			if calls != 0 || len(got.Facts) != 0 || got.Reason != reasonCoverageArtifactTooLarge {
				t.Fatalf("over-limit ingest calls/result = %d/%+v, want reject before parse", calls, got)
			}
		})
	}
}

func TestIngest_FactLimitBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		facts     int
		wantFacts int
	}{
		{name: "at limit", facts: 1, wantFacts: 1},
		{name: "limit plus one", facts: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, sourcePath := coverageFixture(t)
			facts := make([]evidence.CoverageFact, tc.facts)
			for idx := range facts {
				facts[idx] = evidence.CoverageFact{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines}
			}
			calls := 0
			ingests, _ := New(nil, fixtureParser{format: FormatLCOV, facts: facts, calls: &calls}).IngestAll(root, Options{
				Enabled: true,
				Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxFacts: 1}},
			})
			got := ingests[0]
			if len(got.Facts) != tc.wantFacts {
				t.Fatalf("facts = %d, want %d: %+v", len(got.Facts), tc.wantFacts, got)
			}
			if tc.facts > 1 && !strings.Contains(got.Reason, reasonCoverageFactLimitExceeded) {
				t.Fatalf("reason = %q, want %q", got.Reason, reasonCoverageFactLimitExceeded)
			}
		})
	}
}

func TestIngest_FactLimitIsEnforcedOnCacheHit(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
	calls := 0
	ingestor := New(factcache.NewStore(filepath.Join(t.TempDir(), "facts")), fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
	}, calls: &calls})

	atLimit, _ := ingestor.IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxFacts: 2}}})
	if len(atLimit[0].Facts) != 2 || calls != 1 {
		t.Fatalf("cache priming ingest = %+v, calls = %d", atLimit[0], calls)
	}
	overLimit, _ := ingestor.IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxFacts: 1}}})
	if len(overLimit[0].Facts) != 0 || !strings.Contains(overLimit[0].Reason, reasonCoverageFactLimitExceeded) {
		t.Fatalf("over-limit cache hit = %+v", overLimit[0])
	}
	if calls != 1 {
		t.Fatalf("parser calls = %d, want cache hit rejected without reparsing", calls)
	}
}

func TestIngest_AttestationReadsHonorByteLimitBoundary(t *testing.T) {
	t.Run("sidecar at limit and limit plus one", func(t *testing.T) {
		root, sourcePath := coverageFixture(t)
		const sidecar = "coverage.info.sidecar.json"
		writeSidecar(t, root, sidecar, "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
		info, err := os.Stat(filepath.Join(root, sidecar))
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		ingestor := New(nil, fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
			{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
		}, calls: &calls})

		atLimit, _ := ingestor.IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxBytes: info.Size()}}})
		if atLimit[0].Freshness != evidence.FreshnessMatched || atLimit[0].Reason != "" {
			t.Fatalf("at-limit sidecar ingest = %+v", atLimit[0])
		}
		overLimit, _ := ingestor.IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxBytes: info.Size() - 1}}})
		if overLimit[0].Freshness != evidence.FreshnessUnverified || !strings.Contains(overLimit[0].Reason, reasonCoverageSidecarTooLarge) {
			t.Fatalf("over-limit sidecar ingest = %+v", overLimit[0])
		}
	})

	t.Run("attested source at limit and limit plus one", func(t *testing.T) {
		root, sourcePath := coverageFixture(t)
		contents := strings.Repeat("x", 512)
		writeCoverageFile(t, root, sourcePath, contents)
		writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
		calls := 0
		ingestor := New(nil, fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
			{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
		}, calls: &calls})

		atLimit, _ := ingestor.IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxBytes: int64(len(contents))}}})
		if atLimit[0].Freshness != evidence.FreshnessMatched || atLimit[0].Reason != "" {
			t.Fatalf("at-limit attested source ingest = %+v", atLimit[0])
		}
		overLimit, _ := ingestor.IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV, MaxBytes: int64(len(contents) - 1)}}})
		if overLimit[0].Freshness != evidence.FreshnessStale || !strings.Contains(overLimit[0].Reason, reasonCoverageAttestedSourceTooLarge) {
			t.Fatalf("over-limit attested source ingest = %+v", overLimit[0])
		}
	})
}

func TestAttest_SourceFailureReasonUsesSortedPathOrder(t *testing.T) {
	root := t.TempDir()
	const (
		mismatchedSource = "src/a-hash-mismatch.go"
		oversizedSource  = "src/z-oversized.go"
		sidecarPath      = "coverage.info.sidecar.json"
		maxBytes         = int64(512)
	)
	writeCoverageFile(t, root, mismatchedSource, "package mismatch\n")
	writeCoverageFile(t, root, oversizedSource, strings.Repeat("x", int(maxBytes)+1))
	writeSidecar(t, root, sidecarPath, "producer-ref", map[string]string{
		mismatchedSource: strings.Repeat("0", sha256.Size*2),
		oversizedSource:  fileHash(t, root, oversizedSource),
	}, 1)
	info, err := os.Stat(filepath.Join(root, sidecarPath))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxBytes {
		t.Fatalf("sidecar size = %d, test limit = %d", info.Size(), maxBytes)
	}
	normalizer, err := NewNormalizer(root)
	if err != nil {
		t.Fatal(err)
	}

	for run := 0; run < 100; run++ {
		got := attest(normalizer, sidecarPath, maxBytes)
		if got.freshness != evidence.FreshnessStale || got.reason != reasonWorktreeDiffers {
			t.Fatalf("run %d freshness/reason = %q/%q, want stale/%s from lexically first source",
				run, got.freshness, got.reason, reasonWorktreeDiffers)
		}
	}
}

func TestCoverageCacheKeyIncludesParserVersionAndScanRoot(t *testing.T) {
	data := []byte("coverage")
	base := coverageCacheKey("/scan/a", fixtureCoveragePath, FormatLCOV, "parser.v1", "ref-a", data)
	if got := coverageCacheKey("/scan/a", fixtureCoveragePath, FormatLCOV, "parser.v2", "ref-a", data); got == base {
		t.Fatal("parser version did not invalidate the coverage fact key")
	}
	if got := coverageCacheKey("/scan/b", fixtureCoveragePath, FormatLCOV, "parser.v1", "ref-a", data); got == base {
		t.Fatal("ScanRoot did not invalidate the coverage fact key")
	}
	if got := coverageCacheKey("/scan/a", fixtureCoveragePath, FormatLCOV, "parser.v1", "ref-b", data); got == base {
		t.Fatal("source ref did not invalidate the coverage fact key")
	}
}

func TestIngest_AttestedArtifactWithNilStore(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	writeSidecar(t, root, "coverage.info.sidecar.json", "definitely-not-the-worktree-head", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
	calls := 0
	ingests, row := New(nil, fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 7, TotalUnits: 10, Unit: coverageUnitLines},
	}, calls: &calls}).IngestAll(root, fixtureOptions())

	if calls != 1 {
		t.Fatalf("parser calls = %d, want 1", calls)
	}
	assertMatchedIngest(t, ingests, row, sourcePath)
}

func TestIngest_CachesParsedFactsByArtifactContent(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	writeSidecar(t, root, "coverage.info.sidecar.json", "definitely-not-the-worktree-head", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
	calls := 0
	store := factcache.NewStore(filepath.Join(t.TempDir(), "facts"))
	ingestor := New(store, fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 7, TotalUnits: 10, Unit: coverageUnitLines},
	}, calls: &calls})
	options := Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV}}}

	first, firstRow := ingestor.IngestAll(root, options)
	second, secondRow := ingestor.IngestAll(root, options)
	if calls != 1 {
		t.Fatalf("parser calls after identical input = %d, want 1", calls)
	}
	assertMatchedIngest(t, first, firstRow, sourcePath)
	assertMatchedIngest(t, second, secondRow, sourcePath)

	writeCoverageFile(t, root, fixtureCoveragePath, "coverage changed\n")
	third, _ := ingestor.IngestAll(root, options)
	if calls != 2 {
		t.Fatalf("parser calls after artifact content change = %d, want 2", calls)
	}
	if third[0].Freshness != evidence.FreshnessMatched {
		t.Fatalf("freshness after artifact-only edit = %q, want matched", third[0].Freshness)
	}
}

func TestIngest_FreshnessRecomputedOnCacheHits(t *testing.T) {
	t.Run("listed source modified", func(t *testing.T) {
		root, sourcePath := coverageFixture(t)
		writeSidecar(t, root, "coverage.info.sidecar.json", "same-as-head-is-not-proof", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
		calls := 0
		ingestor := fixtureIngestor(t, sourcePath, &calls)
		options := fixtureOptions()
		first, _ := ingestor.IngestAll(root, options)
		if first[0].Freshness != evidence.FreshnessMatched {
			t.Fatalf("first freshness = %q", first[0].Freshness)
		}
		writeCoverageFile(t, root, sourcePath, "package app // changed\n")
		second, _ := ingestor.IngestAll(root, options)
		assertStale(t, second[0])
		if calls != 1 {
			t.Fatalf("source edit should reuse parsed artifact facts; parser calls = %d", calls)
		}
	})

	t.Run("listed source deleted", func(t *testing.T) {
		root, sourcePath := coverageFixture(t)
		writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
		calls := 0
		ingestor := fixtureIngestor(t, sourcePath, &calls)
		ingestor.IngestAll(root, fixtureOptions())
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(sourcePath))); err != nil {
			t.Fatal(err)
		}
		second, _ := ingestor.IngestAll(root, fixtureOptions())
		assertStale(t, second[0])
	})

	t.Run("gitignored listed source modified", func(t *testing.T) {
		root, sourcePath := coverageFixture(t)
		writeCoverageFile(t, root, ".gitignore", sourcePath+"\n")
		writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
		calls := 0
		ingestor := fixtureIngestor(t, sourcePath, &calls)
		ingestor.IngestAll(root, fixtureOptions())
		writeCoverageFile(t, root, sourcePath, "package app // ignored edit\n")
		second, _ := ingestor.IngestAll(root, fixtureOptions())
		assertStale(t, second[0])
	})
}

func TestIngest_CoveredFactsMustBeAttested(t *testing.T) {
	cases := []struct {
		name          string
		writeSidecar  func(t *testing.T, root, sourcePath, secondSource, unrelatedSource string)
		facts         func(sourcePath, secondSource string) []evidence.CoverageFact
		wantFreshness evidence.CoverageFreshness
		wantReason    string
	}{
		{
			name: "sources field omitted",
			writeSidecar: func(t *testing.T, root, _, _, _ string) {
				writeCoverageFile(t, root, "coverage.info.sidecar.json", `{"schema_version":1,"source_ref":"producer-ref","modules":["app"]}`)
			},
			wantFreshness: evidence.FreshnessUnverified,
			wantReason:    reasonFreshnessUnverified,
		},
		{
			name: "empty sources map",
			writeSidecar: func(t *testing.T, root, _, _, _ string) {
				writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{}, 1)
			},
			wantFreshness: evidence.FreshnessUnverified,
			wantReason:    reasonFreshnessUnverified,
		},
		{
			name: "partial sources map",
			writeSidecar: func(t *testing.T, root, sourcePath, _, _ string) {
				writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{
					sourcePath: fileHash(t, root, sourcePath),
				}, 1)
			},
			facts: func(sourcePath, secondSource string) []evidence.CoverageFact {
				return []evidence.CoverageFact{
					{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
					{File: secondSource, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
				}
			},
			wantFreshness: evidence.FreshnessStale,
			wantReason:    reasonWorktreeDiffers,
		},
		{
			name: "unrelated sources map",
			writeSidecar: func(t *testing.T, root, _, _, unrelatedSource string) {
				writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{
					unrelatedSource: fileHash(t, root, unrelatedSource),
				}, 1)
			},
			wantFreshness: evidence.FreshnessStale,
			wantReason:    reasonWorktreeDiffers,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, sourcePath := coverageFixture(t)
			const (
				secondSource    = "src/worker.go"
				unrelatedSource = "src/unrelated.go"
			)
			writeCoverageFile(t, root, secondSource, "package worker\n")
			writeCoverageFile(t, root, unrelatedSource, "package unrelated\n")
			tc.writeSidecar(t, root, sourcePath, secondSource, unrelatedSource)

			facts := []evidence.CoverageFact{{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines}}
			if tc.facts != nil {
				facts = tc.facts(sourcePath, secondSource)
			}
			calls := 0
			ingests, row := New(nil, fixtureParser{format: FormatLCOV, facts: facts, calls: &calls}).IngestAll(root, fixtureOptions())
			if calls != 1 || len(ingests) != 1 || len(ingests[0].Facts) != len(facts) {
				t.Fatalf("ingest calls/results = %d/%+v", calls, ingests)
			}
			got := ingests[0]
			if got.Freshness != tc.wantFreshness || !strings.Contains(got.Reason, tc.wantReason) {
				t.Fatalf("freshness/reason = %q/%q, want %q containing %q", got.Freshness, got.Reason, tc.wantFreshness, tc.wantReason)
			}
			if row.Status != evidence.StatusPartial {
				t.Fatalf("coverage health = %+v, want partial", row)
			}
		})
	}
}

func TestIngest_CoveredFactsMustBeAttestedOnCacheHit(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	const secondSource = "src/worker.go"
	writeCoverageFile(t, root, secondSource, "package worker\n")
	writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{
		sourcePath:   fileHash(t, root, sourcePath),
		secondSource: fileHash(t, root, secondSource),
	}, 1)

	calls := 0
	ingestor := New(factcache.NewStore(filepath.Join(t.TempDir(), "facts")), fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
		{File: secondSource, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
	}, calls: &calls})
	first, _ := ingestor.IngestAll(root, fixtureOptions())
	if calls != 1 || first[0].Freshness != evidence.FreshnessMatched {
		t.Fatalf("cache-priming ingest = %+v, calls = %d", first[0], calls)
	}

	writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{
		sourcePath: fileHash(t, root, sourcePath),
	}, 1)
	second, row := ingestor.IngestAll(root, fixtureOptions())
	if calls != 1 {
		t.Fatalf("parser calls = %d, want cache hit", calls)
	}
	if second[0].Freshness != evidence.FreshnessStale || second[0].Reason != reasonWorktreeDiffers {
		t.Fatalf("cache-hit freshness/reason = %q/%q, want stale/%s", second[0].Freshness, second[0].Reason, reasonWorktreeDiffers)
	}
	if row.Status != evidence.StatusPartial {
		t.Fatalf("coverage health = %+v, want partial", row)
	}
}

func TestIngest_UnverifiedSidecarsNeverPromoteOrCache(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root, sourcePath string)
	}{
		{name: "missing sidecar"},
		{name: "unreadable sidecar", setup: func(t *testing.T, root, _ string) {
			if err := os.Mkdir(filepath.Join(root, "unreadable-sidecar"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "malformed sidecar", setup: func(t *testing.T, root, _ string) {
			writeCoverageFile(t, root, "coverage.info.sidecar.json", "{not-json")
		}},
		{name: "unrecognized schema", setup: func(t *testing.T, root, sourcePath string) {
			writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 2)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, sourcePath := coverageFixture(t)
			options := fixtureOptions()
			if tc.name == "unreadable sidecar" {
				options.Sources[0].SidecarPath = "unreadable-sidecar"
			}
			if tc.setup != nil {
				tc.setup(t, root, sourcePath)
			}
			calls := 0
			cacheDir := filepath.Join(t.TempDir(), "facts")
			ingestor := New(factcache.NewStore(cacheDir), fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
				{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
			}, calls: &calls})
			first, _ := ingestor.IngestAll(root, options)
			second, _ := ingestor.IngestAll(root, options)
			for _, got := range []evidence.CoverageIngest{first[0], second[0]} {
				if got.Freshness != evidence.FreshnessUnverified || !strings.Contains(got.Reason, reasonFreshnessUnverified) {
					t.Fatalf("ingest = %+v, want unverified/freshness_unverified", got)
				}
			}
			if calls != 2 {
				t.Fatalf("unverified ingest parser calls = %d, want 2 (never cached)", calls)
			}
			if got := regularFileCount(t, cacheDir); got != 0 {
				t.Fatalf("unverified cache files = %d, want 0", got)
			}
		})
	}
}

func TestIngest_StaleAttestationIsNeverWrittenToCache(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	const staleSidecar = "attest/stale.json"
	writeSidecar(t, root, staleSidecar, "producer-ref", map[string]string{sourcePath: strings.Repeat("0", sha256.Size*2)}, 1)
	calls := 0
	cacheDir := filepath.Join(t.TempDir(), "facts")
	ingestor := New(factcache.NewStore(cacheDir), fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
	}, calls: &calls})
	options := fixtureOptions()
	options.Sources[0].SidecarPath = staleSidecar
	first, _ := ingestor.IngestAll(root, options)
	second, _ := ingestor.IngestAll(root, options)
	assertStale(t, first[0])
	assertStale(t, second[0])
	if calls != 2 {
		t.Fatalf("stale ingest parser calls = %d, want 2", calls)
	}
	if got := regularFileCount(t, cacheDir); got != 0 {
		t.Fatalf("stale cache files = %d, want 0", got)
	}
}

func TestIngest_UnresolvedPathsAreCountedAndNeverCached(t *testing.T) {
	root, sourcePath := coverageFixture(t)
	writeSidecar(t, root, "coverage.info.sidecar.json", "producer-ref", map[string]string{sourcePath: fileHash(t, root, sourcePath)}, 1)
	calls := 0
	cacheDir := filepath.Join(t.TempDir(), "facts")
	ingestor := New(factcache.NewStore(cacheDir), fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
		{File: "../outside.go", CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
	}, calls: &calls})

	first, _ := ingestor.IngestAll(root, fixtureOptions())
	second, _ := ingestor.IngestAll(root, fixtureOptions())
	for _, got := range []evidence.CoverageIngest{first[0], second[0]} {
		if got.UnresolvedPaths != 1 || len(got.Facts) != 1 {
			t.Fatalf("ingest unresolved/facts = %d/%d, want 1/1", got.UnresolvedPaths, len(got.Facts))
		}
		if !strings.Contains(got.Reason, reasonUnresolvedCoveragePaths) {
			t.Fatalf("reason = %q, want %q", got.Reason, reasonUnresolvedCoveragePaths)
		}
	}
	if calls != 2 {
		t.Fatalf("partial ingest parser calls = %d, want 2", calls)
	}
	if got := regularFileCount(t, cacheDir); got != 0 {
		t.Fatalf("partial cache files = %d, want 0", got)
	}
}

func TestIngest_MissingConfiguredSourceProducesAbsentHealthRow(t *testing.T) {
	root := t.TempDir()
	ingests, row := New(nil).IngestAll(root, Options{Enabled: true, Sources: []Source{{Path: "missing.info", Format: FormatLCOV}}})
	if len(ingests) != 1 || ingests[0].Reason != reasonCoverageSourceUnavailable {
		t.Fatalf("ingests = %+v", ingests)
	}
	if row.Tool != ToolName || row.Status != evidence.StatusAbsent {
		t.Fatalf("coverage row = %+v, want %s/absent", row, ToolName)
	}
}

func assertMatchedIngest(t *testing.T, ingests []evidence.CoverageIngest, row evidence.Coverage, sourcePath string) {
	t.Helper()
	if len(ingests) != 1 || len(ingests[0].Facts) != 1 {
		t.Fatalf("ingests = %+v", ingests)
	}
	got := ingests[0]
	if got.Freshness != evidence.FreshnessMatched || got.Reason != "" {
		t.Fatalf("ingest freshness/reason = %q/%q", got.Freshness, got.Reason)
	}
	fact := got.Facts[0]
	if fact.File != sourcePath || fact.SourcePath != fixtureCoveragePath || fact.SourceRef != "definitely-not-the-worktree-head" {
		t.Fatalf("normalized fact = %+v", fact)
	}
	if row.Status != evidence.StatusOK {
		t.Fatalf("health row = %+v", row)
	}
}

func assertStale(t *testing.T, got evidence.CoverageIngest) {
	t.Helper()
	if got.Freshness != evidence.FreshnessStale || got.Reason != reasonWorktreeDiffers {
		t.Fatalf("ingest freshness/reason = %q/%q, want stale/%s", got.Freshness, got.Reason, reasonWorktreeDiffers)
	}
}

func coverageFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	const sourcePath = "src/app.go"
	writeCoverageFile(t, root, sourcePath, "package app\n")
	writeCoverageFile(t, root, fixtureCoveragePath, "coverage\n")
	return root, sourcePath
}

func fixtureIngestor(t *testing.T, sourcePath string, calls *int) *Ingestor {
	t.Helper()
	return New(factcache.NewStore(filepath.Join(t.TempDir(), "facts")), fixtureParser{format: FormatLCOV, facts: []evidence.CoverageFact{
		{File: sourcePath, CoveredUnits: 1, TotalUnits: 1, Unit: coverageUnitLines},
	}, calls: calls})
}

func fixtureOptions() Options {
	return Options{Enabled: true, Sources: []Source{{Path: fixtureCoveragePath, Format: FormatLCOV}}}
}

func writeCoverageFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSidecar(t *testing.T, root, rel, sourceRef string, sources map[string]string, schemaVersion int) {
	t.Helper()
	data, err := json.Marshal(attestationSidecar{SchemaVersion: schemaVersion, SourceRef: sourceRef, Modules: []string{"app"}, Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	writeCoverageFile(t, root, rel, string(data))
}

func fileHash(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // test helper paths are fixture-owned
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func regularFileCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			count++
		}
		return nil
	})
	return count
}
