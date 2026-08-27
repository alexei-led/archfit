# Coverage parser fixtures

These minimal fixtures use the public output shapes documented by the tools:

- `go.coverprofile`: `go test -coverprofile` coverprofile text format.
- `lcov.info`: LCOV tracefile format emitted by c8, Jest, and cargo llvm-cov.
- `coverage.json`: coverage.py `--cov-report=json` format.
- `llvm-cov.json`: `llvm-cov export -format=json` format.

The values are hand-reduced examples of those formats, with no repository or
proprietary source data. Parser tests assert the numeric per-file summaries.
