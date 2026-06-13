# CI

Run the gate in CI after checkout and language setup:

```sh
archfit check --config .archfit.yaml --full
```

For pull requests, compare to the base branch when available:

```sh
archfit check --config .archfit.yaml --base origin/main
```

Store `archfit scan` output as an artifact when a full report is useful:

```sh
archfit scan --config .archfit.yaml > archfit-report.md
```

For GitHub code scanning (inline PR annotations), emit SARIF and upload:

```sh
archfit check --config .archfit.yaml --full --format sarif > archfit.sarif
# then upload with actions/upload-sarif
```

Calibrate locally before adding the check to required CI. Keep early rules narrow
and baseline accepted current findings before treating the check as a merge gate.
