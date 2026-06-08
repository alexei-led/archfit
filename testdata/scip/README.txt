SCIP test fixtures

This directory holds test data for the internal/extract/scip adapter.

Once the scip Go bindings are available as a standalone importable module,
add a minimal .scip protobuf fixture here for barrel-file resolution tests.
The fixture should represent a small TypeScript project where src/index.ts
re-exports from src/components/Button.tsx, so the resolver can map:
  src/index.ts -> src/components/Button.tsx

Generate with:
  scip-typescript --output testdata/scip/index.scip <fixture-ts-root>
