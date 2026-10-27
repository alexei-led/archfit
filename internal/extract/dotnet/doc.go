// Package dotnet implements the .NET extractor. It is an out-of-process
// adapter: it runs `dotnet msbuild -getItem/-getProperty` and the Roslyn helper
// (tools/dotnet/archfit-roslyn) through the toolrun.Runner boundary, parses
// their JSON, and normalises the result to graph.Facts.
//
// The project graph comes from MSBuild evaluation: each evaluated .csproj
// becomes a project node and each ProjectReference a depends_on edge.
// Evaluation never restores and never builds, but it does run the project's
// imported props and targets, and it may fetch NuGet-resolved MSBuild SDKs when
// the network is reachable. It runs on the original tree.
//
// Multi-targeted projects are evaluated once without a target framework and once
// per entry of TargetFrameworks, because conditional references appear only in
// the per-framework run. References are the union across frameworks.
//
// The type graph comes from the Roslyn helper, located by ARCHFIT_DOTNET_ROSLYN.
// Its design-time build writes under each project's obj/, so it runs on a
// per-run copy of the root (everything but .git and node_modules), which is
// removed on every path. Its per-(project, framework) entries fold to one result
// per project: the worst status and the union of file edges, strongest hint per
// file pair in each of the two hint channels. exclude applies to the emitted
// evidence, after analysis.
//
// One scored edge set per source project: a project the helper loaded
// (complete or partial) emits file -> file edges with strength hints and no
// ProjectReference edges; any other project emits its ProjectReference edges,
// with no hint. The extractor emits hints only; internal/relationship decides
// strength.
package dotnet
