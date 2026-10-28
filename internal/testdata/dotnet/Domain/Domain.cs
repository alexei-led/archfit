namespace Fixture.Domain;

public class Entity
{
    public string Label = "entity";
    public int Count { get; set; }
    public virtual int Next() => Count + 1;
}

public abstract class Policy
{
    public abstract bool Allows(Entity entity);
}

public record struct Money(decimal Amount);

internal static class Secrets
{
    internal static int Seed = 42;
}
