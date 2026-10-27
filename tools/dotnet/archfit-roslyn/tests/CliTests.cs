using System.Diagnostics;
using System.Text.Json;

namespace ArchfitRoslyn.Tests;

// The stdout/stderr contract, through a real helper process.
public class CliTests
{
    private static readonly string Helper = Path.Combine(AppContext.BaseDirectory, "archfit-roslyn.dll");

    [Fact]
    public async Task VersionPrintsTheProducerIdentity()
    {
        var (code, stdout, _) = await Run("--version");

        Assert.Equal(0, code);
        Assert.Matches(@"^archfit-roslyn 1\.0\.0; Microsoft\.CodeAnalysis 5\.6\.0; SDK \d+\.\d+\.\d+\S*\n$", stdout.ReplaceLineEndings("\n"));
    }

    [Theory]
    [InlineData()]
    [InlineData("/nonexistent-archfit-root", "x.slnx")]
    [InlineData(".", "missing.csproj")]
    [InlineData(".", "a.slnx", "b.slnx")]
    public async Task ErrorsGoToStderrWithNoJson(params string[] args)
    {
        var (code, stdout, stderr) = await Run(args);

        Assert.NotEqual(0, code);
        Assert.Equal("", stdout);
        Assert.NotEqual("", stderr.Trim());
    }

    [Fact]
    public async Task PrintsOneJsonDocument()
    {
        var copy = Path.Combine(Path.GetTempPath(), "archfit-roslyn-cli-" + Guid.NewGuid().ToString("N"));
        LoadedFixture.CopyTree(LoadedFixture.Source, copy);
        try
        {
            var (code, stdout, _) = await Run(copy, "Domain/Domain.csproj");

            Assert.Equal(0, code);
            using var json = JsonDocument.Parse(stdout);
            Assert.Equal(1, json.RootElement.GetProperty("schema_version").GetInt32());
            var project = Assert.Single(json.RootElement.GetProperty("projects").EnumerateArray());
            Assert.Equal(("Domain/Domain.csproj", "complete"), (project.GetProperty("path").GetString(), project.GetProperty("status").GetString()));
        }
        finally
        {
            Directory.Delete(copy, recursive: true);
        }
    }

    [Theory]
    [InlineData(true)]
    [InlineData(false)]
    public async Task UnsatisfiableGlobalJsonIsOneStderrLine(bool version)
    {
        var dir = Path.Combine(Path.GetTempPath(), "archfit-roslyn-globaljson-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        File.WriteAllText(Path.Combine(dir, "global.json"), """{"sdk":{"version":"9.0.100","rollForward":"disable"}}""");
        try
        {
            var (code, stdout, stderr) = version
                ? await RunIn(dir, "--version")
                : await RunIn(AppContext.BaseDirectory, dir, "A.csproj");

            Assert.Equal(1, code);
            Assert.Equal("", stdout);
            var line = Assert.Single(stderr.ReplaceLineEndings("\n").TrimEnd('\n').Split('\n'));
            Assert.Equal("archfit-roslyn: global.json requests SDK 9.0.100, which is not installed; install a matching SDK or relax the pin", line);
        }
        finally
        {
            Directory.Delete(dir, recursive: true);
        }
    }

    private static Task<(int Code, string Stdout, string Stderr)> Run(params string[] args) =>
        RunIn(AppContext.BaseDirectory, args);

    private static async Task<(int Code, string Stdout, string Stderr)> RunIn(string workDir, params string[] args)
    {
        var start = new ProcessStartInfo("dotnet")
        {
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            WorkingDirectory = workDir,
        };
        start.ArgumentList.Add(Helper);
        foreach (var arg in args)
        {
            start.ArgumentList.Add(arg);
        }
        using var process = Process.Start(start)!;
        var stdout = process.StandardOutput.ReadToEndAsync();
        var stderr = process.StandardError.ReadToEndAsync();
        await process.WaitForExitAsync();
        return (process.ExitCode, await stdout, await stderr);
    }
}
