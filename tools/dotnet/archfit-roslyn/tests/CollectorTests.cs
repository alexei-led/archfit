using System.Text;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.Text;

namespace ArchfitRoslyn.Tests;

// In-memory solutions: no MSBuild, no generated IVT attribute, exact control
// over which assemblies see which internals.
public class CollectorTests
{
    private static readonly string Root = Path.Combine(Path.GetTempPath(), "archfit-roslyn-adhoc");

    private const string Lib = """
        namespace L;
        public interface IPort { void Send(); }
        public abstract class Rule { public abstract bool Allows(); }
        public class Customer { public string Name = ""; public int Age { get; set; } public void Touch() { } }
        public record CustomerDto(string Name, int Age);
        public readonly record struct Key(int Value);
        public class Bag { public int this[int i] => i; }
        public static class Extensions { public static string Shout(this string s) => s; }
        internal static class Secret { internal static int Seed = 1; }
        public class Base { protected void Hidden() { } private protected void Narrow() { } }
        """;

    [Theory]
    [InlineData("implement", "class U : L.IPort { public void Send() { } }", "contract", "")]
    [InlineData("abstract member call", "class U { bool M(L.Rule r) => r.Allows(); }", "contract", "")]
    [InlineData("concrete field read", "class U { string M(L.Customer c) => c.Name; }", "model", "model")]
    [InlineData("auto-property read", "class U { int M(L.Customer c) => c.Age; }", "model", "model")]
    [InlineData("record property read", "class U { string M(L.CustomerDto d) => d.Name; }", "dto", "dto")]
    [InlineData("record struct property read", "class U { int M(L.Key k) => k.Value; }", "dto", "dto")]
    [InlineData("indexer call", "class U { int M(L.Bag b) => b[0]; }", "functional", "model")]
    [InlineData("concrete method call", "class U { void M(L.Customer c) => c.Touch(); }", "functional", "model")]
    [InlineData("extension call only", "using L; class U { string M() => \"x\".Shout(); }", "functional", "")]
    [InlineData("constructor call", "class U { object M() => new L.Customer(); }", "functional", "model")]
    [InlineData("concrete inheritance", "class U : L.Customer { }", "functional", "")]
    [InlineData("internal without IVT", "class U { int M() => L.Secret.Seed; }", "intrusive", "")]
    [InlineData("private protected without IVT", "class U : L.Base { void M() => Narrow(); }", "intrusive", "")]
    public async Task EdgeHintsPerUse(string name, string source, string hint, string dataHint)
    {
        var report = await Analyze(
            ("Lib", [("Lib/Lib.cs", Lib)], []),
            ("App", [("App/U.cs", source)], ["Lib"]));

        var edge = Assert.Single(Edges(report, "App/U.cs"));
        Assert.Equal((name, "Lib/Lib.cs", false, hint, dataHint), (name, edge.To, edge.External, edge.Hint, edge.DataHint));
    }

    [Fact]
    public async Task InaccessibleProtectedMembersAreNotIntrusive()
    {
        var report = await Analyze(
            ("Lib", [("Lib/Lib.cs", Lib)], []),
            ("App", [("App/U.cs", "class U { void M(L.Base b) => b.Hidden(); }")], ["Lib"]));

        var edge = Assert.Single(Edges(report, "App/U.cs"));
        Assert.Equal("model", edge.Hint);
    }

    [Fact]
    public async Task StrongestHintWinsPerChannelIndependently()
    {
        var report = await Analyze(
            ("Lib", [("Lib/Lib.cs", Lib)], []),
            ("App", [("App/U.cs", """
                class U : L.IPort
                {
                    public void Send() { }
                    string M(L.CustomerDto d) => d.Name;
                    void N(L.Customer c) => c.Touch();
                }
                """)], ["Lib"]));

        var edge = Assert.Single(Edges(report, "App/U.cs"));
        Assert.Equal(("functional", "model", "call"), (edge.Hint, edge.DataHint, edge.Reference));
    }

    [Fact]
    public async Task EdgesTargetTheDeclaringFile()
    {
        var report = await Analyze(
            ("Lib", [("Lib/Port.cs", "namespace L; public interface IPort { }"), ("Lib/Customer.cs", "namespace L; public class Customer { }")], []),
            ("App", [("App/U.cs", "class U { void M(L.IPort p, L.Customer c) { } }"), ("App/V.cs", "class V { U u = new(); }")], ["Lib"]));

        Assert.Equal(
            [("Lib/Customer.cs", "model"), ("Lib/Port.cs", "contract")],
            Edges(report, "App/U.cs").Select(e => (e.To, e.Hint)));
        Assert.Equal([("App/U.cs", "functional")], Edges(report, "App/V.cs").Select(e => (e.To, e.Hint)));
    }

    [Fact]
    public async Task MetadataSymbolsBecomeExternalAssemblyEdges()
    {
        var report = await Analyze(("App", [("App/U.cs", "class U { System.Text.StringBuilder b = new(); }")], []));

        var edge = Assert.Single(Edges(report, "App/U.cs"));
        Assert.Equal((typeof(StringBuilder).Assembly.GetName().Name, true, "functional"), (edge.To, edge.External, edge.Hint));
    }

    [Fact]
    public async Task UnresolvedSymbolsAreCountedAndNotEmitted()
    {
        var report = await Analyze(("App", [("App/U.cs", "using Missing; class U { Widget W() => new Widget(); }")], []));

        var project = Assert.Single(report.Projects);
        Assert.Equal((Status.Partial, 3), (project.Status, project.UnresolvedSymbols));
        Assert.Contains("3 unresolved symbols (CS0246 x3)", project.Reason);
        Assert.Empty(Edges(report, "App/U.cs"));
    }

    [Theory]
    [InlineData("CS1061", "class U { string M(string s) => s.FromMissingPackage(); }")]
    [InlineData("CS0117", "class U { int M() => string.NoSuchMember; }")]
    public async Task MissingMemberDiagnosticsMakeTheProjectPartial(string id, string source)
    {
        var report = await Analyze(("App", [("App/U.cs", source)], []));

        var project = Assert.Single(report.Projects);
        Assert.Equal((Status.Partial, 1), (project.Status, project.UnresolvedSymbols));
        Assert.Contains($"{id} x1", project.Reason);
    }

    // A cross-targeting build imports no Compile target, so MSBuildWorkspace
    // keeps the framework-less evaluation of a multi-targeted project as an
    // empty project and reports its failure. It measured nothing: it is not an
    // entry, and it does not mark the frameworks complete.
    [Fact]
    public async Task TheFrameworklessEvaluationIsNotReportedAndDoesNotMarkAFramework()
    {
        var report = await AnalyzeAttempts(
            [Failed(MultiProject, "Project does not contain 'Compile' target.")],
            ("Multi", ""),
            ("Multi(net10.0)", "class A { }"),
            ("Multi(net8.0)", "class B { }"));

        Assert.Equal(
            [("Multi/Multi.csproj", "net10.0", Status.Complete), ("Multi/Multi.csproj", "net8.0", Status.Complete)],
            report.Projects.Select(p => (p.Path, p.Tfm, p.Status)));
        Assert.All(report.Projects, p => Assert.Equal("", p.Reason));
    }

    // Each framework carries its own failure: the framework that loaded a
    // clean compilation is complete even though another framework of the same
    // project did not load, and neither reason borrows the other's cause.
    [Fact]
    public async Task AFrameworksFailureDoesNotMakeASiblingPartial()
    {
        var report = await AnalyzeAttempts(
            [
                Failed(MultiProject, "Project does not contain 'Compile' target."),
                Failed(MultiProject, "The TargetFramework value 'bogus1.0' was not recognized."),
            ],
            ("Multi", ""),
            ("Multi(net10.0)", "class A { }"),
            ("Multi(bogus1.0)", "class B { Missing m; }"));

        var net10 = report.Projects.Single(p => p.Tfm == "net10.0");
        Assert.Equal((Status.Complete, ""), (net10.Status, net10.Reason));
        var bogus = report.Projects.Single(p => p.Tfm == "bogus1.0");
        Assert.Equal(Status.Partial, bogus.Status);
        Assert.Contains("The TargetFramework value 'bogus1.0' was not recognized", bogus.Reason);
        Assert.DoesNotContain("'Compile' target", bogus.Reason);
    }

    // A framework that loaded no documents is failed, and it carries its own
    // framework: the caller names the framework whose evidence is missing
    // instead of the whole project.
    [Fact]
    public async Task AFrameworkThatLoadedNothingIsFailedWithItsOwnReason()
    {
        var report = await AnalyzeAttempts(
            [Failed(MultiProject, "The TargetFramework value 'missing1.0' was not recognized.")],
            ("Multi(net10.0)", "class A { }"),
            ("Multi(missing1.0)", ""));

        var missing = report.Projects.Single(p => p.Tfm == "missing1.0");
        Assert.Equal(Status.Failed, missing.Status);
        Assert.Contains("The TargetFramework value 'missing1.0' was not recognized", missing.Reason);
        Assert.Empty(missing.Files);
        Assert.Equal(Status.Complete, report.Projects.Single(p => p.Tfm == "net10.0").Status);
    }

    // A compilation that does not compile may have dropped edges, so an error
    // other than an unresolved name keeps the framework partial.
    [Fact]
    public async Task ACompileErrorMakesTheFrameworkPartial()
    {
        var report = await Analyze(("App", [("App/U.cs", "class U { void M() { int x = \"s\"; } }")], []));

        var project = Assert.Single(report.Projects);
        Assert.Equal(Status.Partial, project.Status);
        Assert.Contains("1 compile error (CS0029 x1)", project.Reason);
    }

    // The workspace's references include transitive ones (Core here); the
    // report names only the projects used, and the caller compares them with
    // the direct references it evaluated.
    [Fact]
    public async Task UsedProjectsNameOnlyTheProjectsWhoseSymbolsAreUsed()
    {
        var report = await Analyze(
            ("Core", [("Core/C.cs", "namespace C; public class Id { }")], []),
            ("Lib", [("Lib/Lib.cs", Lib)], ["Core"]),
            ("Other", [("Other/O.cs", "namespace O; public class X { }")], []),
            ("App", [("App/U.cs", "class U { L.Customer? c; }")], ["Lib", "Other", "Core"]));

        var app = report.Projects.Single(p => p.Path == "App/App.csproj");
        Assert.Equal(Status.Complete, app.Status);
        Assert.Equal(["Lib/Lib.csproj"], app.UsedProjects);
    }

    [Fact]
    public async Task EdgeLineIsTheFirstUseInTheFile()
    {
        var report = await Analyze(
            ("Lib", [("Lib/Lib.cs", Lib)], []),
            ("App", [("App/U.cs", "class U\n{\n    int M(L.Customer c) => c.Age;\n    void N(L.Customer c) => c.Touch();\n}")], ["Lib"]));

        var edge = Assert.Single(Edges(report, "App/U.cs"));
        Assert.Equal(("functional", 3), (edge.Hint, edge.Line));
    }

    [Fact]
    public async Task ReportsIvtAttributesLinkedAndGeneratedFiles()
    {
        var report = await Analyze(
            ("Lib", [
                ("Lib/Lib.cs", Lib),
                ("Lib/AssemblyInfo.cs", "[assembly: System.Runtime.CompilerServices.InternalsVisibleTo(\"App, PublicKey=00\")]"),
                ("Lib/Gen.cs", "// <auto-generated/>\nnamespace L; public class Gen { }"),
                ("Lib/View.Designer.cs", "namespace L; public class View { }"),
                ("Shared/Marker.cs", "namespace S; public static class Marker { }"),
            ], []),
            ("App", [("App/U.cs", "class U { int M() => L.Secret.Seed; }")], ["Lib"]));

        var lib = report.Projects.Single(p => p.Path == "Lib/Lib.csproj");
        Assert.Equal([new IvtDeclaration("App", "Lib/AssemblyInfo.cs")], lib.IvtDeclarations);
        Assert.Equal(["Shared/Marker.cs"], lib.LinkedFiles);
        Assert.Equal(
            ["Lib/AssemblyInfo.cs", "Lib/Gen.cs", "Lib/Lib.cs", "Lib/View.Designer.cs", "Shared/Marker.cs"],
            lib.Files.Select(f => f.Path));
        Assert.Equal([false, true, false, true, false], lib.Files.Select(f => f.Generated));
        // With the attribute the use binds directly; it is still intrusive.
        Assert.Equal("intrusive", Assert.Single(Edges(report, "App/U.cs")).Hint);
    }

    [Theory]
    [InlineData("A/B.cs", false)]
    [InlineData("A/B.g.cs", true)]
    [InlineData("A/B.g.i.cs", true)]
    [InlineData("A/Form.Designer.cs", true)]
    [InlineData("A/obj/Debug/net10.0/A.AssemblyInfo.cs", true)]
    [InlineData("A/bin/X.cs", true)]
    [InlineData("artifacts/obj/A/debug/A.AssemblyInfo.cs", true)]
    [InlineData("objects/B.cs", false)]
    public void GeneratedPaths(string path, bool generated)
    {
        Assert.Equal(generated, Generated.IsGeneratedPath(path));
    }

    private static IEnumerable<EdgeReport> Edges(Report report, string file) =>
        report.Projects.SelectMany(p => p.Files).Single(f => f.Path == file).Edges;

    private static async Task<Report> Analyze(params (string Name, (string Path, string Source)[] Files, string[] References)[] projects) =>
        await AnalyzeWith([], projects);

    private static WorkspaceDiagnostic Failed(string project, string message) =>
        new(WorkspaceDiagnosticKind.Failure, $"Msbuild failed when processing the file '{project}' with message: {message}");

    // Roslyn's semantic model throws while binding a file of a compilation
    // whose references are missing. One file must cost its own edges, not the
    // run: the guard turns it into that file's failure.
    [Fact]
    public void AThrowingFileIsNamedAndOtherFilesKeepTheirEdges()
    {
        var (edges, failure) = Collector.Guard(() => throw new NullReferenceException());
        Assert.Empty(edges);
        Assert.Equal("NullReferenceException", failure);

        var (kept, none) = Collector.Guard(() => (IReadOnlyList<EdgeReport>)[new EdgeReport("A.cs", false, "model", "", "type_use", 1)]);
        Assert.Single(kept);
        Assert.Equal("", none);
    }

    // The diagnostics call throws on the same fault; the framework keeps its
    // files and reports the failure instead of the run losing the project.
    [Fact]
    public void ADiagnosticsFailureIsNamed()
    {
        var (unresolved, other, failure) = Collector.ErrorDiagnostics(() => throw new NullReferenceException());
        Assert.Equal(("NullReferenceException", 0, 0), (failure, unresolved.Count, other.Count));
    }

    [Fact]
    public void ErrorDiagnosticsSplitsUnresolvedNamesFromOtherErrors()
    {
        var compilation = CSharpCompilation.Create(
            "T",
            [CSharpSyntaxTree.ParseText("class U { Missing m; void M() { int x = \"s\"; } }")],
            Framework,
            new CSharpCompilationOptions(OutputKind.DynamicallyLinkedLibrary));
        var (unresolved, other, failure) = Collector.ErrorDiagnostics(() => compilation.GetDiagnostics());
        Assert.Equal("", failure);
        Assert.Contains("CS0246", unresolved.Keys);
        Assert.Contains("CS0029", other.Keys);
    }

    // MultiProject is the one project file the attempts of AnalyzeAttempts
    // share, as MSBuildWorkspace reports a multi-targeted project.
    private static readonly string MultiProject = Path.Combine(Root, "Multi", "Multi.csproj");

    // AnalyzeAttempts loads attempts of one project file: the same FilePath
    // under several project names, as MSBuildWorkspace keeps a multi-targeted
    // project's frameworks. An empty source is an attempt with no documents.
    private static async Task<Report> AnalyzeAttempts(
        IReadOnlyList<WorkspaceDiagnostic> diagnostics, params (string Name, string Source)[] attempts)
    {
        using var workspace = new AdhocWorkspace();
        var solution = workspace.CurrentSolution;
        foreach (var (name, source) in attempts)
        {
            var id = ProjectId.CreateNewId();
            var documents = source.Length == 0
                ? Array.Empty<DocumentInfo>()
                : [DocumentInfo.Create(
                    DocumentId.CreateNewId(id),
                    name + ".cs",
                    loader: TextLoader.From(TextAndVersion.Create(SourceText.From(source), VersionStamp.Default)),
                    filePath: Path.Combine(Root, "Multi", name + ".cs"))];
            solution = solution.AddProject(ProjectInfo.Create(
                id,
                VersionStamp.Default,
                name,
                name,
                LanguageNames.CSharp,
                filePath: MultiProject,
                compilationOptions: new CSharpCompilationOptions(OutputKind.DynamicallyLinkedLibrary, nullableContextOptions: NullableContextOptions.Enable),
                parseOptions: new CSharpParseOptions(LanguageVersion.Latest),
                documents: documents,
                metadataReferences: Framework));
        }
        return await Classifier.ClassifyAsync(solution, Root, diagnostics, CancellationToken.None);
    }

    private static async Task<Report> AnalyzeWith(
        IReadOnlyList<WorkspaceDiagnostic> diagnostics,
        params (string Name, (string Path, string Source)[] Files, string[] References)[] projects)
    {
        using var workspace = new AdhocWorkspace();
        var ids = projects.ToDictionary(p => p.Name, _ => ProjectId.CreateNewId());
        var solution = workspace.CurrentSolution;
        foreach (var (name, files, references) in projects)
        {
            solution = solution.AddProject(ProjectInfo.Create(
                ids[name],
                VersionStamp.Default,
                name,
                name,
                LanguageNames.CSharp,
                filePath: Path.Combine(Root, name, name + ".csproj"),
                compilationOptions: new CSharpCompilationOptions(OutputKind.DynamicallyLinkedLibrary, nullableContextOptions: NullableContextOptions.Enable),
                parseOptions: new CSharpParseOptions(LanguageVersion.Latest),
                documents: files.Select(f => DocumentInfo.Create(
                    DocumentId.CreateNewId(ids[name]),
                    Path.GetFileName(f.Path),
                    loader: TextLoader.From(TextAndVersion.Create(SourceText.From(f.Source), VersionStamp.Default)),
                    filePath: Path.Combine(Root, f.Path))),
                projectReferences: references.Select(r => new ProjectReference(ids[r])),
                metadataReferences: Framework));
        }
        return await Classifier.ClassifyAsync(solution, Root, diagnostics, CancellationToken.None);
    }

    private static readonly MetadataReference[] Framework = ((string)AppContext.GetData("TRUSTED_PLATFORM_ASSEMBLIES")!)
        .Split(Path.PathSeparator)
        .Where(p => Path.GetFileName(p) is "System.Private.CoreLib.dll" or "System.Runtime.dll")
        .Select(p => (MetadataReference)MetadataReference.CreateFromFile(p))
        .ToArray();
}
