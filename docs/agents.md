# Coding agent workflows

Use `archfit policy` before a cross-module import. Use `archfit check` after a
change. The check reports the verdict and any repair tasks.

## Before an import

Find the source module and check the proposed import:

```sh
archfit policy where path/to/source.go
archfit policy can-import path/to/source.go path/to/target
```

`where` reports the module, layer, allowlists, and matching rules. `can-import`
returns `denied`, `not_decided`, `allowed`, or `unconstrained`.

- `denied`: do not add the import. Use the reported repair goal and constraints.
- `not_decided`: a whole-graph rule or missing evidence can still block it.
  Add the import only if needed, then run `check`.
- `allowed`: the current allowlist or layer order permits the import.
- `unconstrained`: no rule decides the import. This is not permission.

## Check and repair

Run the agent format after a change:

```sh
archfit check -c .archfit.yaml --format agent
```

Use `--base HEAD` to scope repairs to uncommitted changes. Read the command
status with `next_action`:

| `next_action` | Action |
| --- | --- |
| `repair` | Change the source within the listed goal and constraints. Run `validate`. |
| `ask_owner` | Stop and ask the architecture owner to decide. |
| `restore_evidence` | Install or fix the named analyzer, then run `validate`. |
| `report_blocked` | Report blockers outside the change. |
| `none` | Finish. Exit `2` with `none` is a valid result. |

Use `edit` to find source files. Do not edit the target of a forbidden edge.
An empty `edit` means the source file is unknown. Use `edge.from` to identify
the source.

Do not change the config, baseline, waivers, or labels to make a check pass.
Do not change `baseline` or `waived` findings without an explicit request.
See [results](results.md) for the output contracts and [rules](rules.md) for
policy behavior.

## Install hooks

Add the Claude Code stop hooks to `.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [{ "hooks": [{ "type": "command", "command": "archfit hook claude" }] }],
    "SubagentStop": [{ "hooks": [{ "type": "command", "command": "archfit hook claude" }] }]
  }
}
```

The hook checks changes against `HEAD`. It blocks one stop when an in-scope
repair needs a code change or an owner decision. It fails open when archfit
returns an error.

For a pre-commit hook, add archfit to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/alexei-led/archfit
    rev: v3.1.0
    hooks:
      - id: archfit
```

Install the archfit binary first. The hook checks staged files.

Activate the Git hook after adding the configuration:

```sh
pre-commit install
```

Generate or check the `AGENTS.md` block with `archfit agents-md --write` and
`archfit agents-md --check`. The generated block includes current project
modules and architecture rules.

## Install the portable skill

Install the skill shipped in the binary:

```sh
archfit skill install
```

The command writes `.claude/skills/archfit`. Use `--dir` for another skills
directory. It preserves local changes unless you pass `--force`.

The repository also keeps portable skill sources under `skills/archfit/`.
Point an agent harness at that directory or copy it into the harness's skill
directory. Read the skill's [instructions](../skills/archfit/SKILL.md).
