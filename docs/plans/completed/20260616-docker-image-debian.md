# Prompt: build the archfit Docker image (Debian-minimal, glibc)

Paste this into a fresh session (Docker is available locally via colima — see "Local
env" below). Goal: a working, minimal **Debian** image that bundles every tool
archfit needs to run all checks, validated locally, then published via the release
workflow.

---

## Objective

Produce a single, minimal **Debian-based (glibc)** runtime image at the repo
`Dockerfile` so `docker run ghcr.io/alexei-led/archfit <args>` can run the full
`archfit` pipeline (Go, TypeScript, Python) with no manual tool installs. Then
re-enable the release workflow's Docker jobs and confirm a tagged release pushes a
multi-arch image to GHCR.

## Constraints (from the maintainer)

- **Use a Debian base, not Alpine.** Alpine/musl blocked ast-grep and complicates
  grimp/uv. Prefer a **minimal** image with a standard C library (glibc) and a
  package manager (`apt`): `debian:bookworm-slim` (or `debian:trixie-slim`).
- **One image, not per-language** — archfit needs the Go binary + Python + Node
  toolchains together for a single analysis run (confirmed by the architecture
  advisor; mirrors megalinter's single-canonical-build model).
- Keep it as small as the toolchain allows; multi-stage build; non-root user.

## Required runtime tools (verify with `archfit doctor` inside the image)

Grounded in the extractors (`internal/extract/*`) and `cmd/archfit/doctor.go`:

| Tool                                                                        | Why                                                                | Notes                                                                                                                                                                                                                                                                                             |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go`                                                                        | Go extractor uses `go/packages`, which **shells out to `go list`** | copy the toolchain from a `golang:1.26-bookworm` builder stage into the final image; set `GOTOOLCHAIN=local`, and `GOCACHE`/`GOPATH` to a world-writable path (`/tmp/...`) so the non-root user can run `go list`                                                                                 |
| `git`                                                                       | repo root + change history                                         | `apt-get install git`                                                                                                                                                                                                                                                                             |
| `node` + `npx`                                                              | TS analysis launcher                                               | `apt-get install nodejs` (NodeSource setup_22.x)                                                                                                                                                                                                                                                  |
| `dependency-cruiser`                                                        | TS import analysis (`npx depcruise`)                               | `npm install -g dependency-cruiser@17` (or rely on `npx` fetch; maintainer prefers bundling)                                                                                                                                                                                                      |
| `sg` (ast-grep)                                                             | structural pattern checks; archfit invokes the **`sg`** binary     | `npm install -g @ast-grep/cli` works on **glibc** (unlike musl). **Conflict:** debian ships util-linux `/usr/bin/sg`; `@ast-grep/cli` postinstall fails `EEXIST`. Fix: `rm -f /usr/bin/sg` **before** the npm install so ast-grep owns `/usr/bin/sg`. Verify `sg --version` prints `ast-grep ...` |
| `python3` + `uv`                                                            | Python import analysis                                             | archfit runs `uv run --with grimp` (grimp resolved at runtime). Use debian's `python3` (3.11 on bookworm — grimp supports it; no need to chase 3.12) + the static `uv` binary. Set `UV_PYTHON_DOWNLOADS=never`, `UV_SYSTEM_PYTHON=1`                                                              |
| (optional) scip-go / scip-typescript / scip-python, lizard, jscpd, gitnexus | opt-in only (`enabled: false` defaults)                            | **leave out** — they degrade to `absent` gracefully; users compose them in a derived image                                                                                                                                                                                                        |

## Known pitfalls (already hit this session — do NOT rediscover)

1. **`COPY --from=ghcr.io/astral-sh/uv:${UV_VERSION}` fails** — buildkit rejects
   variable expansion in `--from`. Alias it: `FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-bin`
   then `COPY --from=uv-bin /uv /uvx /usr/local/bin/`. (The `uv` binary is static — runs on debian-slim.)
2. **`go.work` breaks the container build** — it `use`s `./testdata/...` modules that
   `.dockerignore` excludes, so `go build` fails resolving the workspace. Build with
   **`GOWORK=off`** (the binary only needs the main module).
3. **python3.12 is NOT in debian bookworm apt** (bookworm ships 3.11). Either use
   debian's `python3` (3.11 — fine for grimp), or base on `python:3.12-slim-bookworm`
   if 3.12 is truly required. Don't `apt-get install python3.12` on bookworm.
4. **grimp pre-install is bypassed** by `uv run --with grimp` (it builds its own env),
   and trips PEP 668 ("externally managed") if you `uv pip install --system`. For
   v0.1.x just ship `python3` + `uv` and let grimp resolve at runtime (online). True
   offline grimp = pre-warm the uv cache for the non-root user (a later refinement).
5. **`sg` name clash** (pitfall in the table) — `rm -f /usr/bin/sg` before installing
   `@ast-grep/cli`.
6. **Release build is multi-arch** (linux/amd64 + linux/arm64 via buildx) — test both
   if you can; at minimum build your native arch locally and reason about the other.
7. **`doctor`'s `sg` check can false-positive** on util-linux `/usr/bin/sg`; confirm
   the real ast-grep with `docker run --entrypoint sg <img> --version`.

## Advisor guidance (optional slimming, not required to ship)

- Dropping the global `dependency-cruiser` and relying on `npx` on-demand cuts ~150 MB
  but needs network on first TS run; the maintainer prefers bundling, so keep it unless
  size is a priority.
- Keep optional tools (lizard/jscpd/scip/gitnexus) OUT; document a derived-image recipe
  for users who enable them.

## Local env (already set up on this machine)

- Docker via **colima** with the **docker** runtime is running; `docker` + `docker
buildx` (v0.34.1) installed via brew; `~/.docker/config.json` had its stale
  `osxkeychain` credStore removed.
- Build with BuildKit: `docker buildx build --load -t archfit:dev .`
  (plain `docker build` falls back to the legacy builder and fails on `--platform`/`--mount`).

## Approach

1. Start from the committed Alpine `Dockerfile` as a structural reference, but rewrite
   the final stage on `debian:bookworm-slim` (keep the `uv-bin` and `golang:1.26-bookworm`
   builder stages; the Go binary is static via `CGO_ENABLED=0`).
2. Install via apt: `ca-certificates git curl gnupg python3` + NodeSource `nodejs`.
3. `rm -f /usr/bin/sg` then `npm install -g dependency-cruiser@17 @ast-grep/cli`.
4. `COPY --from=uv-bin` the uv binary; `COPY --from=go-builder` the archfit binary AND
   the `/usr/local/go` toolchain; set `PATH`, `GOTOOLCHAIN=local`, `GOCACHE`/`GOPATH`.
5. Non-root user; `ENTRYPOINT ["/usr/local/bin/archfit"]`.

## Validation (must pass before re-enabling release Docker)

1. `docker buildx build --load -t archfit:dev .` — clean build.
2. `docker run --rm archfit:dev doctor` — `go, git, node, npx, uv, python3, sg` all **ok**;
   `sg --version` is ast-grep.
3. Functional smoke per language (mount a tiny git repo, `-w /src`, run `check --full`):
   a **Go** module, a **TS** project (`package.json`), and a **Python** package
   (`pyproject.toml`). Each must produce real coverage, not `absent`. Note: archfit
   requires the target to be a git repo (`git init` + a commit); running non-root on a
   host-mounted dir may need `git config --global --add safe.directory '*'` or matching uid.
4. Confirm image size is reasonable (debian-slim multi-tool ~600–900 MB is expected with the Go SDK).

## Release wiring (to publish the image)

`.github/workflows/release.yaml` Docker jobs are gated behind `vars.PUBLISH_DOCKER`
(currently off → v0.1.0 shipped binaries only). To ship the image in the next release:

1. Get the build + validation above green.
2. Re-add `merge-docker` to the `release` job's `needs:`.
3. Set repo variable `PUBLISH_DOCKER=true` (`gh variable set PUBLISH_DOCKER --body true`)
   or remove the `if:` guards on `build-docker`/`merge-docker`.
4. Tag `vX.Y.Z` → the workflow builds linux/amd64+arm64, pushes by digest, merges the
   manifest to `ghcr.io/alexei-led/archfit:<tag>` + `:latest`, and attaches binaries.

## Done criteria

- `Dockerfile` is debian-minimal, builds via buildx, `doctor` all-green (real ast-grep).
- Functional smoke passes for Go + TS + Python targets.
- Release Docker jobs build both arches and publish to GHCR on tag.
