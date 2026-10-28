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
    // CompileErrors are error diagnostics other than an unresolved name: the
    // file does not compile, so the edges it would carry are not all real.
    int CompileErrors,
    IReadOnlyDictionary<string, int> CompileErrorsById,
    // Unreadable are the files whose symbol walk threw, as "<path>: <exception>".
    // Roslyn's nullable flow analysis throws on a compilation whose references
    // are missing; the file's edges are unknown, the rest of the project is not.
    IReadOnlyList<string> Unreadable,
    // DiagnosticsFailure names the exception the compiler threw while reporting
    // its diagnostics, "" when it reported them.
    string DiagnosticsFailure,
    // Codegen names a code generator whose types the design-time compilation
    // cannot produce, "" when none is detected.
    string Codegen,
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

        var (unresolvedById, compileErrorsById, diagnosticsFailure) =
            ErrorDiagnostics(() => compilation.GetDiagnostics(ct));

        var projectDir = project.FilePath is null ? _root : Path.GetDirectoryName(Paths.Full(project.FilePath))!;
        var files = new List<FileReport>();
        var linked = new List<string>();
        var unreadable = new List<string>();
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
            var syntaxRoot = await tree.GetRootAsync(ct);
            var (edges, failure) = Guard(() => CollectFile(rel, syntaxRoot, model, compilation, usedAssemblies, ct));
            if (failure != "")
            {
                unreadable.Add($"{rel}: {failure}");
                continue;
            }
            files.Add(new FileReport(rel, Generated.IsGenerated(rel, tree, ct), edges));
        }

        return new Collected(
            Tfm(project),
            unresolvedById.Values.Sum(),
            unresolvedById,
            compileErrorsById.Values.Sum(),
            compileErrorsById,
            unreadable,
            diagnosticsFailure,
            Codegen(project),
            UsedProjects(project, usedAssemblies),
            IvtDeclarations(compilation, project),
            linked,
            files);
    }

    // Guard runs one file's collection. Roslyn's semantic model can throw while
    // binding a file of a compilation whose references are missing (nullable
    // flow analysis over error types, in 5.6: System.NullReferenceException),
    // and one such file must cost its own edges, not the run's whole report.
    internal static (IReadOnlyList<EdgeReport> Edges, string Failure) Guard(Func<IReadOnlyList<EdgeReport>> collect)
    {
        try
        {
            return (collect(), "");
        }
        catch (Exception e) when (e is not OperationCanceledException)
        {
            return ([], e.GetType().Name);
        }
    }

    // Codegen names a generator whose output this compilation does not have.
    // The Razor SDK compiles .razor and .cshtml through a source generator that
    // the workspace loads but that produces nothing here, so the namespaces and
    // components it declares stay unresolved (verified on eShop: 25 analyzers
    // including the Razor compiler, 18 .razor additional files, no generated
    // tree). Protobuf/Grpc.Tools needs no note: its design-time build does
    // include the generated sources.
    private static string Codegen(Project project) =>
        project.AdditionalDocuments.Any(d => d.FilePath is { } path
            && (path.EndsWith(".razor", StringComparison.OrdinalIgnoreCase) || path.EndsWith(".cshtml", StringComparison.OrdinalIgnoreCase)))
            ? "Razor"
            : "";

    // ErrorDiagnostics counts the compilation's errors, split into unresolved
    // names and the rest. The same Roslyn fault can throw here, and a framework
    // whose error count is unknown keeps its files and is partial, not a failed
    // run.
    internal static (IReadOnlyDictionary<string, int> Unresolved, IReadOnlyDictionary<string, int> Other, string Failure)
        ErrorDiagnostics(Func<IEnumerable<Diagnostic>> report)
    {
        List<Diagnostic> errors;
        try
        {
            errors = report().Where(d => d.Severity == DiagnosticSeverity.Error).ToList();
        }
        catch (Exception e) when (e is not OperationCanceledException)
        {
            return (new Dictionary<string, int>(), new Dictionary<string, int>(), e.GetType().Name);
        }
        return (
            errors.Where(d => UnresolvedIds.Contains(d.Id)).GroupBy(d => d.Id).ToDictionary(g => g.Key, g => g.Count()),
            errors.Where(d => !UnresolvedIds.Contains(d.Id)).GroupBy(d => d.Id).ToDictionary(g => g.Key, g => g.Count()),
            "");
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
            var symbol = IsReferenceNode(node)
                ? Resolve(model.GetSymbolInfo(node, ct), compilation)
                : IsOperatorNode(node) ? OperatorMethod(node, model, ct) : null;
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

    // IsOperatorNode is an expression that invokes an operator, which for a
    // built-in operand names no symbol to attribute. A user-defined one binds
    // to its operator method, whose declaring file is the target of the call.
    // A simple assignment and ??= bind no operator.
    private static bool IsOperatorNode(SyntaxNode node) => node switch
    {
        BinaryExpressionSyntax or PrefixUnaryExpressionSyntax or PostfixUnaryExpressionSyntax => true,
        AssignmentExpressionSyntax assignment => !assignment.IsKind(SyntaxKind.SimpleAssignmentExpression)
            && !assignment.IsKind(SyntaxKind.CoalesceAssignmentExpression),
        _ => false,
    };

    // OperatorMethod is the user-defined operator an operator expression
    // invokes, or null for a built-in operator (int + int), which has no
    // declaring syntax to name.
    private static IMethodSymbol? OperatorMethod(SyntaxNode node, SemanticModel model, CancellationToken ct) =>
        model.GetSymbolInfo(node, ct).Symbol is IMethodSymbol
        {
            MethodKind: MethodKind.UserDefinedOperator or MethodKind.Conversion,
            ContainingAssembly: not null,
        } method
            ? method.OriginalDefinition
            : null;

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
        var internalElsewhere = InternalElsewhere(node, symbol, compilation);
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

    // InternalElsewhere reports whether this use reaches an internal
    // declaration of another assembly. A property can be public while the
    // accessor the expression invokes is internal (public int Value { get;
    // internal set; }): a friend assembly may take that path through IVT, and
    // the use is the access of that accessor, not of the property name.
    private static bool InternalElsewhere(SyntaxNode node, ISymbol symbol, Compilation compilation)
    {
        if (SymbolEqualityComparer.Default.Equals(symbol.ContainingAssembly, compilation.Assembly))
        {
            return false;
        }
        if (IsEffectivelyInternal(symbol))
        {
            return true;
        }
        if (symbol is not IPropertySymbol property)
        {
            return false;
        }
        var (get, set) = AccessorsUsed(node);
        return (get && IsInternal(property.GetMethod)) || (set && IsInternal(property.SetMethod));

        static bool IsInternal(IMethodSymbol? accessor) => accessor is not null && IsEffectivelyInternal(accessor);
    }

    // AccessorsUsed reports whether the expression invokes a property's getter,
    // setter, or both. A simple assignment invokes the setter alone; a compound
    // assignment, an increment or decrement, and a by-ref use read and write.
    private static (bool Get, bool Set) AccessorsUsed(SyntaxNode node)
    {
        // The named node is the name of the access that qualifies it: c.Value.
        var expression = node;
        while (expression.Parent is MemberAccessExpressionSyntax member && member.Name == expression)
        {
            expression = member;
        }
        switch (expression.Parent)
        {
            case AssignmentExpressionSyntax assignment when assignment.Left == expression:
                return assignment.IsKind(SyntaxKind.SimpleAssignmentExpression) ? (false, true) : (true, true);
            case PrefixUnaryExpressionSyntax prefix
                when prefix.IsKind(SyntaxKind.PreIncrementExpression) || prefix.IsKind(SyntaxKind.PreDecrementExpression):
            case PostfixUnaryExpressionSyntax postfix
                when postfix.IsKind(SyntaxKind.PostIncrementExpression) || postfix.IsKind(SyntaxKind.PostDecrementExpression):
            case RefExpressionSyntax:
                return (true, true);
            case ArgumentSyntax argument
                when argument.RefKindKeyword.IsKind(SyntaxKind.RefKeyword) || argument.RefKindKeyword.IsKind(SyntaxKind.OutKeyword):
                return (true, true);
            default:
                return (true, false);
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

    // Tfm is the project's target framework: the workspace's own property, else
    // the framework in the name MSBuildWorkspace gives a multi-targeted
    // project's framework ("Name(tfm)"). Empty for a framework-less evaluation.
    internal static string Tfm(Project project)
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
