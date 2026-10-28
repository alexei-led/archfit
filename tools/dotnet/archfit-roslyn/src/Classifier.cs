using System.Text.RegularExpressions;
using Microsoft.CodeAnalysis;

namespace ArchfitRoslyn;

// Classifier turns a loaded solution plus workspace diagnostics into the
// report, deciding each attempt's status (D5):
//   failed   no source documents: nothing loaded to measure
//   partial  documents loaded, and a name did not resolve or the compilation
//            has an error
//   complete documents loaded, every name resolved, no compile error
// One entry per (project, target framework). Status and reason belong to the
// attempt: a workspace failure on one attempt never decides another's, and a
// failure that left no documents is not evidence that a loaded one is
// incomplete. Every requested project gets an entry: one the workspace dropped,
// or whose failure names no parsable project path, is failed, never omitted.
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

        // MSBuildWorkspace reports a multi-targeted project once per target
        // framework, and its framework-less evaluation as one more project when
        // that evaluation fails. Grouping the attempts by project file tells
        // that evaluation apart from a framework's own attempt.
        var attempts = new List<Attempt>();
        var loaded = new HashSet<string>(StringComparer.Ordinal);
        foreach (var project in solution.Projects)
        {
            if (project.FilePath is null || Relative(fullRoot, project.FilePath) is not { } path)
            {
                continue;
            }
            var full = Paths.Full(project.FilePath);
            loaded.Add(full);
            attempts.Add(new Attempt(project, path, full, Collector.Tfm(project)));
        }

        // The framework-less evaluation of a multi-targeted project is not a
        // measurement: MSBuild's cross-targeting build imports no Compile
        // target, so it loads no documents and its failure says nothing about
        // the frameworks, which load separately. Omit it, and its failure, so
        // it neither contradicts nor marks the framework entries.
        var crossTargeting = attempts
            .Where(a => a.Tfm == "" && a.Project.DocumentIds.Count == 0
                && attempts.Any(o => o.FullPath == a.FullPath && o.Tfm != ""))
            .Select(a => a.Project.Id)
            .ToHashSet();
        var crossTargetingPaths = attempts
            .Where(a => crossTargeting.Contains(a.Project.Id))
            .Select(a => a.FullPath)
            .ToHashSet(StringComparer.Ordinal);

        var collector = new Collector(solution, fullRoot);
        var skipped = new SortedSet<string>(StringComparer.Ordinal);
        var reports = new List<ProjectReport>();

        foreach (var attempt in attempts)
        {
            if (attempt.Project.Language != LanguageNames.CSharp)
            {
                skipped.Add(attempt.Path);
                continue;
            }
            if (crossTargeting.Contains(attempt.Project.Id))
            {
                continue;
            }
            var projectFailures = ProjectFailures(attempt.FullPath, failures, crossTargetingPaths);
            if (attempt.Project.DocumentIds.Count == 0)
            {
                // A project whose evaluation failed stays in the workspace with
                // no documents; the failure may not name it parsably.
                reports.Add(Failed(attempt.Path, attempt.Tfm, projectFailures.FirstOrDefault() ?? unattributed ?? NoDocuments, fullRoot));
                continue;
            }
            var (collected, failure) = await CollectGuarded(collector, attempt.Project, ct);
            if (collected is null)
            {
                reports.Add(new ProjectReport(
                    attempt.Path, attempt.Tfm, Status.Partial,
                    $"the compiler threw while analysing the project ({failure}), so its file edges are unknown; restore the project with NuGet access (`dotnet restore`) or install the missing references",
                    0, [], [], [], []));
                continue;
            }
            // The attempt's own compilation decides. A workspace failure names
            // the project, not the evaluation it belongs to, so it is the cause
            // of a framework that is already partial; it never marks a
            // framework whose every name resolved and which has no compile
            // error. A failure that lost evidence leaves no documents at all,
            // which is the failed branch above.
            var status = collected.UnresolvedSymbols == 0 && collected.CompileErrors == 0
                && collected.Unreadable.Count == 0 && collected.DiagnosticsFailure == ""
                ? Status.Complete
                : Status.Partial;
            reports.Add(new ProjectReport(
                attempt.Path,
                collected.Tfm,
                status,
                status == Status.Complete ? "" : PartialReason(collected, projectFailures, fullRoot),
                collected.UnresolvedSymbols,
                collected.UsedProjects,
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
                reports.Add(Failed(path, "", messages[0], fullRoot));
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
                reports.Add(Failed(path, "", unattributed ?? NotLoaded, fullRoot));
            }
        }

        reports.Sort((a, b) =>
        {
            var c = string.CompareOrdinal(a.Path, b.Path);
            return c != 0 ? c : string.CompareOrdinal(a.Tfm, b.Tfm);
        });
        return new Report(Report.CurrentSchemaVersion, skipped.ToList(), reports);
    }

    private sealed record Attempt(Project Project, string Path, string FullPath, string Tfm);

    // CollectGuarded keeps one project's failure from ending the run: the
    // collector already guards each file, so this catches what is left (the
    // compilation, its diagnostics, a project's metadata) and names it.
    private static async Task<(Collected? Collected, string Failure)> CollectGuarded(
        Collector collector, Project project, CancellationToken ct)
    {
        try
        {
            return (await collector.CollectAsync(project, ct), "");
        }
        catch (Exception e) when (e is not OperationCanceledException)
        {
            return (null, e.GetType().Name);
        }
    }

    // roslynThrewFix is the fix of a Roslyn fault: the nullable flow analysis
    // dereferences an unresolved type, so an incomplete reference set causes it.
    private const string roslynThrewFix =
        "Roslyn threw while binding the project, which an incomplete reference set causes: restore the project with NuGet access (`dotnet restore`) or install the missing references";

    private const string NoDocuments = "the workspace loaded no source documents";
    private const string NotLoaded = "the workspace did not load the project";

    // IsCrossTargetingFailure reports MSBuild's failure to build the
    // framework-less evaluation of a multi-targeted project. Its cross-targeting
    // build defines no Compile target, so the helper never treats it as a
    // measurement of the project.
    internal static bool IsCrossTargetingFailure(string message) => CrossTargetingFailure().IsMatch(message);

    // ProjectFailures are the workspace failures about one project file. The
    // cross-targeting evaluation's failure is dropped for a multi-targeted
    // project: the helper does not report that evaluation, so its failure is
    // not a cause of anything the caller reads.
    private static IReadOnlyList<string> ProjectFailures(
        string fullPath, IReadOnlyDictionary<string, List<string>> failures, HashSet<string> crossTargetingPaths)
    {
        var messages = failures.GetValueOrDefault(fullPath) ?? [];
        return crossTargetingPaths.Contains(fullPath)
            ? messages.Where(m => !IsCrossTargetingFailure(m)).ToList()
            : messages;
    }

    private static ProjectReport Failed(string path, string tfm, string message, string root) =>
        new(path, tfm, Status.Failed, FailedReason(message, root), 0, [], [], [], []);

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

    // PartialReason names why a loaded attempt is partial, and is empty when it
    // is complete: the attempt's own compilation decides, not another
    // attempt's failure.
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
            var noun = collected.UnresolvedSymbols == 1 ? "symbol" : "symbols";
            parts.Add($"{collected.UnresolvedSymbols} unresolved {noun} ({Ids(collected.UnresolvedById)})");
            fixes.Add(collected.UnresolvedById.ContainsKey("CS0518") && ReferencePack(collected.Tfm) is { } pack
                ? $"reference pack {pack.Name} {pack.Version} is not installed: install the .NET {pack.Version} targeting pack or restore with NuGet access"
                : "restore the project with NuGet access (`dotnet restore`) or install the missing references");
        }
        if (collected.CompileErrors > 0)
        {
            var noun = collected.CompileErrors == 1 ? "compile error" : "compile errors";
            parts.Add($"{collected.CompileErrors} {noun} ({Ids(collected.CompileErrorsById)})");
            fixes.Add("fix the compile errors: a file that does not compile keeps only the edges it resolved");
        }
        if (collected.Unreadable.Count > 0)
        {
            var noun = collected.Unreadable.Count == 1 ? "file" : "files";
            parts.Add($"{collected.Unreadable.Count} {noun} could not be analysed ({ListIds(collected.Unreadable)})");
            fixes.Add(roslynThrewFix);
        }
        if (collected.DiagnosticsFailure != "")
        {
            parts.Add($"the compiler failed to report diagnostics ({collected.DiagnosticsFailure})");
            fixes.Add(roslynThrewFix);
        }
        return string.Join("; ", parts.Concat(fixes.Distinct()));
    }

    // ListIds names at most five items of a list, then counts the rest: a
    // reason keeps which units are affected, not every one.
    internal static string ListIds(IReadOnlyList<string> items)
    {
        const int limit = 5;
        var sorted = items.Order(StringComparer.Ordinal).ToList();
        return sorted.Count <= limit
            ? string.Join(", ", sorted)
            : string.Join(", ", sorted.Take(limit)) + $", and {sorted.Count - limit} more";
    }

    private static string Ids(IReadOnlyDictionary<string, int> byId) =>
        string.Join(", ", byId.OrderBy(kv => kv.Key, StringComparer.Ordinal).Select(kv => $"{kv.Key} x{kv.Value}"));

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

    // The failure of MSBuild's cross-targeting build: its targets define no
    // Compile target, which is why a multi-targeted project's framework-less
    // evaluation loads no documents (MSB4057).
    [GeneratedRegex(@"does not contain 'Compile' target", RegexOptions.IgnoreCase)]
    private static partial Regex CrossTargetingFailure();

    [GeneratedRegex(@"^net(\d+\.\d+)(-.*)?$")]
    private static partial Regex NetCoreTfm();
}
