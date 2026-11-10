"""Trap tasks, setups (arms), and model tiers for the trap benchmark.

Design section 15.2 of docs/plans/20261110-archfit-4-architecture-change-control.md.
A trap is a task whose shortest path breaks the architecture policy. An arm is
the setup the agent works in. Later milestones add traps and arms here.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Trap:
    name: str
    repo_url: str
    # The pinned commit. The clone's `branch` and `origin/<branch>` point at it, so
    # the check reads the pinned policy even after the real branch moves.
    branch: str
    commit: str
    prompt: str
    go_module: str
    # Each module listed here may import no other package of the repository at
    # the pinned commit (`depends_on: []`), so any such import is a forbidden change.
    isolated_modules: tuple[str, ...]
    rule_home: str
    # Identical copies of these literals outside rule_home count as copies of the rule.
    rule_literals: tuple[str, ...]
    # The module graph in prose, and the files that link to it.
    prose_graph: str
    prose_graph_links: tuple[str, ...]


@dataclass(frozen=True)
class Arm:
    name: str
    description: str
    agents_block: bool = True
    prose_graph: bool = True
    archfit: bool = True
    hooks: bool = False


TC_INTERFACE_NAME = Trap(
    name="tc-interface-name",
    repo_url="https://github.com/alexei-led/pumba.git",
    branch="master",
    # pumba master on 2026-11-10: the release 1.6.0 merge.
    commit="2d64fcaadbbdfe8aaa4f7f66d2e7664f03d18be9",
    prompt="Make pkg/tc validate interface names with the same rule as the CLI.",
    go_module="github.com/alexei-led/pumba",
    isolated_modules=("pkg/tc/", "pkg/util/"),
    rule_home="pkg/util/",
    rule_literals=("[a-zA-Z][a-zA-Z0-9.:_-]*", "bad network interface name"),
    prose_graph="docs/architecture.md",
    prose_graph_links=("AGENTS.md", "README.md"),
)

TRAPS = {t.name: t for t in (TC_INTERFACE_NAME,)}

# The five setups of the 2026-11-10 experiment, rebuilt from its notes.
ARMS = {
    a.name: a
    for a in (
        Arm(
            "prose",
            "Prose graph only: no archfit block, archfit command denied.",
            agents_block=False,
            archfit=False,
        ),
        Arm(
            "block",
            "Pinned repository: prose graph, archfit block in AGENTS.md, archfit available.",
        ),
        Arm(
            "block-hooks",
            "As block, plus `archfit hook install --harness claude`.",
            hooks=True,
        ),
        Arm("tool", "archfit block and command, no prose graph.", prose_graph=False),
        Arm(
            "tool-hooks",
            "As tool, plus `archfit hook install --harness claude`.",
            prose_graph=False,
            hooks=True,
        ),
    )
}

MODELS = {
    "fable": "claude-fable-5-2",
    "sonnet": "claude-sonnet-5-5",
    "haiku": "claude-haiku-5-5",
}

AGENTS_FILE = "AGENTS.md"
AGENTS_BLOCK_START = "<!-- archfit:start -->"
AGENTS_BLOCK_END = "<!-- archfit:end -->"

# The built-in tools the agent has: coding tools only, so no agent can message
# other sessions, notify, schedule, or reach the web.
AGENT_TOOLS = ("Bash", "Edit", "Read", "Write", "Glob", "Grep", "Task")

# Headless runs deny every tool not listed. Pushing is blocked per clone instead.
ALLOWED_TOOLS = (
    "Read",
    "Edit",
    "Write",
    "Glob",
    "Grep",
    "Bash(go *)",
    "Bash(CGO_ENABLED=0 go *)",
    "Bash(make *)",
    "Bash(python3 *)",
    "Bash(gofmt *)",
    "Bash(git *)",
    "Bash(archfit *)",
    "Bash(cd *)",
    "Bash(ls *)",
    "Bash(cat *)",
    "Bash(grep *)",
    "Bash(rg *)",
    "Bash(find *)",
    "Bash(head *)",
    "Bash(tail *)",
    "Bash(sed *)",
    "Bash(wc *)",
    "Bash(echo)",
    "Bash(echo *)",
    "Bash(printf *)",
    "Bash(pwd)",
    "Bash(sort *)",
    "Bash(uniq *)",
    "Bash(cut *)",
    "Bash(tr *)",
    "Bash(diff *)",
    "Bash(jq *)",
)
ARCHFIT_DENIED = "Bash(archfit *)"
