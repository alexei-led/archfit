# Archfit Agent Skill

Portable Agent Skill for using `archfit` from coding agents.

The maintained source lives in this directory:

```text
skills/archfit/
├── SKILL.md
└── references/
    ├── agent-loop.md
    ├── commands.md
    ├── languages.md
    └── llm-modes.md
```

`SKILL.md` stays thin: routing, workflow, and review rules. Canonical product
docs stay in `README.md` and `docs/guide/`; `references/` carries the portable
subset this skill needs.

## Use

Point your agent harness at `skills/`, or copy the `skills/archfit/` directory to
the harness-specific skills directory.

Examples:

```sh
# Pi explicit loading
pi --skill ./skills/archfit

# Generic copy pattern for harnesses with project skill folders
mkdir -p .agents/skills
cp -R skills/archfit .agents/skills/
```

Keep this source copy as the maintained version.
