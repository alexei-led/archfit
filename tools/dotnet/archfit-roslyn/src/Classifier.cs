using System.Text.RegularExpressions;
using Microsoft.CodeAnalysis;

namespace ArchfitRoslyn;

// Classifier turns a loaded solution plus workspace diagnostics into the
// report, deciding each project's status (D5):
//   failed   0 documents, or requested and never loaded
//   partial  documents loaded, and a workspace failure or unresolved names
//   complete documents loaded, no failure, no unresolved names
// Every requested project gets an entry: one the workspace dropped, or whose
// failure names no parsable project path, is failed, never omitted.
internal static partial class Classifier
{
    public static async Task<Report> ClassifyAsync(
        Solution solution, string root, IReadOnlyList<WorkspaceDiagnostic> diagnostics, CancellationToken ct,
        IReadOnlyCollection<string>? requested = null)
    {
        var fullRoot = Paths.Full(root);
        var unattributed = diagnostics
            .Where(d => d.Kind == WorkspaceDiagnosticKind.Failure && FailingProject(d.Message) is null)
            .Select(d => d.Message)
            .FirstOrDefault();
        var failures = diagnostics
            .Where(d => d.Kind == WorkspaceDiagnosticKind.Failure)
            .Select(d => (Project: FailingProject(d.Message), d.Message))
            .Where(f => f.Project is not null)
            .GroupBy(f => f.Project!, StringComparer.Ordinal)
            .ToDictionary(g => g.Key, g => g.Select(f => f.Message).ToList(), StringComparer.Ordinal);

        var collector = new Collector(solution, fullRoot);
        var skipped = new SortedSet<string>(StringComparer.Ordinal);
        var reports = new List<ProjectReport>();
        var loaded = new HashSet<string>(StringComparer.Ordinal);

        foreach (var project in solution.Projects)
        {
            if (project.FilePath is null || Relative(fullRoot, project.FilePath) is not { } path)
            {
                continue;
            }
            loaded.Add(Paths.Full(project.FilePath));
            if (project.Language != LanguageNames.CSharp)
            {
                skipped.Add(path);
                continue;
            }
            var projectFailures = failures.GetValueOrDefault(Paths.Full(project.FilePath)) ?? [];
            if (project.DocumentIds.Count == 0)
            {
                // A project whose evaluation failed stays in the workspace with
                // no documents; the failure may not name it parsably.
                reports.Add(Failed(path, projectFailures.FirstOrDefault() ?? unattributed ?? NoDocuments, fullRoot));
                continue;
            }
            var collected = await collector.CollectAsync(project, ct);
            var status = projectFailures.Count == 0 && collected.UnresolvedSymbols == 0 ? Status.Complete : Status.Partial;
            reports.Add(new ProjectReport(
                path,
                collected.Tfm,
                status,
                status == Status.Partial ? PartialReason(collected, projectFailures, fullRoot) : "",
                collected.UnresolvedSymbols,
                status == Status.Complete ? collected.UnusedReferences : null,
                collected.IvtDeclarations,
                collected.LinkedFiles,
                collected.Files));
        }

        // A project the workspace could not add at all: not a C# project (the
        // workspace has no language for it), or a C# project that did not open.
        foreach (var (project, messages) in failures)
        {
            if (loaded.Contains(project) || Relative(fullRoot, project) is not { } path)
            {
                continue;
            }
            if (path.EndsWith(".csproj", StringComparison.OrdinalIgnoreCase))
            {
                reports.Add(Failed(path, messages[0], fullRoot));
            }
            else
            {
                skipped.Add(path);
            }
        }

        var reported = reports.Select(r => r.Path).Concat(skipped).ToHashSet(StringComparer.Ordinal);
        foreach (var target in requested ?? [])
        {
            if (Relative(fullRoot, target) is { } path && !reported.Contains(path))
            {
                reports.Add(Failed(path, unattributed ?? NotLoaded, fullRoot));
            }
        }

        reports.Sort((a, b) =>
        {
            var c = string.CompareOrdinal(a.Path, b.Path);
            return c != 0 ? c : string.CompareOrdinal(a.Tfm, b.Tfm);
        });
        return new Report(Report.CurrentSchemaVersion, skipped.ToList(), reports);
    }

    private const string NoDocuments = "the workspace loaded no source documents";
    private const string NotLoaded = "the workspace did not load the project";

    private static ProjectReport Failed(string path, string message, string root) =>
        new(path, "", Status.Failed, FailedReason(message, root), 0, null, [], [], []);

    // FailingProject returns the project a workspace failure is about: the
    // first quoted project path in the message ("Msbuild failed when
    // processing the file '<path>' ..."). Later paths in the message name
    // other projects and do not count.
    internal static string? FailingProject(string message)
    {
        var match = QuotedProjectPath().Match(message);
        return match.Success ? Paths.Full(match.Groups[1].Value) : null;
    }

    internal static string FailedReason(string message, string root)
    {
        var (cause, fix) = Cause(message, root);
        return $"{cause}; {fix}";
    }

    internal static string PartialReason(Collected collected, IReadOnlyList<string> failures, string root)
    {
        var parts = new List<string>();
        var fixes = new List<string>();
        if (failures.Count > 0)
        {
            var (cause, fix) = Cause(failures[0], root);
            parts.Add(cause);
            fixes.Add(fix);
        }
        if (collected.UnresolvedSymbols > 0)
        {
            var ids = string.Join(", ", collected.UnresolvedById.OrderBy(kv => kv.Key, StringComparer.Ordinal).Select(kv => $"{kv.Key} x{kv.Value}"));
            var noun = collected.UnresolvedSymbols == 1 ? "symbol" : "symbols";
            parts.Add($"{collected.UnresolvedSymbols} unresolved {noun} ({ids})");
            fixes.Add(collected.UnresolvedById.ContainsKey("CS0518") && ReferencePack(collected.Tfm) is { } pack
                ? $"reference pack {pack.Name} {pack.Version} is not installed: install the .NET {pack.Version} targeting pack or restore with NuGet access"
                : "restore the project with NuGet access (`dotnet restore`) or install the missing references");
        }
        return string.Join("; ", parts.Concat(fixes.Distinct()));
    }

    // Cause names what a workspace failure says went wrong and the fix, with
    // the analysis root removed from paths.
    internal static (string Cause, string Fix) Cause(string message, string root)
    {
        if (MissingSdk().Match(message) is { Success: true } sdk)
        {
            return ($"MSB4236: SDK '{sdk.Groups[1].Value}' not resolvable", "run with NuGet access and a warm cache, or install the SDK");
        }
        var line = MSBuildPrefix().Replace(message, "")
            .Split('\n', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries)
            .FirstOrDefault() ?? message;
        line = line.Replace(root + Path.DirectorySeparatorChar, "", StringComparison.Ordinal);
        return message.Contains("global.json", StringComparison.OrdinalIgnoreCase)
            ? (line, "install the SDK that global.json pins or relax the pin")
            : (line, "fix the project so `dotnet build` loads it");
    }

    // ReferencePack names the framework reference pack a .NET (Core) TFM
    // needs, e.g. net8.0 -> "Microsoft.NETCore.App.Ref 8.0".
    internal static (string Name, string Version)? ReferencePack(string tfm)
    {
        var match = NetCoreTfm().Match(tfm);
        return match.Success ? ("Microsoft.NETCore.App.Ref", match.Groups[1].Value) : null;
    }

    private static string? Relative(string root, string path)
    {
        var full = Paths.Full(path);
        return Paths.IsUnder(root, full) ? Paths.Relative(root, full) : null;
    }

    [GeneratedRegex(@"'([^']+\.[A-Za-z]+proj)'")]
    private static partial Regex QuotedProjectPath();

    // "The SDK 'Name/1.0.0' specified could not be found" (MSB4236), or
    // "Could not resolve SDK \"Name\"" when a referenced project needs it.
    [GeneratedRegex(@"(?:The SDK '|resolve SDK "")([^'""/]+)")]
    private static partial Regex MissingSdk();

    [GeneratedRegex(@"^Msbuild failed when processing the file '[^']*' with message:\s*")]
    private static partial Regex MSBuildPrefix();

    [GeneratedRegex(@"^net(\d+\.\d+)(-.*)?$")]
    private static partial Regex NetCoreTfm();
}
