namespace ArchfitRoslyn.Tests;

public class SdkTests
{
    private const string Resolver = """
        A compatible .NET SDK was not found.

        Requested SDK version: 9.0.100
        global.json file: /repo/global.json

        Installed SDKs:
        """;

    [Theory]
    [InlineData(Resolver, "/repo/global.json requests SDK 9.0.100, which is not installed; install a matching SDK or relax the pin")]
    [InlineData("global.json file: /repo/global.json\n", "/repo/global.json cannot be satisfied by the installed SDKs; install a matching SDK or relax the pin")]
    [InlineData("\n  boom  \nmore\n", "no .NET SDK resolvable from /nonexistent-archfit-dir: boom; install the .NET SDK")]
    public void UnresolvableIsOneLineNamingTheCause(string text, string want)
    {
        Assert.Equal(want, Sdk.Unresolvable(text, "/nonexistent-archfit-dir"));
    }
}
