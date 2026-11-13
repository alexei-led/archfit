---
name: archfit
description: Keep a code change inside the repository's architecture policy with archfit. Use when a repository has .archfit.yaml and you add imports, move code between modules, or finish a change. Not for writing or approving the policy itself (owners: see docs/agents.md).
license: Apache-2.0
compatibility: Needs a shell with the archfit binary and git.
metadata:
  author: alexei-led
  tags:
    - architecture
    - archfit
---

# archfit: the worker loop

1. Before you start: `archfit rules`. It lists the rules for the files you
   change.
2. Before a new dependency between modules: `archfit can <from> <to>`.
   `no` names the rule and what to do instead.
3. Before you finish: `archfit check`. Read the default text record; do what each `next:` line says.
   `archfit: pass` (exit 0) means done.
   - `fail` (exit 1): fix the code, or, if the rule is in your way,
     `archfit request -m "<why>"`, then wait or do other work.
   - `unknown` (exit 2): do what `next:` says (install a tool, git fetch)
     and run it again. If it names the policy, stop and ask an owner.
     Wrong usage and tool errors also exit 2; they are not a pass.
4. Never edit `.archfit.yaml`. Never copy code to get around a rule.
5. Claude: `/plugin marketplace add alexei-led/archfit`, then
   `/plugin install archfit@archfit`. Remove: `/plugin uninstall archfit@archfit`.
   Or use `archfit doctor --fix` per checkout, not both. See docs/agents.md.
