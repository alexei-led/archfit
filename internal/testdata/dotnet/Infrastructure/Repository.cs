using Fixture.Application;
using Fixture.Domain;

namespace Fixture.Infrastructure;

public class Repository : Entity, IRepository
{
    public Entity Load() => new();
}

public class Allowed : Policy
{
    public override bool Allows(Entity entity) => true;
}

public static class Uses
{
    public static string Model(Entity entity) => entity.Label;
    public static decimal Dto(Money money) => money.Amount;
    public static int Functional(Entity entity) => entity.Next();
    public static bool Contract(Policy policy, Entity entity) => policy.Allows(entity);
    public static int Intrusive() => Secrets.Seed;
    public static string Order(OrderDto order) => order.Id;
}
