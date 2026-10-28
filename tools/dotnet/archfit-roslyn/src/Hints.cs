namespace ArchfitRoslyn;

// How a file uses a referenced symbol.
public enum UseKind
{
    // Names a type: parameter, field, local, cast, generic argument, qualifier.
    TypeUse,
    // Lists a type in a base list.
    Inherit,
    // Calls a method, event, constructor, or a property with behavior.
    Call,
    // Reads or writes a field, constant, or auto-property value.
    DataUse,
}

// What the referenced symbol is.
public enum TargetKind
{
    // An interface or abstract type, or an abstract or interface member.
    Abstraction,
    // A concrete class, struct, enum, or delegate, or a member of one.
    Concrete,
    // A record or struct with data members and no behavior, or a member of one.
    Dto,
}

public readonly record struct Use(UseKind Kind, TargetKind Target, bool InternalElsewhere);

// Strength labels shared with the Go side (graph.Edge StrengthHint and
// DataStrengthHint; coupling strengths plus the hint-only "dto").
public static class Hint
{
    public const string None = "";
    public const string Contract = "contract";
    public const string Dto = "dto";
    public const string Model = "model";
    public const string Functional = "functional";
    public const string Intrusive = "intrusive";

    // Same order as the Go extractor's goStrengthRank: dto sits between
    // contract and model.
    public static int Rank(string hint) => hint switch
    {
        Contract => 1,
        Dto => 2,
        Model => 3,
        Functional => 4,
        Intrusive => 5,
        _ => 0,
    };

    public static string Stronger(string a, string b) => Rank(b) > Rank(a) ? b : a;

    // Map applies the D3 table to one use. Hint carries every row; DataHint
    // carries only the non-callable rows (model, dto).
    public static (string Hint, string DataHint) Map(Use use)
    {
        if (use.InternalElsewhere)
        {
            return (Intrusive, None);
        }
        if (use.Target == TargetKind.Abstraction)
        {
            return (Contract, None);
        }
        if (use.Kind is UseKind.Call or UseKind.Inherit)
        {
            return (Functional, None);
        }
        var data = use.Target == TargetKind.Dto ? Dto : Model;
        return (data, data);
    }
}
