# Getting started

This guide installs archfit, creates a starter configuration, and runs a first analysis. For full tool requirements, see [Language support](languages.md).

## Install archfit

On macOS or Linux, install the latest release with Homebrew:

```sh
brew install alexei-led/tap/archfit
```

With Go, install a tagged release:

```sh
go install github.com/alexei-led/archfit/v3/cmd/archfit@v3.1.0
```

You can also download a release binary from the [release page](https://github.com/alexei-led/archfit/releases). Check the file against `SHA256SUMS` before you install it.

## Check the tools

Run:

```sh
archfit doctor
```

This command reports the tools that archfit can find. Install the tools for the languages in your repository. A missing optional tool can leave some evidence incomplete. See [Language support](languages.md#analyzer-tools).

## Create a configuration

From your repository root, run:

```sh
archfit config init --root .
```

This command writes `.archfit.yaml`. Review the module `paths`, layer assignments, languages, and rules. Generated paths can include files that do not belong to a module you intend to check.

Read [Configuration](configuration.md) for field meanings and defaults. Use [Rules](rules.md) to choose boundaries and gates.

## Review the first report

Run:

```sh
archfit analyze -c .archfit.yaml
```

`analyze` prints a report and exits `0` when the analysis completes. Review its verdict, coverage, and findings. Fix incorrect paths or missing tools before you accept the result.

## Add a baseline and CI check

Create a baseline only after you review the report:

```sh
archfit baseline -c .archfit.yaml
```

A baseline records reviewed findings as accepted state. Do not create one to hide findings that you have not reviewed.

Run the gate locally or in CI:

```sh
archfit check -c .archfit.yaml
```

The command returns `0` for healthy, `1` for blocked, `2` for needs attention, and `3` for a configuration, tool, or runtime error. During adoption, CI can fail only on blockers:

```sh
archfit check -c .archfit.yaml || [ "$?" -eq 2 ]
```

This shell expression accepts exit `2`. It still fails on exit `1` or `3`. See [CI](ci.md) for stable workflows and stricter policies.

## Use Docker

The release image runs archfit with a bundled toolchain:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:v3.1.0 \
  check --config /repo/.archfit.yaml
```

The image includes Go, Node.js 24, dependency-cruiser, ast-grep (`sg`), Python 3, and `uv`. Python analysis resolves `grimp` at run time.

The image does not include Rust tools. Rust analysis reports unavailable evidence when `cargo` is absent. Use a host with Rust tools or build a derived image when Rust analysis is required.

## More help

- [Commands](commands.md) — flags and exit codes.
- [Configuration](configuration.md) — configuration fields and validation.
- [Troubleshooting](troubleshooting.md) — common setup and analysis issues.
