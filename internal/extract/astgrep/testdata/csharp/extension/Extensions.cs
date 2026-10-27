public static class StringExtensions
{
    extension(string s)
    {
        public int Length2 => s.Length;
    }
}

public class Ok
{
    [HttpGet("kept")]
    public void Kept() {}
}
