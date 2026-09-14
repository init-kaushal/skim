---
description: Inspect and control skim (token-saving tool-call interceptor)
---

Run the `skim` CLI subcommand the user asked for and report the output:

- `doctor`  — environment and config diagnostics
- `stats`   — cumulative interception savings
- `config`  — show config; `config set <key> <value>` to change it
- `off` / `on` — disable / re-enable interception

Argument: $ARGUMENTS (default to `doctor` if empty).

Use the Bash tool: `${CLAUDE_PLUGIN_ROOT}/bin/skim $ARGUMENTS`
