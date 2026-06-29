# Doctor --fix design

Design for a cross-platform `archfit doctor --fix` that detects platform, package
managers, and PATH issues, then applies safe, reversible fixes. Builds on the
[Tooling reference](../guide/tooling.md).

## Status

Proposal — not implemented.

## Motivation

`archfit doctor` is read-only and prints install hints that are platform-agnostic
one-liners. `archfit install` bootstraps only `uv`/`node` via Homebrew. Both are
too narrow:

- No Linux distro detection — hints assume `brew` or a URL.
- No identity verification — util-linux `sg` and Homebrew's compression `lizard`
  pass the `Detect` check.
- No PATH analysis — package managers can install tools correctly while the
  shell still cannot find them because a bin directory is missing from `PATH`.
- No `--fix` — the user must copy hints by hand.

## Design

Add a single tool catalog. `doctor`, `install`, and coverage-gap hints all read
from it. Then add platform/package-manager detection, PATH analysis, and a safe
`--fix` mode.

### Tool catalog

```
type ToolSpec struct {
    ID          string           // e.g. "sg", "lizard"
    Label       string           // "sg (ast-grep)"
    Binary      string           // probe command
    DocsURL     string           // upstream homepage
    RequiredFor []string
    Optional    bool
    Identity    func(out) bool   // verify it is the right binary
    MinVersion  string           // semver floor
    Methods     []InstallMethod
}

type InstallMethod struct {
    Platform string   // darwin, linux-debian, linux-fedora, any
    Manager  string   // brew, apt, dnf, npm, uv, go, cargo, rustup, url
    Command  []string
    AutoFix  bool
    Rank     int
}
```

The catalog replaces the scattered `doctorTool` struct + `install.go` methods.
`doctor`, `install`, and coverage-gap messages all source install commands from
it. Adding a tool is one entry.

### Platform detection

Run `runtime.GOOS`/`GOARCH` for the OS.

On Linux, parse `/etc/os-release`:

- `ID=debian`/`ID=ubuntu`/`ID_LIKE=debian` → `linux-debian`.
- `ID=fedora`/`ID=rhel`/`ID_LIKE=fedora` → `linux-fedora`.

Detect package managers available on the current machine:

- macOS: `brew`.
- Linux: `apt` and/or `dnf` (both may exist on the same machine).
- Fallback: `npm`, `cargo`, `go`, `uv` are cross-platform.

### Identity probes

Some commands have name collisions. `doctor` must verify, not only detect:

| Binary   | Check                                         | Reason                       |
| -------- | --------------------------------------------- | ---------------------------- |
| `sg`     | `sg --version` contains `ast-grep`            | util-linux `/usr/bin/sg`     |
| `lizard` | `lizard --version` contains `1.x` (not `2.x`) | Homebrew `lizard` compressor |

Probe result states:

- `ok` — found and identity matches.
- `missing` — not on `PATH`.
- `wrong-tool` — found but identity probe fails.
- `too-old` — found but version below floor.
- `path-missing` — installed but bin directory not in `PATH`.
- `shadowed` — wrong binary on `PATH` before the correct one.
- `blocked` — config/install conflict (e.g. util-linux `sg` at `/usr/bin`).
- `optional-missing` — optional tool not installed.
- `disabled-by-config` — `tools.<x>.enabled: off` so not probed.

### PATH analyser

A platform-independent probe that checks known bin directories:

```
knownBins = []string{
    brewPrefix + "/bin", brewPrefix + "/sbin",
    npmGlobalPrefix + "/bin",
    GOPATH + "/bin", GOBIN,
    "$HOME/.cargo/bin", "$CARGO_HOME/bin",
    uvToolDir + "/bin",
    "$HOME/.local/bin",
    "/usr/local/bin", "/usr/bin",
}
```

For each directory:

- Report whether it exists. If it does, report whether it appears in `PATH`.
- Never edit `PATH` automatically. Report the missing entry with a copyable
  shell command.

`exec.LookPath` + a `LookAll` helper that walks `PATH` to find every `sg`
on the system, then verifies each. `LookAll` also catches the case where
`$HOME/.cargo/bin/sg` (ast-grep) is on `PATH` but `/usr/bin/sg` (util-linux)
is found first.

### Output

Default table with status, version, and action:

```text
Platform: darwin/arm64
Package manager: brew /opt/homebrew
Shell: zsh
Config: .archfit.yaml

TOOL              NEED                 STATUS       VERSION   PATH / ACTION
git               change history       ok           2.54.0    /opt/homebrew/bin/git
go                Go analysis          ok           1.26.4    /opt/homebrew/bin/go
node              TS launcher          ok           26.3.1    /opt/homebrew/bin/node
sg                structural patterns  ok           0.44.0    /opt/homebrew/bin/sg
lizard            complexity optional  wrong-tool   2.1.0     /opt/homebrew/bin/lizard
  expected: PyPI lizard complexity analyzer
  fix:      uv tool install 'lizard==1.23.0'
  doc:      http://www.lizard.ws/
cargo-modules     Rust module graph    missing      —         cargo install cargo-modules --version 0.26.0

PATH issues:
  warn: /Users/me/.cargo/bin is a valid cargo install target but is not in PATH
  add:  export PATH="$HOME/.cargo/bin:$PATH"
```

Add `--format json` for agent/CI consumption.

### `doctor --fix`

Keep `archfit install` as a convenience, but both route through the same planner.

Flags:

```
--fix                      Apply safe fixes.
--fix --dry-run            Print what would be done, do nothing.
--fix --yes                Apply without interactive prompts.
--fix --tool <name>        Fix one tool.
--fix --lang <name>        Fix all tools for a language.
--fix --allow-sudo         Allow `sudo apt install` / `sudo dnf install`.
--fix --allow-remote       Allow `curl | sh` installers.
--fix --path-only          Only fix PATH entries, skip tool installs.
```

Safety rules:

1. `doctor` without `--fix` never mutates.
2. `--fix --dry-run` prints exact commands and shell-file edits.
3. `--fix --yes` can run safe user-scoped or package-manager installs.
4. Do not run `curl | sh` unless `--allow-remote`.
5. Do not run `sudo` unless `--allow-sudo`.
6. Do not mutate `package.json` (dependency-cruiser is project-local by policy).
7. PATH edits only touch user shell files (`.zshrc`/`.bash_profile`/`.profile`):
   - Detect shell from `$SHELL` environment variable.
   - Prefer `~/.zshrc` for zsh, `~/.bash_profile` for bash.
   - Back up the file before editing.
   - Add only missing directories, idempotent markers.
   - Never remove or reorder existing entries.
8. Report blocked actions (needs `--allow-sudo` or `--allow-remote`).

Example plan output:

```text
Plan:
  install:
    npm install -g jscpd@5.0.11
  path:
    add /Users/me/.cargo/bin to ~/.zshrc
Blocked:
  rustup installer requires --allow-remote
  set --allow-remote and re-run, or install rustup manually from https://rust-lang.org/tools/install/
```

Exit codes:

- `0` — all tools ok, or only optional tools missing.
- `1` — required tool missing (only with `--fix` failure or explicit gate).
- `3` — config or runtime error.

## Implementation order

1. Add `internal/doctor/` with tool catalog, platform detector, PATH analyser.
2. Add identity probes (`sg`/`lizard`).
3. Replace `doctor.go` table with catalog-driven probes.
4. Rewire `install.go` to use the same planner.
5. Add `--fix` with dry-run/yes/safety flags.
6. Add `--format json`.
7. Update `doctor` tests.

## Open questions

- **`--fix --path-only` output format:** just print the shell snippet? Or edit
  the shell file? Editing the shell file is safer when the user runs `--fix --yes`
  directly. Print-only is better for CI/scripts.
- **Config awareness:** default `doctor` probes tools relevant to the repo
  (based on project markers + config). `--all` shows every known tool. Agreed;
  `--all` is the escape hatch.
- **Homebrew Linux prefix:** Homebrew on Linux installs to
  `/home/linuxbrew/.linuxbrew`. Detect with `brew --prefix`, fall back to the
  known path.
