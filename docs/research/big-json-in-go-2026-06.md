# Parsing ast-grep's large JSON in Go: efficient, fast, no OOM

Date: 2026-06-26. Problem: `sg scan --json=compact` emits one JSON array that
reaches **~100 MB** on rule-dense repos (herdr: 13,957 matches, 100 MB). archfit
today reads the whole thing into `[]byte` and `json.Unmarshal`s it into
`[]sgSyntaxMatch` (`internal/extract/astgrep/syntax.go:334`) — peak memory ≈ input
buffer + full decoded slice. This is the latent OOM liability flagged in the
tooling report (§5.4). Goal: cut peak memory and avoid OOM without losing matches
or adding a fragile dependency. Every claim below is **measured**, not asserted.

## Method

One Go harness (`scratchpad/jsonbench`), one input (herdr's 100 MB output in both
`--json=compact` array and `--json=stream` NDJSON form), each strategy run as a
fresh process under `/usr/bin/time -l` for **peak RSS** (the OOM-relevant number),
plus in-process wall time and `runtime.MemStats`. Every strategy produces the same
13,957 compact "facts" (file, line range, ruleId, captured name) — mimicking
archfit converting matches → SyntaxFacts — so only the strategy differs. Built with
`CGO_ENABLED=0` to honour archfit's static-binary / cross-compile constraint. Go
1.26.4, Apple Silicon.

## Results (100 MB input, 13,957 matches)

| Strategy                                     | peak RSS   | wall   | deps          | what it does                                               |
| -------------------------------------------- | ---------- | ------ | ------------- | ---------------------------------------------------------- |
| `baseline-full` (**archfit today**)          | **221 MB** | 554 ms | stdlib        | ReadAll + Unmarshal whole array, full struct               |
| `baseline-lean`                              | 112 MB     | 539 ms | stdlib        | same, but struct decodes only needed fields                |
| `stream-bytes-lean`                          | 114 MB     | 536 ms | stdlib        | `json.Decoder` element-loop over in-memory bytes           |
| **`stream-file-lean`**                       | **13 MB**  | 528 ms | stdlib        | `json.Decoder` element-loop straight off the reader (pipe) |
| **`ndjson-lean`** (`--json=stream`)          | **16 MB**  | 550 ms | stdlib        | NDJSON, `bufio.Reader.ReadBytes`, per-line Unmarshal       |
| `ndjson-scanner-bad`                         | —          | —      | stdlib        | **FAILS — see footgun below**                              |
| `gojson-full`                                | 269 MB     | 126 ms | +go-json      | goccy/go-json drop-in whole-array Unmarshal                |
| `gojson-stream-lean`                         | 143 MB     | 108 ms | +go-json      | go-json streaming decoder, lean struct                     |
| `gjson-ondemand`                             | 207 MB     | 97 ms  | +gjson        | tidwall/gjson, pull only needed paths, no struct           |
| `gjson-ndjson`                               | 143 MB     | 75 ms  | +gjson        | gjson per NDJSON line                                      |
| `jsonv2-ndjson-lean` (`GOEXPERIMENT=jsonv2`) | **12 MB**  | 138 ms | stdlib (exp.) | `encoding/json/v2` per-line Unmarshal                      |

## What the numbers prove

1. **The dominant lever is streaming off the reader/pipe — not the decoder, not the
   library.** `stream-bytes-lean` (114 MB) still calls `os.ReadFile`, so the 100 MB
   input stays resident; it only saves the decoded-slice copy. `stream-file-lean`
   reads the input incrementally and discards each element after converting it →
   **13 MB, a 17× cut**, at identical wall time. Peak becomes independent of output
   size. _This is the fix._

2. **A lean struct halves memory for free.** Decoding only `file`/`range`/`ruleId`/
   `metaVariables.single` (the fields archfit uses) lets `encoding/json` skip the
   bulk — `labels`, `lines`, `text`, `charCount` are ~60% of each object.
   221 → 112 MB with no other change. Compose with streaming for the 13 MB result.

3. **NDJSON (`--json=stream`) is an equally good, simpler-code path — with one
   mandatory caveat.** Line-by-line `ReadBytes('\n')` + Unmarshal = 16 MB, stdlib,
   trivial. But see the footgun.

4. **A faster JSON library is the wrong tool here.** go-json and gjson cut CPU ~5×
   (554 → ~100 ms) but as whole-document parsers they _raise_ peak memory
   (269 MB / 207 MB) — they trade memory for speed, the opposite of what we need.
   Even their streaming forms (143 MB) don't approach stdlib streaming's 13 MB.
   archfit's parse is already ~0.5 s and is **not** the bottleneck (the `sg scan`
   subprocess is ~1 s); spending a dependency to shave CPU we don't need, while
   making the OOM worse, fails the cost/benefit and archfit's minimal-deps posture.

5. **`encoding/json/v2` is the future answer.** Under `GOEXPERIMENT=jsonv2` (Go
   1.26) it hits **12 MB and 138 ms** — the lowest memory _and_ ~4× faster than
   stdlib v1, pure standard library. When it leaves the experiment (targeted for a
   coming Go release) it is the drop-in that wins both axes. Do **not** depend on
   the experiment flag in production yet.

## The footgun (validated): `bufio.Scanner` silently truncates NDJSON

A naive `bufio.Scanner` over `--json=stream` is the obvious-looking implementation
and it is **wrong**. Scanner's default max token is 64 KB; herdr's NDJSON has **546
lines over 64 KB (max 567 KB)** — a single match whose `text`/`metaVariables` span a
large node. Result: `ndjson-scanner-bad` processed **3,098 of 13,957 matches
(22%)** then returned `bufio.Scanner: token too long`. It does not crash loudly —
it stops early, so the syntax-facts layer would silently lose 78% of matches and
report a confidently-wrong result. Mitigations: use `bufio.Reader.ReadBytes('\n')`
(no limit, used above), or `sc.Buffer(make([]byte, 0, 1<<20), 1<<20)`. Never the
default Scanner on tool output with unbounded line length.

## Recommendation for archfit

1. **Stream the ast-grep stdout pipe into `encoding/json.Decoder`, element by
   element, into a lean struct.** 221 MB → 13 MB, no new dependency, no wall-time
   cost, no lost matches. Same change applies to the `sg run` pattern path
   (`astgrep.go`), which buffers identically.
   - Implementation cost (why it's not a one-liner): `toolrun.Runner` currently
     returns `Stdout []byte` (fully buffered). This needs a streaming variant —
     `cmd.StdoutPipe()` + `cmd.Start()` + decode-while-running + `cmd.Wait()` for
     exit status — used for the two ast-grep calls. The benchmark's
     `stream-file-lean` is a faithful file-as-proxy for that pipe (both `io.Reader`,
     incremental; peak is size-independent either way).
2. **Adopt the lean struct now regardless** (`syntax.go`'s `sgSyntaxMatch`): drop
   the unused fields so even the current buffered path halves its memory (221 →
   112 MB) before the streaming change lands.
3. **If you prefer `--json=stream` NDJSON** (simpler per-object code, same 16 MB):
   parse with `bufio.Reader.ReadBytes('\n')`, never `bufio.Scanner` (see footgun).
4. **No new JSON dependency.** Revisit only when `encoding/json/v2` ships stable —
   then it supersedes both as a pure-stdlib drop-in (12 MB / 4× faster).
5. **Excluded: bytedance/sonic.** Arch-specific (amd64 JIT, arm64 fallback), trades
   memory for speed (won't fix OOM), and a heavy dependency against archfit's
   static cross-compile + minimal-deps constraints.

## Appendix: reproduce

`scratchpad/jsonbench/main.go` (subcommands = strategies). Generate inputs with
`sg scan -r internal/extract/astgrep/rules/rust.yml --json=compact ~/Workspace/herdr`
(and `--json=stream` for NDJSON). Run each subcommand under `/usr/bin/time -l`. The
`jsonv2` mode needs `GOEXPERIMENT=jsonv2 go build -tags jsonv2exp`.
