public interface IRepo
{
    int Count();
}

public abstract class Base
{
    protected abstract void Hook();

    public Base(int x)
    {
        X = x;
    }

    public int X { get; }

    public int Sum(int[] xs)
    {
        int Twice(int v)
        {
            return v * 2;
        }
        Func<int, int> next = v => v + 1;
        return xs.Select(Twice).Select(next).Sum();
    }

    public int Arrow() => 1;
}
