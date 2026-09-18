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
- `demo`    — write a sample file big enough to trigger interception, so the
  user can watch a Read get substituted

Argument: $ARGUMENTS (default to `doctor` if empty).

Use the Bash tool: `${CLAUDE_PLUGIN_ROOT}/bin/skim $ARGUMENTS`

If the subcommand was `demo`, then after the file is written, **use the Read
tool on it** — that is the whole point, and the user cannot see the effect
unless a Read actually happens. Read it, then say plainly what came back
instead of the file: how many lines the file has, what the digest contained,
and the fact that you can still answer questions about the file's structure
from it. Do not use Bash or `skim cat` to read it; those bypass the hook.
