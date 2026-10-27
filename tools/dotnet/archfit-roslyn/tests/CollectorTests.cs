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

    private static async Task<Report> Analyze(params (string Name, (string Path, string Source)[] Files, string[] References)[] projects)
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
        return await Classifier.ClassifyAsync(solution, Root, [], CancellationToken.None);
    }

    private static readonly MetadataReference[] Framework = ((string)AppContext.GetData("TRUSTED_PLATFORM_ASSEMBLIES")!)
        .Split(Path.PathSeparator)
        .Where(p => Path.GetFileName(p) is "System.Private.CoreLib.dll" or "System.Runtime.dll")
        .Select(p => (MetadataReference)MetadataReference.CreateFromFile(p))
        .ToArray();
}
