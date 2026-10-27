// Package dotnet implements the .NET project graph extractor using MSBuild
// evaluation. It is an out-of-process adapter: it runs `dotnet msbuild
// -getItem/-getProperty` through the toolrun.Runner boundary, parses the JSON,
// and normalises the result to graph.Facts.
//
// Granularity is project-level: each evaluated .csproj becomes a project node
// and each ProjectReference a depends_on edge. Evaluation never restores and
// never builds, but it does run the project's imported props and targets, and it
// may fetch NuGet-resolved MSBuild SDKs when the network is reachable.
//
// Multi-targeted projects are evaluated once without a target framework and once
// per entry of TargetFrameworks, because conditional references appear only in
// the per-framework run. References are the union across frameworks.
//
// The extractor emits facts only. Whether a project's edges are kept or replaced
// by file edges is decided by the caller from LastEvaluation.
package dotnet
