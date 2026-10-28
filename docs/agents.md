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
- `allowed`: the current allowlist or layer order permits the import, and no
  configured whole-graph rule could still block it.
- `unconstrained`: no rule decides the import. This is not permission.

## Check and repair

Run the agent format after a change:

```sh
archfit check -c .archfit.yaml --format agent
```

Use `--base HEAD` to scope blockers to uncommitted changes. The result has two
parts. `change` says whether your change may land. `repository` lists what is
wrong outside it. Read `change.status` and follow `next_action`; both come from
`change` only:

| `change.status` | `next_action`      | Action                                                                    |
| --------------- | ------------------ | ------------------------------------------------------------------------- |
| `fail`          | `repair`           | Change the source within the listed goal and constraints. Run `validate`. |
| `fail`          | `ask_owner`        | Stop and ask the architecture owner to decide.                            |
| `unknown`       | `restore_evidence` | Install or fix the named analyzer, then run `validate`.                   |
| `pass`          | `none`             | Finish. Exit `2` with `none` is a valid result.                           |

`unknown` never means pass: a required analyzer failed or a required rule has no
producer evidence.

`repository.attention` lists items the change did not cause: an existing
violation, a metric ratchet that cannot be evaluated, a rule selector that
matches nothing. Report them to the owner. Do not act on them.
`identity` names the policy, baseline, labels, tree, and engine the result was
judged under.

Use `edit` to find source files. Do not edit the target of a forbidden edge.
An empty `edit` means the source file is unknown. Use `edge.from` to identify
the source.

Do not change the config, baseline, waivers, or labels to make a check pass.
Do not change `baseline` or `waived` findings without an explicit request.
See [results](results.md) for the output contracts and [rules](rules.md) for
policy behavior.

Read repair advice within its evidence and role limits:

- API ceiling findings request source review. Only the architecture owner may
  change the threshold.
- Metric ratchets compare against the accepted baseline. That difference alone
  does not establish which PR introduced it.
- Clone advisories request inspection of whether similar code represents shared
  knowledge that must evolve together. Extraction and label acceptance are not
  default repairs.

## Install hooks

Install the hooks of your harness:

```sh
archfit hook install --harness claude   # .claude/settings.json
archfit hook install --harness codex    # .codex/hooks.json
archfit hook install --harness git      # the git pre-commit hook
```

`install` merges into the settings file and keeps every other key and hook.
Running it again changes nothing. It removes entries of the old
`archfit hook claude` command. Pass `-c <config>` when the config is not
`.archfit.yaml` at the repository root; the path is relative to the root, so a subproject's config is `-c service/.archfit.yaml` even when you run `install` from `service/`. `install` warns when the hooks would load another config, or none. Pass `--dir <repo>` to install into another repository. A
pre-commit hook that archfit did not write is left alone: `install` exits `3`
and names it.

The installed hooks call `archfit hook run <event>`:

| Event | Claude Code | Codex | What it does |
| --- | --- | --- | --- |
| `session-start` | `SessionStart` | `SessionStart` | Records `HEAD` as the session base in `.archfit-cache/session/<session_id>.json`. Prints three lines for the agent: run `policy can-import` before a cross-module import, run `check --format agent` before finishing, never edit the policy files. |
| `post-tool-use` | `PostToolUse` after `Edit`, `Write`, `MultiEdit`, `NotebookEdit`, `Bash` | `PostToolUse` after `apply_patch`, `Bash` | Finds the files that differ from the session base with git, reads the imports their added lines introduce (Go, relative TypeScript and JavaScript, Python), and asks `can-import` about each. A denial exits `2` with the file, the target, the rule, why, and what to use instead on stderr. Each denial is reported once while it stays; an import removed and added again is reported again. A relative import of a file that does not exist yet is judged once the file exists. |
| `stop` | `Stop`, `SubagentStop` | `Stop` | Runs `check --format agent --base <session base>` with the policy guard of `check`. A `fail` change blocks the stop once, with the blockers on stderr. The same blockers on the same tree do not block again, until a stop passes. A `SubagentStop` is blocked once and never suppresses the main agent's stop. An `unknown` change shows a message and does not block. |
| `pre-commit` | — | — | Checks the staged content against `HEAD`. Exit `1` blocks the commit. |

The hooks read changes with git, never from the tool payload, so an edit made
through a shell command counts. A commit made during the session does not hide
a violation: the stop compares with the session base, not with `HEAD`. The
hooks ignore their own writes under `.archfit-cache/`. An archfit error in
`post-tool-use` or `stop` fails open with a message and never blocks the
agent.

Codex runs a project hook only after you trust it. After `install`, start
Codex and approve the hooks, or run it once with
`--dangerously-bypass-hook-trust`.

Check the loop with `archfit doctor`. Its agent loop section lists the
harnesses it finds and each hook as `active` or `inactive` (not installed,
switched off by `disableAllHooks` in the Claude Code project settings, or, for
Codex, not trusted yet). It also shows the target branch, the scope `check`
would use, and whether a policy file differs from the target.
`archfit doctor --format json` reports the same under `agent_loop`.

Generate or check the `AGENTS.md` block with `archfit agents-md --write` and
`archfit agents-md --check`. The generated block includes current project
modules and architecture rules.

### Other harnesses

For pi, Cursor, Aider, or a custom runner, call these commands from the
harness's own hook system or from the agent's instructions:

| When | Command |
| --- | --- |
| At session start | `archfit doctor` |
| After an edit | `archfit policy can-import <file> <target>` |
| Before finishing | `archfit check --base <sha> --format agent`, where `<sha>` is the commit the session started from |

The git pre-commit hook works with every harness. Install it with
`archfit hook install --harness git`, or through the pre-commit framework in
`.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/alexei-led/archfit
    rev: v3.1.1
    hooks:
      - id: archfit
```

Install the archfit binary first, then activate the framework hook:

```sh
pre-commit install
```

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
