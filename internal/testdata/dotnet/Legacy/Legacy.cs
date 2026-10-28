namespace Fixture.Legacy;

public static class Compat
{
#if NET10_0
    public static int Next() => new Fixture.Domain.Entity().Next();
#else
    public static int Next() => 0;
#endif
}
