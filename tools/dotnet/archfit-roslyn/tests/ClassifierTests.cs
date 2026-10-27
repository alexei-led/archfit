using Microsoft.CodeAnalysis;

namespace ArchfitRoslyn.Tests;

public class ClassifierTests
{
    private const string Root = "/snap";

    [Theory]
    [InlineData("Msbuild failed when processing the file '/snap/A/A.csproj' with message: Project '../B/B.csproj' targets 'net10.0'.", "/snap/A/A.csproj")]
    [InlineData("Msbuild failed when processing the file '/snap/Sql/Sql.sqlproj' with message: x", "/snap/Sql/Sql.sqlproj")]
    [InlineData("Found project reference without a matching metadata reference: /snap/B/B.csproj", null)]
    [InlineData("no path here", null)]
    public void FailingProjectIsTheFirstQuotedProjectPath(string message, string? project)
    {
        Assert.Equal(project, Classifier.FailingProject(message));
    }

    [Theory]
    [InlineData(
        "Msbuild failed when processing the file '/snap/B/B.csproj' with message: The SDK 'Bitwarden.Server.Sdk/1.8.0' specified could not be found.  /snap/B/B.csproj",
        "MSB4236: SDK 'Bitwarden.Server.Sdk' not resolvable; run with NuGet access and a warm cache, or install the SDK")]
    [InlineData(
        "Msbuild failed when processing the file '/snap/T/T.csproj' with message: Could not resolve SDK \"Aspire.AppHost.Sdk\". Exactly one of the probing messages below indicates why.",
        "MSB4236: SDK 'Aspire.AppHost.Sdk' not resolvable; run with NuGet access and a warm cache, or install the SDK")]
    [InlineData(
        "Msbuild failed when processing the file '/snap/A/A.csproj' with message: A compatible .NET SDK was not found. Requested SDK version: 9.0.100\nglobal.json file: /snap/global.json",
        "A compatible .NET SDK was not found. Requested SDK version: 9.0.100; install the SDK that global.json pins or relax the pin")]
    [InlineData(
        "Msbuild failed when processing the file '/snap/A/A.csproj' with message: /snap/A/A.csproj(3,5): error MSB4025: bad XML\nsecond line",
        "A/A.csproj(3,5): error MSB4025: bad XML; fix the project so `dotnet build` loads it")]
    public void FailedReasonNamesCauseAndFix(string message, string reason)
    {
        Assert.Equal(reason, Classifier.FailedReason(message, Root));
    }

    // Messages as MSBuildWorkspace reports them for a .slnx entry it cannot add.
    [Fact]
    public async Task ProjectsTheWorkspaceCouldNotAddAreSkippedOrFailed()
    {
        using var workspace = new AdhocWorkspace();
        WorkspaceDiagnostic[] diagnostics =
        [
            new(WorkspaceDiagnosticKind.Failure, "Cannot open project '/snap/Sql/Sql.sqlproj' because the file extension '.sqlproj' is not associated with a language."),
            new(WorkspaceDiagnosticKind.Failure, "Project file not found: '/snap/Gone/Gone.csproj'"),
            new(WorkspaceDiagnosticKind.Warning, "Found project reference without a matching metadata reference: '/snap/Ref/Ref.csproj'"),
        ];

        var report = await Classifier.ClassifyAsync(workspace.CurrentSolution, Root, diagnostics, CancellationToken.None);

        Assert.Equal(["Sql/Sql.sqlproj"], report.SkippedProjects);
        var gone = Assert.Single(report.Projects);
        Assert.Equal(
            ("Gone/Gone.csproj", "failed", "Project file not found: 'Gone/Gone.csproj'; fix the project so `dotnet build` loads it"),
            (gone.Path, gone.Status, gone.Reason));
    }

    [Fact]
    public async Task RequestedProjectsAreNeverOmitted()
    {
        using var workspace = new AdhocWorkspace();
        var solution = workspace.CurrentSolution.AddProject(ProjectInfo.Create(
            ProjectId.CreateNewId(), VersionStamp.Default, "Empty", "Empty", LanguageNames.CSharp, filePath: "/snap/Empty/Empty.csproj"));
        WorkspaceDiagnostic[] diagnostics =
        [
            new(WorkspaceDiagnosticKind.Failure, "Msbuild failed with a message that names no project file"),
        ];

        var report = await Classifier.ClassifyAsync(
            solution, Root, diagnostics, CancellationToken.None, ["/snap/Empty/Empty.csproj", "/snap/Dropped/Dropped.csproj"]);

        Assert.Equal(
            [
                ("Dropped/Dropped.csproj", "failed", "Msbuild failed with a message that names no project file; fix the project so `dotnet build` loads it"),
                ("Empty/Empty.csproj", "failed", "Msbuild failed with a message that names no project file; fix the project so `dotnet build` loads it"),
            ],
            report.Projects.Select(p => (p.Path, p.Status, p.Reason)));
    }

    [Fact]
    public async Task RequestedProjectWithoutAFailureIsFailedNotLoaded()
    {
        using var workspace = new AdhocWorkspace();

        var report = await Classifier.ClassifyAsync(workspace.CurrentSolution, Root, [], CancellationToken.None, ["/snap/Gone/Gone.csproj"]);

        var gone = Assert.Single(report.Projects);
        Assert.Equal(
            ("Gone/Gone.csproj", "failed", "the workspace did not load the project; fix the project so `dotnet build` loads it"),
            (gone.Path, gone.Status, gone.Reason));
    }

    [Theory]
    [InlineData("net8.0", "Microsoft.NETCore.App.Ref", "8.0")]
    [InlineData("net10.0", "Microsoft.NETCore.App.Ref", "10.0")]
    [InlineData("net8.0-windows", "Microsoft.NETCore.App.Ref", "8.0")]
    public void ReferencePackForNetTfm(string tfm, string name, string version)
    {
        Assert.Equal((name, version), Classifier.ReferencePack(tfm));
    }

    [Theory]
    [InlineData("netstandard2.0")]
    [InlineData("net48")]
    [InlineData("")]
    public void NoReferencePackForOtherTfms(string tfm)
    {
        Assert.Null(Classifier.ReferencePack(tfm));
    }

    [Fact]
    public void PartialReasonNamesTheMissingReferencePack()
    {
        var collected = new Collected("net8.0", 12, new Dictionary<string, int> { ["CS0518"] = 10, ["CS0246"] = 2 }, [], [], [], []);
        Assert.Equal(
            "12 unresolved symbols (CS0246 x2, CS0518 x10); reference pack Microsoft.NETCore.App.Ref 8.0 is not installed: install the .NET 8.0 targeting pack or restore with NuGet access",
            Classifier.PartialReason(collected, [], Root));
    }
}
