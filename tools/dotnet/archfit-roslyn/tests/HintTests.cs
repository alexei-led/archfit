namespace ArchfitRoslyn.Tests;

public class HintTests
{
    // One row per D3 table row and use shape.
    [Theory]
    // Implements or uses an interface or abstract type: contract.
    [InlineData(UseKind.Inherit, TargetKind.Abstraction, false, "contract", "")]
    [InlineData(UseKind.TypeUse, TargetKind.Abstraction, false, "contract", "")]
    [InlineData(UseKind.Call, TargetKind.Abstraction, false, "contract", "")]
    // Uses a concrete type, field, property value, or constant as data only: model.
    [InlineData(UseKind.TypeUse, TargetKind.Concrete, false, "model", "model")]
    [InlineData(UseKind.DataUse, TargetKind.Concrete, false, "model", "model")]
    // Uses a record or struct with no behavior as data only: dto.
    [InlineData(UseKind.TypeUse, TargetKind.Dto, false, "dto", "dto")]
    [InlineData(UseKind.DataUse, TargetKind.Dto, false, "dto", "dto")]
    // Calls a method, property, event, or constructor of a concrete type: functional.
    [InlineData(UseKind.Call, TargetKind.Concrete, false, "functional", "")]
    [InlineData(UseKind.Call, TargetKind.Dto, false, "functional", "")]
    // Inherits a concrete class: functional, never contract.
    [InlineData(UseKind.Inherit, TargetKind.Concrete, false, "functional", "")]
    [InlineData(UseKind.Inherit, TargetKind.Dto, false, "functional", "")]
    // Internal or private protected symbol declared in another assembly: intrusive.
    [InlineData(UseKind.TypeUse, TargetKind.Concrete, true, "intrusive", "")]
    [InlineData(UseKind.DataUse, TargetKind.Concrete, true, "intrusive", "")]
    [InlineData(UseKind.Call, TargetKind.Concrete, true, "intrusive", "")]
    [InlineData(UseKind.Inherit, TargetKind.Abstraction, true, "intrusive", "")]
    public void MapAppliesTheD3Table(UseKind kind, TargetKind target, bool internalElsewhere, string hint, string dataHint)
    {
        Assert.Equal((hint, dataHint), Hint.Map(new Use(kind, target, internalElsewhere)));
    }

    [Theory]
    [InlineData("contract", "dto", "dto")]
    [InlineData("dto", "model", "model")]
    [InlineData("model", "functional", "functional")]
    [InlineData("functional", "intrusive", "intrusive")]
    [InlineData("intrusive", "contract", "intrusive")]
    [InlineData("", "contract", "contract")]
    [InlineData("model", "", "model")]
    public void StrongerKeepsTheHigherRank(string a, string b, string want)
    {
        Assert.Equal(want, Hint.Stronger(a, b));
    }
}
