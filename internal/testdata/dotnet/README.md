# Shared .NET fixture

Used by the Roslyn helper tests (`tools/dotnet/archfit-roslyn`), the Go extractor tests, and the language/image contracts. Keep it tiny. Do not add `global.json`: the host SDK must be used. Do not add a restorable `PackageReference`.

`Fixture.slnx` lists seven projects. Namespaces are `Fixture.<Project>`; assembly names are the project names.

## Evaluation (`dotnet msbuild -getItem/-getProperty`)

Recorded with SDK 10.0.402, empty `NUGET_PACKAGES`, network blocked (`http(s)_proxy=http://127.0.0.1:9`). Command per project:

    dotnet msbuild <proj> -getItem:ProjectReference,PackageReference \
      -getProperty:TargetFramework -getProperty:TargetFrameworks -getProperty:IsTestProject

| Project | Exit | TargetFramework | TargetFrameworks | IsTestProject | ProjectReference | PackageReference |
| --- | --- | --- | --- | --- | --- | --- |
| Domain | 0 | net10.0 | | | none | none |
| Application | 0 | net10.0 | | | Domain | none |
| Infrastructure | 0 | net10.0 | | | Application, Domain | none |
| Legacy (no TFM) | 0 | | net10.0;net8.0 | | none | none |
| Legacy `-p:TargetFramework=net8.0` | 0 | net8.0 | net10.0;net8.0 | | none | none |
| Legacy `-p:TargetFramework=net10.0` | 0 | net10.0 | net10.0;net8.0 | | Domain | none |
| Broken | 1 | | | | | |
| Tests | 0 | net10.0 | | true | Broken | none |
| Unresolved | 0 | net10.0 | | | none | Archfit.Fixture.Missing.Package 1.0.0 |

Broken, last error line:

    Broken/Broken.csproj : error MSB4236: The SDK 'Archfit.Fixture.Missing.Sdk/1.0.0' specified could not be found.

Evaluation never needs restore, so `Unresolved` and `Tests` evaluate at exit 0. Without a TFM, `Legacy` shows no reference: the `net10.0` reference appears only in the per-TFM run (this is why the extractor loops over `TargetFrameworks`). The conditional reference is under `net10.0`, not `net8.0`: `Domain` targets only `net10.0`, and a `net8.0` project cannot reference it (the design-time build fails with "Project '../Domain/Domain.csproj' targets 'net10.0'. It cannot be referenced by a project that targets '.NETCoreApp,Version=v8.0'").

## Expected helper status

| Project | Status | Reason |
| --- | --- | --- |
| Domain | complete | |
| Application | complete | |
| Infrastructure | complete | |
| Legacy | one entry per TFM. `net10.0`: complete. `net8.0`: complete on a host with the 8.0 targeting pack; `partial` without it (the image) | `net8.0` has no framework references offline (`CS0518`); the reason names `Microsoft.NETCore.App.Ref` 8.0 (D7). Host tests assert `complete` or this `partial`, nothing else. Image contract asserts `partial`. |
| Broken | failed | MSB4236, SDK `Archfit.Fixture.Missing.Sdk` |
| Tests | partial | its document loads, so not `failed` (D5); the workspace failure names `Broken`'s SDK (MSB4236, `Archfit.Fixture.Missing.Sdk`), and `Fixture.Broken` is unresolved (`CS0234`) |
| Unresolved | partial | unresolved symbols counted (`Widget`, `Archfit.Fixture.Missing`); no node emitted for them |

No edges: `Broken` (failed, evaluation fails too, so no fallback) and `Tests` (partial, its only use is unresolved; as a test project its `ProjectReference` edges are dropped anyway). If the helper fails but evaluation succeeds (for example `Legacy`), fallback emits its `ProjectReference` edges (`Legacy` to `Domain`, from the `net10.0` run).

## Expected file edges (helper)

Paths relative to this directory. Hint is `StrengthHint`; data hint is `DataStrengthHint` (only `model` or `dto`, empty otherwise).

`Infrastructure/Repository.cs` to:

| Target file | Source construct | Hint | Data hint | D3 row |
| --- | --- | --- | --- | --- |
| `Application/Contracts.cs` | `IRepository` implemented | contract | | interface |
| `Application/Contracts.cs` | `OrderDto.Id` read in `Uses.Order` | dto | dto | record data use |
| `Domain/Domain.cs` | `Repository : Entity` (concrete inheritance), `new()`, `entity.Next()` | functional | | inherit / call |
| `Domain/Domain.cs` | `Policy` abstract use, `Allows` override | contract | | abstract type |
| `Domain/Domain.cs` | `Secrets.Seed` (`internal`, other assembly) | intrusive | | internal in other assembly |

The per-pair result is the strongest hint in each channel (rank contract < dto < model < functional < intrusive, as in the Go extractor), so `Application/Contracts.cs` aggregates to `dto` / `dto` and `Domain/Domain.cs` to `intrusive` / `model`. The per-construct rows, to test the pure hint function and per-use classification, live in `Uses`:

| Method in `Uses` | Symbol used | Hint | Data hint |
| --- | --- | --- | --- |
| `Model` | `Entity.Label` (field, data only) | model | model |
| `Dto` | `Money.Amount` (record struct, no behavior) | dto | dto |
| `Functional` | `Entity.Next()` (method call) | functional | |
| `Contract` | `Policy.Allows` (abstract member) | contract | |
| `Intrusive` | `Secrets.Seed` (`internal`, `InternalsVisibleTo` from Domain's csproj) | intrusive | |
| `Order` | `OrderDto.Id` (record property, data only) | dto | dto |

`Application/Generated/Mapper.g.cs` uses `OrderDto` in the same project. It has an `<auto-generated>` header, so it is a generated file; edges from it are not scored.

`Legacy/Legacy.cs` has an edge to `Domain/Domain.cs` (functional) only in the `net10.0` compilation (`#if NET10_0`). Union across TFMs: the edge exists.

## Other coverage

- Global using: `Application/GlobalUsings.cs` (`global using Fixture.Domain;`). `Contracts.cs` uses `Entity` without a `using`.
- Linked file: `Application/Application.csproj` links `..\Shared\Marker.cs` as `Linked\Marker.cs`. It lies outside the project directory; the helper reports it as a linked file outside the project.
- `InternalsVisibleTo`: `Domain/Domain.csproj` item for assembly `Infrastructure` (declaration is coverage detail, not an edge).
- Unused `ProjectReference` (complete projects only): `Infrastructure` uses both of its references; `Application` references `Domain` and uses it (`Entity`, via global using); none expected.
- Unresolved package: `Unresolved` references a package id that does not exist and uses its namespace.
