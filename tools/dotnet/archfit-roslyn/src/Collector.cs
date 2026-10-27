using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace ArchfitRoslyn;

// Collected facts for one loaded project. Status is decided by the caller,
// which also sees workspace diagnostics.
internal sealed record Collected(
    string Tfm,
    int UnresolvedSymbols,
    IReadOnlyDictionary<string, int> UnresolvedById,
    IReadOnlyList<string> UsedProjects,
    IReadOnlyList<IvtDeclaration> IvtDeclarations,
    IReadOnlyList<string> LinkedFiles,
    IReadOnlyList<FileReport> Files);

// Collector walks one project's compilation: every name that resolves to a
// symbol becomes a use of the file (or assembly) that declares it. It needs a
// Solution, not an MSBuildWorkspace, so tests can drive it from an
// AdhocWorkspace.
internal sealed class Collector(Solution solution, string root)
{
    // Compiler diagnostics that mean a name did not resolve (D5).
    internal static readonly IReadOnlySet<string> UnresolvedIds = new HashSet<string>
    {
        "CS0006", // metadata file not found
        "CS0012", // type defined in an unreferenced assembly
        "CS0103", // name does not exist in the current context
        "CS0234", // type or namespace does not exist in the namespace
        "CS0246", // type or namespace not found
        "CS0400", // type not found in the global namespace
        "CS0518", // predefined type not defined (no reference pack)
        "CS0117", // type has no such member: the defining package is not loaded
        "CS1061", // receiver has no such member: an extension from a package not loaded
    };

    private const string IvtAttribute = "System.Runtime.CompilerServices.InternalsVisibleToAttribute";

    private readonly string _root = Paths.Full(root);

    public async Task<Collected> CollectAsync(Project project, CancellationToken ct)
    {
        var compilation = await project.GetCompilationAsync(ct)
            ?? throw new InvalidOperationException($"no compilation for {project.FilePath}");

        var unresolvedById = compilation.GetDiagnostics(ct)
            .Where(d => d.Severity == DiagnosticSeverity.Error && UnresolvedIds.Contains(d.Id))
            .GroupBy(d => d.Id)
            .ToDictionary(g => g.Key, g => g.Count());

        var projectDir = project.FilePath is null ? _root : Path.GetDirectoryName(Paths.Full(project.FilePath))!;
        var files = new List<FileReport>();
        var linked = new List<string>();
        var usedAssemblies = new HashSet<string>(StringComparer.Ordinal);

        foreach (var document in project.Documents.OrderBy(d => d.FilePath, StringComparer.Ordinal))
        {
            var rel = Relative(document.FilePath);
            if (rel is null)
            {
                continue;
            }
            var tree = await document.GetSyntaxTreeAsync(ct);
            if (tree is null)
            {
                continue;
            }
            if (!Paths.IsUnder(projectDir, Paths.Full(document.FilePath!)))
            {
                linked.Add(rel);
            }
            var model = compilation.GetSemanticModel(tree);
            var edges = CollectFile(rel, await tree.GetRootAsync(ct), model, compilation, usedAssemblies, ct);
            files.Add(new FileReport(rel, Generated.IsGenerated(rel, tree, ct), edges));
        }

        return new Collected(
            Tfm(project),
            unresolvedById.Values.Sum(),
            unresolvedById,
            UsedProjects(project, usedAssemblies),
            IvtDeclarations(compilation, project),
            linked,
            files);
    }

    private List<EdgeReport> CollectFile(
        string from,
        SyntaxNode syntaxRoot,
        SemanticModel model,
        Compilation compilation,
        HashSet<string> usedAssemblies,
        CancellationToken ct)
    {
        // Per target: strongest hint, its reference kind, strongest data hint,
        // and the line of the first use. Each channel aggregates independently.
        var byTarget = new Dictionary<(string To, bool External), (string Hint, string Reference, string DataHint, int Line)>();

        foreach (var node in syntaxRoot.DescendantNodes())
        {
            if (!IsReferenceNode(node))
            {
                continue;
            }
            var symbol = Resolve(model.GetSymbolInfo(node, ct), compilation);
            if (symbol is null || UseOf(node, symbol, compilation) is not { } use)
            {
                continue;
            }
            usedAssemblies.Add(symbol.ContainingAssembly.Name);
            if (Target(symbol) is not { } target || (!target.External && target.To == from))
            {
                continue;
            }

            var (hint, dataHint) = Hint.Map(use);
            var reference = ReferenceKind(use, symbol);
            if (byTarget.TryGetValue(target, out var seen))
            {
                if (Hint.Rank(hint) > Hint.Rank(seen.Hint))
                {
                    seen.Hint = hint;
                    seen.Reference = reference;
                }
                seen.DataHint = Hint.Stronger(seen.DataHint, dataHint);
                byTarget[target] = seen;
            }
            else
            {
                // DescendantNodes walks in document order: the first use seen
                // is the first in the file.
                byTarget[target] = (hint, reference, dataHint, node.GetLocation().GetLineSpan().StartLinePosition.Line + 1);
            }
        }

        return byTarget
            .OrderBy(kv => kv.Key.External)
            .ThenBy(kv => kv.Key.To, StringComparer.Ordinal)
            .Select(kv => new EdgeReport(kv.Key.To, kv.Key.External, kv.Value.Hint, kv.Value.DataHint, kv.Value.Reference, kv.Value.Line))
            .ToList();
    }

    private static bool IsReferenceNode(SyntaxNode node) => node is SimpleNameSyntax
        or BaseObjectCreationExpressionSyntax
        or ConstructorInitializerSyntax
        or PrimaryConstructorBaseTypeSyntax
        or ElementAccessExpressionSyntax;

    // Resolve returns the bound symbol, or an inaccessible candidate when it is
    // internal (or private protected) and declared in source of another
    // workspace project: that use is real but lacks the IVT attribute in this
    // compilation (D3). Other candidates (protected members, metadata
    // internals, overload failures) are not uses.
    private ISymbol? Resolve(SymbolInfo info, Compilation compilation)
    {
        var symbol = info.Symbol;
        if (symbol is null && info.CandidateReason == CandidateReason.Inaccessible)
        {
            symbol = info.CandidateSymbols.FirstOrDefault(c =>
                IsEffectivelyInternal(c)
                && !SymbolEqualityComparer.Default.Equals(c.ContainingAssembly, compilation.Assembly)
                && DeclaredInWorkspaceSource(c));
        }
        if (symbol is IAliasSymbol alias)
        {
            symbol = alias.Target;
        }
        if (symbol is IMethodSymbol { ReducedFrom: { } reduced })
        {
            symbol = reduced;
        }
        return symbol switch
        {
            null or INamespaceSymbol or ILocalSymbol or IParameterSymbol or ITypeParameterSymbol
                or ILabelSymbol or IRangeVariableSymbol or IDiscardSymbol or IPreprocessingSymbol => null,
            ITypeSymbol { TypeKind: TypeKind.Error } => null,
            IArrayTypeSymbol or IPointerTypeSymbol or IFunctionPointerTypeSymbol or IDynamicTypeSymbol => null,
            _ when symbol.ContainingAssembly is null => null,
            _ => symbol.OriginalDefinition,
        };
    }

    private bool DeclaredInWorkspaceSource(ISymbol symbol) => symbol.Locations.Any(l =>
        l.IsInSource
        && l.SourceTree?.FilePath is { Length: > 0 } path
        && !solution.GetDocumentIdsWithFilePath(path).IsEmpty);

    private static Use? UseOf(SyntaxNode node, ISymbol symbol, Compilation compilation)
    {
        var internalElsewhere = IsEffectivelyInternal(symbol)
            && !SymbolEqualityComparer.Default.Equals(symbol.ContainingAssembly, compilation.Assembly);
        switch (symbol)
        {
            case INamedTypeSymbol type:
                return new Use(InBaseList(node) ? UseKind.Inherit : UseKind.TypeUse, TypeTarget(type), internalElsewhere);
            case IMethodSymbol method:
                return new Use(UseKind.Call, MemberTarget(method), internalElsewhere);
            case IEventSymbol evt:
                return new Use(UseKind.Call, MemberTarget(evt), internalElsewhere);
            case IPropertySymbol property:
                return new Use(IsDataProperty(property) ? UseKind.DataUse : UseKind.Call, MemberTarget(property), internalElsewhere);
            case IFieldSymbol field:
                return new Use(UseKind.DataUse, MemberTarget(field), internalElsewhere);
            default:
                return null;
        }
    }

    private static string ReferenceKind(Use use, ISymbol symbol) => use.Kind switch
    {
        UseKind.Inherit => symbol is INamedTypeSymbol { TypeKind: TypeKind.Interface } ? "implement" : "inherit",
        UseKind.Call => "call",
        UseKind.DataUse => "data_use",
        _ => "type_use",
    };

    // InBaseList reports whether a name is the type in a base list entry, not a
    // type argument inside it (": List<Entity>" inherits List, uses Entity).
    private static bool InBaseList(SyntaxNode node)
    {
        var current = node;
        while (current.Parent is QualifiedNameSyntax q && q.Right == current
            || current.Parent is AliasQualifiedNameSyntax a && a.Name == current)
        {
            current = current.Parent;
        }
        return current.Parent is BaseTypeSyntax { Parent: BaseListSyntax };
    }

    private static TargetKind TypeTarget(INamedTypeSymbol type)
    {
        if (type.TypeKind == TypeKind.Interface || (type.IsAbstract && !type.IsStatic && type.TypeKind == TypeKind.Class))
        {
            return TargetKind.Abstraction;
        }
        return IsDto(type) ? TargetKind.Dto : TargetKind.Concrete;
    }

    private static TargetKind MemberTarget(ISymbol member)
    {
        if (member.IsAbstract || member.ContainingType?.TypeKind == TypeKind.Interface)
        {
            return TargetKind.Abstraction;
        }
        return member.ContainingType is { } owner && IsDto(owner) ? TargetKind.Dto : TargetKind.Concrete;
    }

    // IsDto: a source record or struct with at least one data member and no
    // behavior (no declared methods, operators, events, indexers, or property
    // bodies). Compiler-synthesized record members do not count. A metadata
    // type cannot be inspected this way and stays concrete.
    internal static bool IsDto(INamedTypeSymbol type)
    {
        if (!(type.IsRecord || type.TypeKind == TypeKind.Struct) || type.TypeKind == TypeKind.Enum)
        {
            return false;
        }
        if (!type.Locations.Any(l => l.IsInSource))
        {
            return false;
        }
        // A record struct derives from System.ValueType and a record class from System.Object; neither is a user base.
        if (type.IsRecord && type.BaseType is { SpecialType: not (SpecialType.System_Object or SpecialType.System_ValueType) } baseType && !IsDto(baseType))
        {
            return false;
        }
        var data = 0;
        foreach (var member in type.GetMembers().Where(m => !m.IsImplicitlyDeclared))
        {
            switch (member)
            {
                case IFieldSymbol:
                    data++;
                    break;
                case IPropertySymbol property when IsDataProperty(property):
                    data++;
                    break;
                case IMethodSymbol { MethodKind: MethodKind.Constructor or MethodKind.PropertyGet or MethodKind.PropertySet }:
                    break;
                case INamedTypeSymbol:
                    break;
                default:
                    return false;
            }
        }
        return data > 0;
    }

    // IsDataProperty: an auto-property or a positional record property. A
    // property with an accessor body, an expression body, or an indexer is
    // behavior. Metadata properties cannot be inspected and count as data.
    internal static bool IsDataProperty(IPropertySymbol property)
    {
        if (property.IsIndexer)
        {
            return false;
        }
        foreach (var reference in property.DeclaringSyntaxReferences)
        {
            if (reference.GetSyntax() is PropertyDeclarationSyntax declaration
                && (declaration.ExpressionBody is not null
                    || declaration.AccessorList?.Accessors.Any(a => a.Body is not null || a.ExpressionBody is not null) == true))
            {
                return false;
            }
        }
        return true;
    }

    internal static bool IsEffectivelyInternal(ISymbol symbol)
    {
        for (var current = symbol; current is not null and not INamespaceSymbol; current = current.ContainingSymbol)
        {
            if (current.DeclaredAccessibility is Accessibility.Internal or Accessibility.ProtectedAndInternal)
            {
                return true;
            }
        }
        return false;
    }

    // Target is the declaring file (root-relative) for a source symbol, or the
    // assembly for a metadata symbol. A source symbol declared only outside the
    // root or in build output (obj/, bin/, source generators) has no target.
    private (string To, bool External)? Target(ISymbol symbol)
    {
        var sourcePaths = symbol.Locations
            .Where(l => l.IsInSource)
            .Select(l => Relative(l.SourceTree?.FilePath))
            .OfType<string>()
            .ToList();
        if (sourcePaths.Count > 0)
        {
            // A partial type has several declaring files: prefer a hand-written one.
            var path = sourcePaths
                .OrderBy(p => Generated.IsGeneratedPath(p))
                .ThenBy(p => p, StringComparer.Ordinal)
                .First();
            return (path, false);
        }
        if (symbol.Locations.Any(l => l.IsInSource))
        {
            return null;
        }
        return (symbol.ContainingAssembly.Name, true);
    }

    // UsedProjects names the other workspace projects whose assembly this
    // project used. ProjectReferences would not do: the design-time build
    // adds transitive references to them, and drops ReferenceOutputAssembly=
    // false ones; the caller has the direct references from evaluation.
    private List<string> UsedProjects(Project project, HashSet<string> usedAssemblies) => solution.Projects
        .Where(p => p.FilePath != project.FilePath && usedAssemblies.Contains(p.AssemblyName))
        .Select(p => Relative(p.FilePath))
        .OfType<string>()
        .Distinct(StringComparer.Ordinal)
        .Order(StringComparer.Ordinal)
        .ToList();

    private List<IvtDeclaration> IvtDeclarations(Compilation compilation, Project project)
    {
        var projectPath = Relative(project.FilePath);
        var declarations = new SortedSet<IvtDeclaration>(Comparer<IvtDeclaration>.Create((a, b) =>
        {
            var c = string.CompareOrdinal(a.Assembly, b.Assembly);
            return c != 0 ? c : string.CompareOrdinal(a.DeclaredIn, b.DeclaredIn);
        }));
        foreach (var attribute in compilation.Assembly.GetAttributes())
        {
            if (attribute.AttributeClass?.ToDisplayString() != IvtAttribute
                || attribute.ConstructorArguments is not [{ Value: string name }])
            {
                continue;
            }
            var assembly = name.Split(',')[0].Trim();
            // The SDK writes csproj items into a generated AssemblyInfo under
            // obj/, which Relative maps to null: report the project file.
            var file = Relative(attribute.ApplicationSyntaxReference?.SyntaxTree.FilePath);
            declarations.Add(new IvtDeclaration(assembly, file ?? projectPath ?? ""));
        }
        return declarations.ToList();
    }

    private static string Tfm(Project project)
    {
        if (project.AnalyzerOptions.AnalyzerConfigOptionsProvider.GlobalOptions
                .TryGetValue("build_property.TargetFramework", out var tfm) && tfm.Length > 0)
        {
            return tfm;
        }
        // Multi-targeted projects are named "Name(tfm)".
        var name = project.Name;
        var open = name.LastIndexOf('(');
        return open >= 0 && name.EndsWith(')') ? name[(open + 1)..^1] : "";
    }

    // Relative returns a root-relative '/' path for a file under the root and
    // outside build output, or null.
    private string? Relative(string? path)
    {
        if (string.IsNullOrEmpty(path) || !Path.IsPathRooted(path))
        {
            return null;
        }
        var full = Paths.Full(path);
        if (!Paths.IsUnder(_root, full))
        {
            return null;
        }
        var rel = Paths.Relative(_root, full);
        return Generated.IsBuildOutputPath(rel) ? null : rel;
    }
}
