# Optional AI features

Plain `archfit analyze` and `archfit check` do not call a model. The gate stays
deterministic. AI features run only when you request a command or flag that
uses a provider.

Use `archfit analyze --ai-summary` for an optional narrative after deterministic
analysis. The narrative can interpret findings and metrics. It cannot change
the verdict, findings, metrics, score, config, labels, or gate. `check` does
not accept `--ai-summary`.

## Configure a provider

Set `ai.provider` and `ai.model` in `.archfit.yaml`:

```yaml
ai:
  provider: anthropic # anthropic, openai, or ollama
  model: claude-opus-4-8
  # base_url: http://localhost:11434/v1 # ollama only
```

Anthropic uses `ANTHROPIC_API_KEY`. OpenAI uses `OPENAI_API_KEY`. Ollama uses
`base_url` and does not need an API key. archfit reads `.env` from the current
directory, the repository root, and the config directory. Existing environment
values take priority.

Keep `.env` and `.archfit-cache/` out of version control. The cache stores
provider responses, which can include repository text. `archfit doctor` shows
provider setup and key presence.

## Review and apply drafts

AI config commands create suggestions for human review. Review draft files and
reports before you use their content.

- `config init --ai-classify` creates AI suggestions for a new config. Use
  `-o` to write a separate draft. `--apply` writes suggestions into the output
  config, so review that file before using it as a gate.
- `config update --ai-classify` adds review-only semantic proposals to the
  update report. It does not apply those proposals.
- `config enrich labels` writes label drafts to `.archfit-labels.yaml`. Change
  accepted entries to `status: approved` before they affect classification.
- `config enrich owner`, `volatility`, and `subdomain` write separate draft
  files. Review entries and mark accepted entries approved before using
  `--apply` to add them to `.archfit.yaml`.

`--apply` for these metadata drafts writes approved entries only. It skips
fields that already have values. `config enrich labels` does not use `--apply`.
For command syntax and config fields, see [commands](commands.md) and
[configuration](configuration.md).

## Review provider data

AI commands send bounded repository evidence to the configured provider. The
evidence can include docs, Go package comments, exported Go names, config
snippets, diagnostics, module names, and dependency samples. archfit redacts
secret-like config values and excludes secret-like paths, but this is not
secret scanning.

Do not run provider-backed commands on a repository that contains text you
must not send to that provider. Review the provider's data policy before use.
Use `--refresh` to skip cached responses and request new responses. This can
increase provider cost.

See [results](results.md) for machine-readable output contracts and
[configuration](configuration.md) for the `ai` fields.
