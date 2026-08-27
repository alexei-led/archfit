# Quick start

Run these from the repository root. Examples assume `archfit` is on your `PATH`.

## 1. Check analyzer tools

```sh
archfit doctor
```

Start here. `archfit doctor` checks the analyzers archfit needs and shows where each one was found. A healthy setup shows `ok` for the tools that match your repo's languages; if something is missing, fix that before you trust the results.

Sample output:

```text
TOOL             STATUS   PATH / INSTALL
------------------------------------------------------------
git              ok       /path/to/git
go               ok       /path/to/go
scip-go          ok       /path/to/scip-go
node             ok       /path/to/node
python3          ok       /path/to/python3
```

**If something looks wrong:** If a required tool shows `missing` or the path is wrong, install it or run `archfit doctor --fix`, then rerun this step. If a language you do not use appears, you can disable it later in `.archfit.yaml`.

## 2. Create config

```sh
archfit config init --root .
```

This writes a starter `.archfit.yaml` for the current repo. Review the generated module `paths`, `public` globs, layer assignments, enabled languages, and the starter `no-layer-back-edges` rule before you treat the file as policy.

Sample output:

```yaml
version: 2
languages:
  go:
    enabled: true
layers:
  - model
  - core
modules:
  cmd_archfit:
    paths:
      - "cmd/archfit/**"
rules:
  - id: no-layer-back-edges
    type: forbidden_layer_direction
    gate: warn
```

**If something looks wrong:** `config init` can only infer structure, not intent. Fix over-broad module globs, wrong layer names, and any disabled language you actually need before moving on.

## 3. Explore findings

```sh
archfit analyze -c .archfit.yaml
```

This is the report-only readout. `VERDICT` is the headline judgment, `COVERAGE`
says how many of the nine dimensions were actually measured, and each dimension
states its own denominator. There is no repository score.

Sample output:

```text
ARCHITECTURE STATE

VERDICT    NEEDS ATTENTION
BLOCKING   0 active  ·  hard gates: pass
ATTENTION  2 dimension(s) flagged  ·  55 diagnostic(s)
COVERAGE   5 measured · 3 partial · 1 unmeasured  (of 9)

DIMENSIONS

  intent          measured    gate: pass            confidence: high     declared rules evaluated 60/60
  structure       measured    gate: pass            confidence: high     discovered edges resolved to a declared module 544/1255
  ...
  drift           unmeasured  gate: not_applicable  confidence: unrated  no denominator

NOT MEASURED (5)

  complexity — cognitive complexity
    no cognitive-complexity analyzer is claimed; module-graph shape is the architecture-level measure

COUPLING SEAMS (67)

  assessment-repair -> relationship-analysis
    functional × cross_module_same_owner × high volatility · 12 critical of 34 scored · median balance 7
    try: reduce_strength
```

**If something looks wrong:** Do not baseline or gate a run you do not trust. Fix config path globs, language settings, or missing analyzer tools first, then rerun until the findings match the repo you meant to analyze.

## 4. Accept baseline

```sh
archfit baseline -c .archfit.yaml
```

Do this after you have reviewed the current findings. It writes `.archfit-baseline.json` and records the current finding fingerprints as accepted state, so future checks can focus on new drift instead of today's known debt.

Sample output:

```text
baseline saved: .archfit-baseline.json
```

**If something looks wrong:** If this baseline would accept findings you do not want to carry forward, stop and tighten the config or fix the code first. A baseline should lock in reviewed debt, not hide surprises.

## 5. Set up the CI gate

```sh
archfit check -c .archfit.yaml
```

This is the command to put in CI and local validation loops. The exit code IS the
verdict: `0` is `healthy`, `1` is `blocked`, `2` is `needs_attention`, and `3` is
a config, tool, or runtime error.

Exit `0` is reachable once every dimension has complete evidence, hard gates
pass, and no diagnostic is active. During adoption, missing supplied coverage,
operational corroboration, or a comparable persisted baseline can still produce
exit `2`. The following recipe allows that yellow state and gates on `1`:

```sh
archfit check -c .archfit.yaml || [ $? -eq 2 ]
```

Sample output:

```text
ARCHITECTURE STATE

VERDICT    NEEDS ATTENTION
BLOCKING   0 active  ·  hard gates: pass
ATTENTION  2 dimension(s) flagged  ·  55 diagnostic(s)
COVERAGE   5 measured · 3 partial · 1 unmeasured  (of 9)
```

**If something looks wrong:** If the first gate run fails on already-known issues, you probably need to finish step 4 or narrow the policy. If it exits `3`, fix the config or toolchain problem before wiring it into CI.

## Optional: measure a config change before adopting it

```sh
cp .archfit.yaml candidate.archfit.yaml   # edit the copy
archfit config compare candidate.archfit.yaml -c .archfit.yaml
```

This runs the full pipeline twice over the same code — once per config — and
prints only what changed. It writes nothing, ignores the baseline, and never
exits non-zero on a difference.

Sample output:

```text
config compare
  current:   .archfit.yaml
  candidate: candidate.archfit.yaml

coverage evidence: comparable

measurement differences:
  score: 71 (serviceable) → 84 (strong) (+13)
  findings only under the current config (2): 4f1c..., 9ab2...

measurement loss:
  - external_edges_rose: edges excluded as external rose (0 to 37)
```

**If something looks wrong:** Read the warnings before the score. A candidate
that scores higher while `external_edges_rose` or `scored_fraction_fell` fired
measured less of the same code — that is a loss of evidence, not an
improvement. A `coverage evidence` grade of `not_comparable` means the two runs
did not rest on the same analyzer evidence, so trust the difference less.

## Next steps

- [Commands reference](commands.md)
- [CI guide](ci.md)
- [AI-agent feedback loop](agent-feedback.md)
