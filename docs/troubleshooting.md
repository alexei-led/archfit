# Troubleshooting

Start with `archfit doctor` for tool and hook status, then follow the failed command's `next:` line.

## Unexpected exit status

| Exit | Meaning                                                    | Action                                                                              |
| ---- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `0`  | Pass, yes, or done.                                        | No repair is needed. `report` can show errors and still exit 0: it decides nothing. |
| `1`  | The change fails a rule, or a dependency is not allowed.   | Follow `next:`: fix the code or ask an owner.                                       |
| `2`  | Unknown, wrong usage, a tool error, or an interrupted run. | Read stderr and the record. Fix the named cause, then run again.                    |

Unknown commands and flags also exit `2`. Missing evidence never passes. See [Commands](commands.md#exit-codes).

## Missing or version 2 policy

Run `archfit init` in the repository. It creates a version 3 policy or converts an Archfit 3 one. Read the result, then open a pull request with it. `check` no longer runs an Archfit 3 check.

A path glob that matches nothing, an unknown module, or a malformed rule stops the check. Workers must not change the policy to pass: use `archfit request -m "<why>"` when the approved architecture must change.

See [Migration](migration.md) and [Configuration](configuration.md).

## Policy differs from the target

A code change cannot judge itself under a policy it changes. Open a separate policy pull request for an owner, or bring the approved target policy into the branch when it is stale.

An owner can preview a candidate with `archfit check --policy candidate.yaml`. A preview gives no verdict and saves no output for `archfit request`.

## Target branch cannot be resolved

Fetch the target history, then run `archfit check --base origin/main` (use the repository's target branch name). CI needs enough history to find the merge base. See [CI](ci.md).

## Incomplete evidence

| Symptom                                         | Action                                                                                                  |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| A file is listed as not read.                   | Fix its syntax, permissions, language settings, or the missing tool named in the record.                |
| No source file matches a module path.           | Check the repository selected by `-C` and the policy's `paths`. Ask an owner if the policy must change. |
| A report measurement is partial or unavailable. | Read `evidence not complete:`. Optional report tools do not affect the check.                           |
| `sg` exists but extraction fails.               | Run `sg --version`: another program named `sg` may precede ast-grep on `PATH`.                          |

Run `archfit doctor` to see the check and report tool groups. See [Language support](languages.md#analyzer-tools).

## Installed tool is missing

Archfit uses the current process's `PATH`. Use `command -v <tool>` and `which -a <tool>` to find which executable runs first. Fix the order if it selects an old or incorrect tool.

`archfit doctor --fix` installs supported missing toolchains and hooks, not every optional report tool.

## Hooks do not run

Run `archfit doctor` to see the detected harnesses and why each hook is inactive. Then run `archfit doctor --fix`.

Check Claude Code's project and local settings for disabled hooks. For Codex, trust the installed hooks. Settings ignored by Git do not reach other clones or worktrees. Archfit leaves foreign pre-commit hooks and shared hooks directories alone; doctor prints what to add instead.

## Stale request output

Run `archfit check` or `archfit can <from> <to>` after each edit. Requests use only output for the current tree. A pass, an unknown result, or an error that requires a code fix gives nothing an owner may allow.

Without the App, the request prints `next: ask an owner` and exits `2`. See [Commands](commands.md#request).

## Clear cached facts

To clear local cached extraction results, delete only `.archfit-cache/facts/` at the repository root. The next run rebuilds them. The check has no `--refresh` flag.
