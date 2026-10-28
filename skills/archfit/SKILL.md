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

1. Before an import across modules: `archfit policy can-import <file> <target>`.
   `denied`: do not add it; read `instead`. `unconstrained` is not permission.
   `archfit policy where <path>` gives a path's module, allowlists and rules.
2. Before you finish: `archfit check --format agent`; follow `next_action`:
   - `repair`: fix the source within the goal and constraints; run `validate`.
   - `restore_evidence`: install or fix the named analyzer, or merge or
     rebase the target when the result says so; run again.
   - `ask_owner`, or `unknown` you cannot fix: stop; report the rule, the
     file and what you need. Do not retry the same change.
   - `none`: done, even with exit 2. `repository.attention` is not yours.
3. Never edit `.archfit.yaml`, `.archfit-baseline.json` or
   `.archfit-labels.yaml`, add a waiver, or run `archfit baseline` to pass.
