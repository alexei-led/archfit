# Quick start

From the root of a repository:

```sh
archfit init --root .
$EDITOR .archfit.yaml
archfit analyze --config .archfit.yaml --full
```

Recommended first-run workflow:

1. Generate `.archfit.yaml` with `archfit init`.
2. Review modules, layers, public APIs, and generated rules.
3. Run `archfit analyze --full` to see current findings (report-only, no gate).
4. Keep early rules narrow while calibrating.
5. Save accepted current findings with `archfit baseline --full`.
6. Add `--gate` once you want CI to block on violations.

Create a Markdown audit report:

```sh
archfit analyze --markdown --config .archfit.yaml > archfit-report.md
```

If the current findings are accepted technical debt, save a baseline:

```sh
archfit baseline --full --config .archfit.yaml
```

After that, new findings are marked as `new`; known findings are marked as
`baseline` until fixed or re-baselined.
