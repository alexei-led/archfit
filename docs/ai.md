# Optional AI features

Plain `archfit analyze` and `archfit check` do not call a model.
The gate stays deterministic.
AI features run only when you request a command or flag that uses a provider.

Use `archfit analyze --ai-summary` for an optional narrative after deterministic analysis.
The narrative cannot change the verdict, findings, metrics, score, config, labels, or gate.
`check` does not accept `--ai-summary`.

## Configure a provider

Archfit uses the official Anthropic and OpenAI Go SDKs, not an agent framework.
Set `ai.provider` and `ai.model` in `.archfit.yaml`.
See [configuration](configuration.md#ai-provider) for every field, default, and restriction.

| Provider | API | Model example / init-update default | Credential |
| --- | --- | --- | --- |
| `anthropic` | Messages | `claude-opus-5-5` | `ANTHROPIC_API_KEY` |
| `openai` | Responses | `gpt-6.1-sol` | `OPENAI_API_KEY` |
| `deepseek` | Chat Completions | `deepseek-flash` | `DEEPSEEK_API_KEY` |
| `ollama` | Chat Completions | Caller supplies an installed model | None |
| `openai-compatible` | Chat Completions | Caller supplies a served model | Variable named by `api_key_env`, or none |

Model IDs pass through to the provider.
This table names integration targets, not guaranteed availability for every account or model setting.

```yaml
ai:
  provider: anthropic
  model: claude-opus-5-5
  effort: medium
  max_tokens: 32768
  timeout: 5m
```

Opus 5.5 always thinks adaptively.
Archfit omits the thinking field instead of disabling thinking or setting a manual budget.
OpenAI uses Responses with `store: false`.
This disables response storage, not every provider retention policy.
No adapter sends temperature, top-p, or top-k.

Provider failures, refusals, empty replies, and incomplete replies are errors.
Archfit does not return partial text as a successful draft or cache it.
After a token-limit error, raise the cap or lower effort.
Archfit never silently switches models or weakens settings.

### DeepSeek Flash

Official docs checked on 2026-10-23 identify `deepseek-flash` as the API ID.
It currently serves **DeepSeek-V4.1-Flash-1017**.
The alias can change server-side and is not an immutable snapshot.

```yaml
ai:
  provider: deepseek
  model: deepseek-flash
  effort: high
  max_tokens: 65536
  timeout: 5m
```

Set `DEEPSEEK_API_KEY`, then run `archfit config enrich labels`.
The preset sends `thinking: {type: enabled}` and `reasoning_effort` to `/chat/completions`.
Only final answer content is used.
`reasoning_content` is not treated as the answer.

Sources: [first API call](https://api-docs.deepseek.com/),
[thinking controls](https://api-docs.deepseek.com/guides/thinking_mode),
[request reference](https://api-docs.deepseek.com/api/create-chat-completion),
and [release log](https://api-docs.deepseek.com/updates/).

### Another OpenAI-compatible endpoint

No code change is needed for servers implementing the same Chat Completions contract.

```yaml
ai:
  provider: openai-compatible
  model: vendor-model-id
  base_url: https://llm.example.com/v1
  api_key_env: EXAMPLE_LLM_API_KEY
```

Use the vendor's documented base path.
Archfit appends `chat/completions`.
Omit `api_key_env` for keyless local access.
This provider never inherits `OPENAI_API_KEY`.
A configured credential variable must exist and contain a non-empty value.

### Init/update overrides

Precedence is explicit flags, then `ai:` config, then provider-specific defaults.
Without config or flags, init/update use Anthropic Opus 5.5.
Selecting only `--ai-provider openai` selects Sol 6.1.
Selecting `deepseek` selects Flash.
Ollama and generic endpoints have no default model.

Switching providers discards the previous model, endpoint, credential-variable name, effort, budget, and timeout.
Selecting the same provider preserves them.
`--ai-model` changes only the model.

## Review and apply drafts

AI config commands create suggestions for human review.
Review draft files and reports before using their content.

- `config init --ai-classify` creates suggestions for a new config.
  Use `-o` to write a separate draft.
  `--apply` writes suggestions into the output config.
  Review that file before using it as a gate.
- `config update --ai-classify` adds review-only semantic proposals to the update report.
  It does not apply those proposals.
- `config enrich labels` writes drafts to `.archfit-labels.yaml`.
  Change accepted entries to `status: approved` before they affect classification.
- `config enrich owner`, `volatility`, and `subdomain` write separate draft files.
  Mark accepted entries approved before using `--apply`.

Metadata apply writes approved entries only.
It skips fields that already have values.
`config enrich labels` does not use `--apply`.
See [commands](commands.md) and [configuration](configuration.md) for syntax and draft fields.

## Review provider data

AI commands send bounded repository evidence to the configured provider.
Evidence includes docs, Go comments, exported names, config snippets, diagnostics, module names, and dependency samples.
Archfit redacts secret-like config values and excludes secret-like paths.
This is not secret scanning.

Only use trusted endpoints.
Do not run provider-backed commands on repositories containing text you must not send.
HTTP sends prompts and credentials without TLS.
Use HTTPS except for trusted local connections.
URLs containing credentials, queries, or fragments are rejected.
Never put a key in `base_url` or config values.
Configure endpoint overrides through `ai.base_url`, not SDK base-URL environment variables.

Archfit loads `.env` from cwd and explicit root/config directories without replacing existing environment values.
Only `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, and `DEEPSEEK_API_KEY` are loaded.
Export custom `api_key_env` credentials in the process environment.
Target `.env` files cannot configure analyzers, executables, package indexes, toolchains, or provider endpoints.
Keep `.env` and `.archfit-cache/` untracked.
Cached responses can include repository text.
`archfit doctor` reports credential presence, never credential values.

The versioned response cache separates provider/API, model, endpoint, effort, output budget, credential-variable name, and prompts.
Credential values are not part of the cache key.
Older transport cache entries are not replayed.
`--refresh` skips cached responses and can increase provider cost.

## Add models and providers

- **Existing API:** configure the new model ID.
  Check its current effort, output-budget, and endpoint requirements.
  Accepting a model name is not compatibility verification.
- **Compatible vendor:** use `openai-compatible`.
  The server must accept system/user messages, `max_tokens`, and optional `reasoning_effort`.
  It must return text with `finish_reason: stop`.
  Different contracts need an adapter, not guessed fallback parameters.
- **Native API or named preset:** implement `llm.Provider` under `internal/llm/`.
  Keep vendor SDK types there.
  Register construction in `cmd/archfit/enrich.go`.
  Update validation/schema, CLI help, `doctor`, tests, and these docs.
  Include endpoint and generation settings in the private `cacheIdentity` method.
  Keep the provider outside deterministic analysis.

## Keep model support current

1. Follow official release, migration, and deprecation notices.
   Model cards are reviewed guidance, not a runtime dependency or live capability registry.
2. Update pinned SDKs through reviewed dependency PRs.
   Check migration notes and transitive dependencies.
3. Add exact request-contract tests, including refusal, truncation, auth, malformed response, and timeout paths.
   Compare enrichment quality on fixed inputs before promoting defaults.
4. Run opt-in live smoke tests for configured accounts:

   ```sh
   ARCHFIT_LLM_LIVE=1 go test ./internal/llm -run TestLiveProviders -v
   ```

   This sends a small prompt and **spends tokens**.
   Missing keys skip their provider tests visibly.
   Ordinary tests and CI gates never enable this flag.

Defaults change explicitly, never through automatic discovery or fallback.
See [Opus 5.5 migration](https://platform.claude.com/docs/en/models/opus-5-5/migration-guide)
and [Sol 6.1 specifications](https://developers.openai.com/api/docs/models/gpt-6.1-sol) for current API constraints.
