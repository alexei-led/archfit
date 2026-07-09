# Archfit Agent Skills

Portable agent skills for using and evaluating `archfit` from coding agents.

The maintained source lives in this directory:

```text
skills/
├── archfit/
│   ├── SKILL.md
│   └── references/
│       ├── agent-loop.md
│       ├── commands.md
│       ├── languages.md
│       └── llm-modes.md
└── archfit-eval/
    ├── SKILL.md
    └── references/
        ├── book-alignment.md
        └── corpus-sweep.md
```

- `skills/archfit/` stays focused on normal `archfit` use: setup, config,
  CI, findings, and repair.
- `skills/archfit-eval/` is for evaluating archfit itself: book alignment,
  corpus sweeps, semantic comparisons, and docs/UX checks.

`SKILL.md` stays thin: routing, workflow, and review rules. Canonical product
facts stay in `README.md`, `docs/guide/`, `docs/book-alignment-review-prompt.md`,
and `docs/test-corpus.md`; `references/` carry the portable subset each skill
needs.

## Use

Point your agent harness at `skills/`, or copy the specific skill directory to
the harness-specific skills folder.

Examples:

```sh
# Pi explicit loading
pi --skill ./skills/archfit
pi --skill ./skills/archfit-eval

# Generic copy pattern for harnesses with project skill folders
mkdir -p .agents/skills
cp -R skills/archfit .agents/skills/
cp -R skills/archfit-eval .agents/skills/
```

Keep this source copy as the maintained version.
