namespace Fixture.Application;

public interface IRepository
{
    Entity Load();
}

public record OrderDto(string Id, int Quantity);
