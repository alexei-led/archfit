using System.Diagnostics;
using System.Text.RegularExpressions;

namespace ArchfitRoslyn;

// Sdk turns "no SDK resolvable from this directory" into one stderr line that
// names the cause (usually a global.json pin), instead of a resolver exception.
internal static partial class Sdk
{
    // EnsureResolvable asks the muxer which SDK it resolves from dir before
    // MSBuildLocator does. The locator's native resolver prints the installed
    // SDKs to stdout and throws a multi-line exception when the pin cannot be
    // met; the child process keeps that output off the helper's stdout.
    public static void EnsureResolvable(string dir)
    {
        var start = new ProcessStartInfo(Muxer())
        {
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            WorkingDirectory = dir,
        };
        start.ArgumentList.Add("--version");
        using var process = Process.Start(start)
            ?? throw new UserError("cannot start dotnet to resolve the SDK; install the .NET SDK");
        var stdout = process.StandardOutput.ReadToEndAsync();
        var stderr = process.StandardError.ReadToEndAsync();
        process.WaitForExit();
        if (process.ExitCode != 0)
        {
            throw new UserError(Unresolvable(stderr.Result + "\n" + stdout.Result, dir));
        }
    }

    // Unresolvable names why no SDK resolves from dir, on one line.
    internal static string Unresolvable(string text, string dir)
    {
        var file = GlobalJsonFile().Match(text);
        if (file.Success)
        {
            var pin = Display(file.Groups[1].Value.Trim(), dir);
            var version = RequestedVersion().Match(text);
            return version.Success
                ? $"{pin} requests SDK {version.Groups[1].Value}, which is not installed; install a matching SDK or relax the pin"
                : $"{pin} cannot be satisfied by the installed SDKs; install a matching SDK or relax the pin";
        }
        var first = text.Split('\n').Select(l => l.Trim()).FirstOrDefault(l => l.Length > 0) ?? "no output";
        return $"no .NET SDK resolvable from {dir}: {first}; install the .NET SDK";
    }

    // The muxer that runs this helper (`dotnet archfit-roslyn.dll`), else the
    // one on PATH (apphost launch).
    private static string Muxer() =>
        Environment.ProcessPath is { } path && Path.GetFileNameWithoutExtension(path) == "dotnet" ? path : "dotnet";

    // Display spells the pin relative to dir when it is dir's own global.json:
    // the resolver takes the nearest one walking up, so that is the one it
    // read. Comparing paths would miss symlinked spellings (macOS /var).
    private static string Display(string path, string dir) =>
        Path.GetFileName(path) == "global.json" && File.Exists(Path.Combine(dir, "global.json")) ? "global.json" : path;

    [GeneratedRegex(@"(?m)^global\.json file:\s*(.+?)\s*$")]
    private static partial Regex GlobalJsonFile();

    [GeneratedRegex(@"(?m)^Requested SDK version:\s*(\S+)")]
    private static partial Regex RequestedVersion();
}
