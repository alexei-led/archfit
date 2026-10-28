using System.Globalization;
using System.Reflection;
using Microsoft.Build.Locator;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.MSBuild;

namespace ArchfitRoslyn;

// archfit-roslyn loads C# projects through MSBuildWorkspace and prints one
// JSON report on stdout (Output.cs). It never restores. MSBuild design-time
// builds write under each project's obj/, so run it on a disposable copy of
// the tree (D6).
//
//   archfit-roslyn <root> <solution.slnx|solution.sln|filter.slnf|project.csproj>...
//   archfit-roslyn --version
internal static class Program
{
    private const string Usage = "usage: archfit-roslyn <root> <solution.slnx|solution.sln|filter.slnf|project.csproj>... | --version";

    public static async Task<int> Main(string[] args)
    {
        // Workspace and MSBuild messages feed reason strings; keep them stable.
        CultureInfo.DefaultThreadCurrentUICulture = CultureInfo.InvariantCulture;
        CultureInfo.CurrentUICulture = CultureInfo.InvariantCulture;
        try
        {
            if (args is ["--version"])
            {
                Console.WriteLine(Version(RegisterMSBuild(Environment.CurrentDirectory)));
                return 0;
            }
            if (args.Length < 2)
            {
                Console.Error.WriteLine(Usage);
                return 2;
            }
            var root = Paths.Full(args[0]);
            if (!Directory.Exists(root))
            {
                throw new UserError($"root {root} is not a directory");
            }
            var targets = args[1..].Select(t => Paths.Full(Path.Combine(root, t))).ToList();
            RegisterMSBuild(root);
            var report = await Loader.LoadAsync(root, targets, CancellationToken.None);
            Console.Out.Write(report.ToJson());
            Console.Out.WriteLine();
            return 0;
        }
        catch (UserError e)
        {
            Console.Error.WriteLine($"archfit-roslyn: {e.Message}");
            return 1;
        }
        catch (Exception e)
        {
            // One line for the caller's reason; the trace only on request.
            Console.Error.WriteLine(Unexpected(e));
            if (Environment.GetEnvironmentVariable("ARCHFIT_ROSLYN_DEBUG") == "1")
            {
                Console.Error.WriteLine(e);
            }
            return 2;
        }
    }

    internal static string Unexpected(Exception e) =>
        $"archfit-roslyn: {e.GetType().FullName}: {e.Message.Split('\n')[0].Trim()}";

    // RegisterMSBuild picks the SDK that `dotnet` resolves from dir, so a
    // global.json in the target applies, and loads MSBuild from it. It must run
    // before any MSBuild type is touched. An unresolvable SDK is one stderr
    // line (Sdk.Unresolvable), never a resolver stack trace.
    internal static VisualStudioInstance RegisterMSBuild(string dir)
    {
        Sdk.EnsureResolvable(dir);
        VisualStudioInstance? instance;
        try
        {
            instance = MSBuildLocator.QueryVisualStudioInstances(new VisualStudioInstanceQueryOptions
            {
                DiscoveryTypes = DiscoveryType.DotNetSdk,
                WorkingDirectory = dir,
            }).FirstOrDefault();
        }
        catch (InvalidOperationException e)
        {
            throw new UserError(Sdk.Unresolvable(e.Message, dir));
        }
        if (instance is null)
        {
            throw new UserError($"no .NET SDK resolvable from {dir}; install the .NET SDK, or the SDK that {dir}/global.json pins");
        }
        MSBuildLocator.RegisterInstance(instance);
        return instance;
    }

    private static string Version(VisualStudioInstance sdk) =>
        $"archfit-roslyn {InformationalVersion(typeof(Program).Assembly)}; " +
        $"Microsoft.CodeAnalysis {Metadata("RoslynVersion")}; " +
        $"SDK {sdk.Version}";

    private static string InformationalVersion(Assembly assembly) =>
        assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion ?? "unknown";

    private static string Metadata(string key) =>
        typeof(Program).Assembly.GetCustomAttributes<AssemblyMetadataAttribute>().FirstOrDefault(a => a.Key == key)?.Value ?? "unknown";
}

internal sealed class UserError(string message) : Exception(message);

internal static class Loader
{
    public static async Task<Report> LoadAsync(string root, IReadOnlyList<string> targets, CancellationToken ct)
    {
        foreach (var target in targets)
        {
            if (!File.Exists(target))
            {
                throw new UserError($"{target} does not exist");
            }
        }
        var solutions = targets.Count(IsSolution);
        if (solutions > 1 || (solutions == 1 && targets.Count > 1))
        {
            throw new UserError("pass one solution, or one or more project files");
        }

        using var workspace = MSBuildWorkspace.Create();
        var diagnostics = new List<WorkspaceDiagnostic>();
        workspace.RegisterWorkspaceFailedHandler(e =>
        {
            lock (diagnostics)
            {
                diagnostics.Add(e.Diagnostic);
            }
            if (e.Diagnostic.Kind == WorkspaceDiagnosticKind.Failure
                && !Classifier.IsCrossTargetingFailure(e.Diagnostic.Message))
            {
                Console.Error.WriteLine($"archfit-roslyn: workspace failure: {e.Diagnostic.Message}");
            }
        });

        if (solutions == 1)
        {
            await workspace.OpenSolutionAsync(targets[0], cancellationToken: ct);
        }
        else
        {
            foreach (var target in targets)
            {
                // A project opened earlier as a reference is already loaded.
                if (!workspace.CurrentSolution.Projects.Any(p => p.FilePath is { } path && Paths.Full(path) == target))
                {
                    await workspace.OpenProjectAsync(target, cancellationToken: ct);
                }
            }
        }

        List<WorkspaceDiagnostic> snapshot;
        lock (diagnostics)
        {
            snapshot = [.. diagnostics];
        }
        IReadOnlyCollection<string> requested = solutions == 1 ? [] : targets;
        return await Classifier.ClassifyAsync(workspace.CurrentSolution, root, snapshot, ct, requested);
    }

    // A solution filter (.slnf) opens like a solution: MSBuildWorkspace loads
    // its base solution and keeps the projects the filter lists.
    private static bool IsSolution(string path) =>
        path.EndsWith(".sln", StringComparison.OrdinalIgnoreCase)
        || path.EndsWith(".slnx", StringComparison.OrdinalIgnoreCase)
        || path.EndsWith(".slnf", StringComparison.OrdinalIgnoreCase);
}
