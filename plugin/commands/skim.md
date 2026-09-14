---
description: Inspect and control skim (token-saving tool-call interceptor)
---

Run the `skim` CLI subcommand the user asked for and report the output:

- `doctor`  — environment and config diagnostics
- `stats`   — the interception ledger: tokens kept out of context, dollars the
  worker actually billed, and whether the trade is net ahead. Takes
  `--session-model opus-5|sonnet-5|haiku-4.5` (default `opus-5`) and `--turns N`,
  because the saving is priced at the *session* model's rate and that model
  cannot be detected from the hook payload.
- `config`  — show config; `config set <key> <value>` to change it
- `off` / `on` — disable / re-enable interception

Argument: $ARGUMENTS (default to `doctor` if empty).

Use the Bash tool: `${CLAUDE_PLUGIN_ROOT}/bin/skim $ARGUMENTS`
