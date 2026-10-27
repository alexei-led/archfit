using Microsoft.Build.Locator;

namespace ArchfitRoslyn.Tests;

// LoadedFixture copies internal/testdata/dotnet to a temp directory (the
// design-time build writes obj/, D6), loads it once, and deletes the copy.
public sealed class LoadedFixture : IAsyncLifetime
{
    public static readonly string Source = FindSource();

    public string Copy { get; } = Path.Combine(Path.GetTempPath(), "archfit-roslyn-fixture-" + Guid.NewGuid().ToString("N"));

    internal Report Report { get; private set; } = null!;

    public async Task InitializeAsync()
    {
        CopyTree(Source, Copy);
        if (!MSBuildLocator.IsRegistered)
        {
            Program.RegisterMSBuild(Copy);
        }
        Report = await Loader.LoadAsync(Paths.Full(Copy), [Paths.Full(Path.Combine(Copy, "Fixture.slnx"))], CancellationToken.None);
    }

    public Task DisposeAsync()
    {
        Directory.Delete(Copy, recursive: true);
        return Task.CompletedTask;
    }

    internal static void CopyTree(string from, string to)
    {
        foreach (var dir in Directory.EnumerateDirectories(from, "*", SearchOption.AllDirectories))
        {
            Directory.CreateDirectory(Path.Combine(to, Path.GetRelativePath(from, dir)));
        }
        Directory.CreateDirectory(to);
        foreach (var file in Directory.EnumerateFiles(from, "*", SearchOption.AllDirectories))
        {
            File.Copy(file, Path.Combine(to, Path.GetRelativePath(from, file)));
        }
    }

    private static string FindSource()
    {
        for (var dir = new DirectoryInfo(AppContext.BaseDirectory); dir is not null; dir = dir.Parent)
        {
            var candidate = Path.Combine(dir.FullName, "internal", "testdata", "dotnet");
            if (File.Exists(Path.Combine(candidate, "Fixture.slnx")))
            {
                return candidate;
            }
        }
        throw new DirectoryNotFoundException("internal/testdata/dotnet not found above " + AppContext.BaseDirectory);
    }
}

// Expectations come from internal/testdata/dotnet/README.md.
public class FixtureTests(LoadedFixture fixture) : IClassFixture<LoadedFixture>
{
    private Report Report => fixture.Report;

    [Theory]
    [InlineData("Application/Application.csproj", "net10.0", "complete")]
    [InlineData("Domain/Domain.csproj", "net10.0", "complete")]
    [InlineData("Infrastructure/Infrastructure.csproj", "net10.0", "complete")]
    [InlineData("Legacy/Legacy.csproj", "net10.0", "complete")]
    [InlineData("Broken/Broken.csproj", "", "failed")]
    [InlineData("Tests/Tests.csproj", "net10.0", "partial")]
    [InlineData("Unresolved/Unresolved.csproj", "net10.0", "partial")]
    public void StatusPerProject(string path, string tfm, string status)
    {
        Assert.Equal(status, Project(path, tfm).Status);
    }

    [Fact]
    public void ReportsEveryProjectOncePerTargetFramework()
    {
        Assert.Equal(
            [
                "Application/Application.csproj@net10.0", "Broken/Broken.csproj@", "Domain/Domain.csproj@net10.0",
                "Infrastructure/Infrastructure.csproj@net10.0", "Legacy/Legacy.csproj@net10.0", "Legacy/Legacy.csproj@net8.0",
                "Tests/Tests.csproj@net10.0", "Unresolved/Unresolved.csproj@net10.0",
            ],
            Report.Projects.Select(p => $"{p.Path}@{p.Tfm}"));
        Assert.Equal(Report.CurrentSchemaVersion, Report.SchemaVersion);
        Assert.Empty(Report.SkippedProjects);
    }

    // The host may lack the 8.0 targeting pack; the image always does (D7).
    [Fact]
    public void LegacyNet8IsCompleteOrPartialNamingTheReferencePack()
    {
        var legacy = Project("Legacy/Legacy.csproj", "net8.0");
        if (legacy.Status == Status.Complete)
        {
            return;
        }
        Assert.Equal(Status.Partial, legacy.Status);
        Assert.Matches(@"Microsoft\.NETCore\.App\.Ref 8\.\d", legacy.Reason);
        Assert.DoesNotContain("Msbuild failed", legacy.Reason);
    }

    [Fact]
    public void FailedAndPartialReasonsNameCauseAndFix()
    {
        var broken = Project("Broken/Broken.csproj", "");
        Assert.Equal(
            "MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; run with NuGet access and a warm cache, or install the SDK",
            broken.Reason);
        Assert.Empty(broken.Files);

        var tests = Project("Tests/Tests.csproj", "net10.0");
        Assert.StartsWith("MSB4236: SDK 'Archfit.Fixture.Missing.Sdk' not resolvable; 1 unresolved symbol (CS0234 x1)", tests.Reason);
        Assert.DoesNotContain(fixture.Copy, tests.Reason);
    }

    [Fact]
    public void UnresolvedPackageIsCountedWithoutNodes()
    {
        var project = Project("Unresolved/Unresolved.csproj", "net10.0");
        Assert.Equal(3, project.UnresolvedSymbols);
        Assert.Empty(Assert.Single(project.Files).Edges);
    }

    [Fact]
    public void RepositoryEdgesAggregatePerFilePair()
    {
        Assert.Equal(
            [
                new EdgeReport("Application/Contracts.cs", false, "dto", "dto", "type_use", 6),
                new EdgeReport("Domain/Domain.cs", false, "intrusive", "model", "type_use", 6),
            ],
            FileIn("Infrastructure/Infrastructure.csproj", "Infrastructure/Repository.cs").Edges);
    }

    [Fact]
    public void ApplicationUsesDomainThroughTheGlobalUsing()
    {
        Assert.Equal(
            [new EdgeReport("Domain/Domain.cs", false, "model", "model", "type_use", 5)],
            FileIn("Application/Application.csproj", "Application/Contracts.cs").Edges);
        Assert.Equal(["Domain/Domain.csproj"], Project("Application/Application.csproj", "net10.0").UsedProjects);
        Assert.Equal(["Application/Application.csproj", "Domain/Domain.csproj"], Project("Infrastructure/Infrastructure.csproj", "net10.0").UsedProjects);
    }

    [Fact]
    public void TfmOnlyEdgeAppearsInItsTargetFramework()
    {
        Assert.Equal(
            [new EdgeReport("Domain/Domain.cs", false, "functional", "model", "call", 6)],
            Project("Legacy/Legacy.csproj", "net10.0").Files.Single(f => f.Path == "Legacy/Legacy.cs").Edges);
        Assert.Empty(Project("Legacy/Legacy.csproj", "net8.0").Files.Single(f => f.Path == "Legacy/Legacy.cs").Edges);
    }

    [Fact]
    public void GeneratedLinkedAndIvtDetail()
    {
        var application = Project("Application/Application.csproj", "net10.0");
        Assert.Equal(
            [("Application/Contracts.cs", false), ("Application/Generated/Mapper.g.cs", true), ("Application/GlobalUsings.cs", false), ("Shared/Marker.cs", false)],
            application.Files.Select(f => (f.Path, f.Generated)));
        Assert.Equal(["Shared/Marker.cs"], application.LinkedFiles);
        Assert.Equal(
            [new IvtDeclaration("Infrastructure", "Domain/Domain.csproj")],
            Project("Domain/Domain.csproj", "net10.0").IvtDeclarations);
    }

    [Fact]
    public void LeavesTheSourceFixtureUntouched()
    {
        Assert.Empty(Directory.EnumerateDirectories(LoadedFixture.Source, "obj", SearchOption.AllDirectories));
        Assert.Empty(Directory.EnumerateDirectories(LoadedFixture.Source, "bin", SearchOption.AllDirectories));
    }

    private ProjectReport Project(string path, string tfm) => Report.Projects.Single(p => p.Path == path && p.Tfm == tfm);

    private FileReport FileIn(string project, string file) =>
        Report.Projects.Single(p => p.Path == project).Files.Single(f => f.Path == file);
}
