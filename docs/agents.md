# Coding agent workflows

Pick the section for your role. A worker with a shell runs archfit itself. A
PR-only agent reads the result that the App posts. An owner changes the policy.
An unattended session cannot get an answer from an owner and must stop when it
needs one.

## Worker with a shell

`archfit init` writes these five lines for agents into `AGENTS.md`:

```text
Run `archfit rules` before you start and `archfit check` before you finish.
Do what `next:` says. `archfit: pass` means done.
Before a new dependency between modules: `archfit can <from> <to>`.
If a rule is in your way: `archfit request -m "<why>"`, then wait or do other work.
Never edit `.archfit.yaml`. Never copy code to get around a rule.
```

That is the whole loop:

1. Run `archfit rules` before you start. It lists the rules for the files you
   change, in plain words.
2. Before a new dependency between modules, run `archfit can <from> <to>`.
3. Make the change.
4. Before you finish, run `archfit check`. Do what each `next:` line says.
   `archfit: pass` means done.

The record names the file and line, the rule, what is wrong, why, what to do
instead, and the next step. See [results](results.md) for the record and the
exit codes, and [rules](rules.md) for what each rule checks.

### When a rule is in your way

Ask an owner. Write only the reason:

```sh
archfit request -m "<why you need it>"
```

`request` takes the errors of the last `check`. Then wait, or do other work.
Do not copy code to get around a rule. A copy is an error of the `duplication`
rule.

Never edit `.archfit.yaml` to make a check pass. The policy on the target
branch is the one that counts: a branch that changes the policy gets `unknown`
until an owner approves the change.

### Hooks

Install the hooks once in each checkout:

```sh
archfit doctor --fix
```

It writes the hooks of each harness it finds, at the repository root:
`.claude/settings.json` for Claude Code, `.codex/hooks.json` for Codex, and
the git pre-commit hook. It keeps every other key and hook in the settings
file. Running it again changes nothing. A pre-commit hook that archfit did not
write, or one in a hooks directory outside the repository, stays: doctor prints
the line to add to it. When git ignores a settings file, doctor warns: other
checkouts and worktrees do not get those hooks.

The hooks run steps 2 and 4 of the loop for you. They read `.archfit.yaml` at
the repository root, as `check` does:

| Event | Claude Code | Codex | What it does |
| --- | --- | --- | --- |
| `session-start` | `SessionStart` | `SessionStart` | Records `HEAD` as the session base in `.archfit-cache/session/<session_id>.json`. Prints the five lines above. |
| `post-tool-use` | `PostToolUse` after `Edit`, `Write`, `MultiEdit`, `NotebookEdit`, `Bash` | `PostToolUse` after `apply_patch`, `Bash` | When the working tree changed, asks `archfit can` about each import on a line added since the session base. A `no` exits `2` with the file, the line and the record of `can`. An error that an exception covers is left out, as `check` leaves it out. Each `no` is shown once while it stays, and the first one is stored for `archfit request`. A `yes` or an `unknown` is silent. |
| `stop` | `Stop`, `SubagentStop` | `Stop` | Runs `archfit check`. A `fail` stops the agent once, with the record. The same errors do not stop it again, until a stop passes or gives `unknown`. A `SubagentStop` is stopped once and never uses up the stop of the main agent. An `unknown` shows the record and does not stop. |
| `pre-commit` | — | — | Runs `archfit check` on the staged content against `HEAD`: the errors the commit adds, and an exception past its date. A `fail` stops the commit with the record. An `unknown` shows the record. |

The hooks read changes with git, never from the tool payload, so an edit made
through a shell command counts. They ignore their own writes under
`.archfit-cache/`. A hook that fails for its own reasons (no git repository, a
git error) shows why and never stops the agent or the commit.

The `post-tool-use` hook answers as `can` does: it does not see a cycle the
import would close. The `stop` hook runs `check`, which sees it.

Codex runs a project hook only after you trust it. After `doctor --fix`, start
Codex and approve the hooks, or run it once with
`--dangerously-bypass-hook-trust`.

Check the hooks with `archfit doctor`. It lists the harnesses it finds and
each hook as `active` or `inactive` (not installed, switched off by
`disableAllHooks` in the Claude Code project settings, or, for Codex, not
trusted yet). `archfit doctor --format json` reports the same under
`agent_loop`.

#### Other harnesses

For pi, Cursor, Aider, or a custom runner, call the commands from the
harness's own hook system or from the agent's instructions:

| When | Command |
| --- | --- |
| At session start | `archfit rules` |
| Before a new dependency | `archfit can <from> <to>` |
| Before finishing | `archfit check` |

The git pre-commit hook works with every harness. Install it with
`archfit doctor --fix`, or through the pre-commit framework in
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

## PR-only agent (no shell)

You cannot run archfit. Read the result the App leaves on the pull request:

- Read the pull-request comment. It shows the status of the change, the
  errors, and the next step.
- Read the `archfit/gate` check. `success` is the only state that means the
  change may land.
- Do what `next:` says. Push a fix to the code, not to the policy.
- Never edit `.archfit.yaml` to make a check pass.
- If a rule is in your way, reply on the pull request with
  `/archfit request <why>`, then wait. Do not push a copy of the code to get
  around the rule.

## Owner

The owner writes the policy and answers requests. Workers and PR-only agents
do not. Each row links to the page that holds the detail.

| Owner work | Command or page |
| --- | --- |
| Set up a repository, or convert an Archfit 3 one | `archfit init`. See [getting started](getting-started.md). |
| Read the policy as a worker sees it | `archfit rules`. See [commands](commands.md). |
| Preview a policy change | `archfit check --policy <candidate>`. See [commands](commands.md#check). |
| See the state of the repository | `archfit report`. See [commands](commands.md#report). |
| Write the policy | See [configuration](configuration.md) and [rules](rules.md). |
| Install the session hooks | `archfit doctor --fix` |

Read a policy edit as a policy change, not as code. A pull request that
changes `.archfit.yaml` gets `unknown` until an owner approves it.

## Sessions without a human

An unattended agent cannot get an answer from an owner in the session. Two
results need one:

- A `fail` whose `next:` is `archfit request`: the policy does not allow the
  change. Only an owner can change that.
- An `unknown` whose cause is `policy`: the policy differs from the target
  branch. An owner approves a policy change.

An `unknown` whose `next:` names a tool or a `git fetch` is yours to clear.

When you reach one you cannot clear:

1. Stop. Do not retry the same change.
2. Report the error, the rule, and what you need from the owner. With a shell,
   run `archfit request -m "<why>"`.
3. Leave `.archfit.yaml` as it is.

Never loosen the policy to finish. An edit to `.archfit.yaml` does not
finish the work. It makes the change `unknown`.

A `waiting` check on the pull request means a request is open. It does not ask
you to push again. Wait, or report the state.

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
