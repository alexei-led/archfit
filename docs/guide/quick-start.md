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
version: 1
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

This is the report-only readout. `Decision` is the headline judgment, `Score` is the 0-100 health signal, findings appear in the sections below, and `Warnings` tells you how much advisory noise or degraded coverage the run found.

Sample output:

```text
ARCHFIT RESULT

Decision   ACCEPTABLE WITH WATCH ITEMS
Gate       PASS  ·  0 blocking
Warnings   38 advisory
Score      71 / 100  serviceable

RECOMMENDATIONS
  WATCH
    · bc/duplicated_knowledge — duplicated knowledge: cross-module code clones ...
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

This is the command to put in CI and local validation loops. Exit code `0` means pass, `1` means blocking violations or enforced regressions, `2` means a warning-level policy result, and `3` means config, tool, or runtime error.

Sample output:

```text
ARCHFIT RESULT

Decision   ACCEPTABLE WITH WATCH ITEMS
Gate       PASS  ·  0 blocking
Warnings   38 advisory
Score      71 / 100  serviceable
```

**If something looks wrong:** If the first gate run fails on already-known issues, you probably need to finish step 4 or narrow the policy. If it exits `3`, fix the config or toolchain problem before wiring it into CI.

## Next steps

- [Commands reference](commands.md)
- [CI guide](ci.md)
- [AI-agent feedback loop](agent-feedback.md)
