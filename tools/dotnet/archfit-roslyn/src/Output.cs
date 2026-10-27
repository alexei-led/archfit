using System.Text.Encodings.Web;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace ArchfitRoslyn;

// The helper's stdout contract. Paths are relative to the analysis root and
// use '/'. The Go extractor (internal/extract/dotnet) checks SchemaVersion.
internal sealed record Report(
    int SchemaVersion,
    IReadOnlyList<string> SkippedProjects,
    IReadOnlyList<ProjectReport> Projects)
{
    // 2: per-edge first-use line; per-project used_projects replaces
    // unused_references (the caller knows the direct references).
    public const int CurrentSchemaVersion = 2;

    private static readonly JsonSerializerOptions Json = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
        WriteIndented = false,
    };

    public string ToJson() => JsonSerializer.Serialize(this, Json);
}

internal static class Status
{
    public const string Complete = "complete";
    public const string Partial = "partial";
    public const string Failed = "failed";
}

// One entry per (project, target framework). A failed project has no TFM
// ("") and no files.
internal sealed record ProjectReport(
    string Path,
    string Tfm,
    string Status,
    string Reason,
    int UnresolvedSymbols,
    // Root-relative paths of the other workspace projects whose symbols this
    // project uses, sorted. The workspace's references include transitive
    // ones, so the caller compares this with the project's direct references.
    IReadOnlyList<string> UsedProjects,
    IReadOnlyList<IvtDeclaration> IvtDeclarations,
    IReadOnlyList<string> LinkedFiles,
    IReadOnlyList<FileReport> Files);

// DeclaredIn is the project file for an MSBuild InternalsVisibleTo item (the
// SDK turns it into a generated attribute) or the source file of an
// [assembly: InternalsVisibleTo] attribute.
internal sealed record IvtDeclaration(string Assembly, string DeclaredIn);

internal sealed record FileReport(string Path, bool Generated, IReadOnlyList<EdgeReport> Edges);

// To is a root-relative file path, or an assembly name when External is true.
// Line is the 1-based line of the first use in the file.
internal sealed record EdgeReport(string To, bool External, string Hint, string DataHint, string Reference, int Line);
