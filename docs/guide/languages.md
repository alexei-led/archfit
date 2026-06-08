# Language support

`archfit` can analyze Go, TypeScript/JavaScript, and Python in the same run.
Enable languages in `.archfit.yaml` with `tools.<language>.enabled`.

```yaml
tools:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto
```

Tool modes:

- `auto` — use the adapter when project markers and tools are found.
- `on` — require the adapter; missing project markers or tools are errors.
- `off` — skip the adapter.

Use `auto` for mixed repos while calibrating. Use `on` in CI when a language must
be analyzed.

## Go

Requirements:

- `go` on `PATH`;
- `go.mod` at the repository root.

How extraction works:

- loads packages with `go/packages`;
- emits file-to-package import edges;
- strips the module path so internal imports become repo-relative paths;
- records import locations when available.

Install/check:

```sh
go version
archfit doctor
```

Example config:

```yaml
tools:
  go:
    enabled: on
  typescript:
    enabled: off
  python:
    enabled: off

layers: [model, core, adapter, cmd]
modules:
  domain:
    paths: [internal/domain/**]
    public: [internal/domain]
    internal: [internal/domain/internal/**]
    layer: model
    subdomain: core
  http:
    paths: [internal/http/**]
    public: [internal/http]
    layer: adapter
    subdomain: supporting
rules:
  - id: domain_no_http
    type: forbidden_dependency
    from: internal/domain/**
    to: internal/http
```

For Go, `public` usually names package import paths, such as `internal/domain`,
not individual `.go` files.

## TypeScript and JavaScript

Requirements:

- `package.json` at the repository root;
- Node.js with `npx`, or Bun with `bunx`;
- dependency-cruiser available locally or through the package runner.

Install:

```sh
npm install --save-dev dependency-cruiser
# or
bun add --dev dependency-cruiser
```

How extraction works:

- runs `bunx depcruise` when `bunx` is available;
- otherwise runs `npx depcruise`;
- reads dependency-cruiser JSON output;
- skips Node.js core modules;
- emits file-to-file dependency edges.

Example config:

```yaml
tools:
  go:
    enabled: off
  typescript:
    enabled: on
  python:
    enabled: off

layers: [domain, app, ui]
modules:
  domain:
    paths: [src/domain/**]
    public: [src/domain/index.ts]
    internal: [src/domain/internal/**]
    layer: domain
    subdomain: core
  ui:
    paths: [src/ui/**]
    public: [src/ui/index.ts]
    layer: ui
    subdomain: supporting
rules:
  - id: domain_no_ui
    type: forbidden_dependency
    from: src/domain/**
    to: src/ui/**
    gate: fail
  - id: no_internal_imports
    type: public_api_only
    gate: fail
```

For TypeScript, module paths and rule filters are repo-relative file path globs.

## Python

Requirements:

- `pyproject.toml`, `setup.py`, or configured `python_package`;
- `uv`, or Python 3.12+ with `grimp` installed.

Recommended install:

```sh
brew install uv
```

How extraction works:

- detects `uv` first;
- with `uv`, runs the helper with `uv run --with grimp`;
- without `uv`, runs Python 3.12+ and expects `grimp` to be installed;
- emits dotted module-to-module import edges, such as `myapp.service`.

Set `python_package` when the top-level package is not the repository directory
name, or when the repo uses `src/` layout.

```yaml
python_package: myapp
tools:
  go:
    enabled: off
  typescript:
    enabled: off
  python:
    enabled: on
```

Example config:

```yaml
python_package: myapp
tools:
  python:
    enabled: on

layers: [domain, app, adapter]
modules:
  domain:
    paths: [myapp.domain**]
    public: [myapp.domain.api**]
    internal:
      - myapp.domain._internal**
      - myapp/domain/_internal/**
    layer: domain
    subdomain: core
  web:
    paths: [myapp.web**]
    public: [myapp.web**]
    layer: adapter
    subdomain: supporting
rules:
  - id: domain_no_web
    type: forbidden_dependency
    from: myapp.domain**
    to: myapp.web**
    gate: fail
  - id: no_private_python_imports
    type: internal_api_access
    gate: fail
```

Python notes:

- dependency nodes are dotted module names;
- use dotted globs for `modules.paths`, `public`, and rule `from`/`to` filters;
- include slash-style `internal` globs too when you want the extractor to mark
  `_internal` packages as internal-access edges;
- imports of underscore-prefixed modules, such as `myapp._internal`, are treated
  as intrusive coupling signals.

## Mixed repositories

For a repo with more than one language, keep each language's paths in distinct
modules where possible:

```yaml
tools:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto

modules:
  api_go:
    paths: [internal/api/**]
    layer: adapter
  web_ts:
    paths: [src/web/**]
    layer: adapter
  jobs_py:
    paths: [myapp.jobs**]
    layer: adapter
```

Run `archfit doctor` before blaming config. Missing optional tools in `auto` mode
produce absent coverage, not a hard failure.
