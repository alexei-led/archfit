# Archfit Agent Skill

Portable Agent Skill for using `archfit` from coding agents.

The skill follows the Agent Skills directory format:

```text
skills/archfit/
├── SKILL.md
└── references/
    └── archfit-docs.md
```

`SKILL.md` contains only routing and workflow instructions. Canonical docs stay
in `README.md` and `docs/guide/`; `references/archfit-docs.md` links to them.

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
