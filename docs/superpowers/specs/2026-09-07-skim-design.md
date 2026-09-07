# skim — Design Spec

**Date:** 2026-09-07
**Status:** Draft for review
**Author:** Kaushal (with Claude Code)

## 1. Problem

Claude Code agents spend a large share of their token budget on non-reasoning
work: reading big files, scanning wide grep results, and absorbing noisy command
output (test runs, `curl`, log dumps). That bulk content lands in the main
context and is then re-sent on every subsequent turn, so the cost compounds.

Spotify's "Portal + shunt" pattern addresses this by hard-blocking expensive
operations from the expensive model and routing them to cheaper worker models.
Portal's implementation depends on an authenticated Spotify Portal instance,
which is not available to an individual developer.

`skim` reproduces the useful core of that pattern locally, with no external
platform, for a developer on a **Claude Pro subscription**. The worker is a
**nested `claude -p` call pinned to Haiku** for the interception paths (`Read`,
`Grep`, `Bash`); the opt-in `code-writer` path instead uses a real Claude Code
`Task` subagent whose agent definition pins `model: haiku`.

### What "reduce token cost" means here

On a Pro subscription there is no per-token invoice to shrink. The gains are:

1. **Digest instead of dump.** A 900-line file becomes a ~500-token structured
   map + summary rather than thousands of tokens of raw content.
2. **No re-send.** The digest, not the file, is what occupies the main context
   on every following turn.

Net effect: less pressure on the 5-hour and weekly usage limits, and slower
context-window growth. The cost is added latency per interception (a nested
Haiku call), mitigated by caching and a kill switch.

## 2. Goals / Non-goals

### Goals

- Intercept large `Read`, wide `Grep`, and noisy `Bash` operations before they
  run and substitute a cheap Haiku-produced digest.
- Preserve the agent's ability to get exact content when it needs it (targeted
  re-read, or an explicit force-full escape hatch). Never lose data.
- Provide an opt-in `code-writer` delegation path for boilerplate.
- Fail open: if the optimizer breaks, the original operation proceeds normally.
- Ship as an installable Claude Code plugin.
- Make the savings measurable (`skim stats`).

### Non-goals

- No dollar-cost accounting or invoicing (there is no invoice on Pro).
- No support for API-metered billing models in v1 (design does not preclude it).
- No local-model (Ollama) backend in v1.
- No interception of operations that need design judgement — the `code-writer`
  path is explicitly scoped to mechanical boilerplate.
- No rewriting of tool arguments (`updatedInput`) — the design relies only on
  `PreToolUse` deny + reason text, which is stable across Claude Code versions.

## 3. Architecture

### 3.1 Delivery

A Claude Code plugin, `skim`, installable via:

```
claude plugin marketplace add <owner>/skim
claude plugin install skim@skim
```

### 3.2 Components

| Component | Path in plugin | Purpose |
|---|---|---|
| `skim` binary | `bin/skim` | All hook logic and CLI. Single Go binary. |
| Hook registration | `hooks/hooks.json` | Registers `PreToolUse` handlers for `Read`, `Grep`, `Bash`. |
| `code-writer` agent | `agents/code-writer.md` | Haiku agent definition for boilerplate generation. |
| `code-writer` skill | `skills/code-writer/SKILL.md` | Instructs the main agent to delegate boilerplate to the agent. |
| `/skim` command | `commands/skim.md` | Wraps `skim doctor` / `stats` / `config` / `off` / `on`. |
| Plugin manifest | `.claude-plugin/plugin.json` + marketplace manifest | Install metadata. |

### 3.3 Repo layout

```
~/Projects/skim/
  cmd/skim/main.go            # CLI entrypoint, subcommand dispatch
  internal/
    hook/                     # stdin/stdout hook JSON contract, per-tool handlers
    detect/                   # size/threshold/pattern detection
    worker/                   # nested `claude -p` invocation + JSON parse
    digest/                   # map/cluster/run schemas + compact rendering
    cache/                    # content-addressed digest cache
    config/                   # config load + precedence
    metrics/                  # JSONL append + `skim stats` aggregation
  plugin/
    .claude-plugin/plugin.json
    hooks/hooks.json
    agents/code-writer.md
    skills/code-writer/SKILL.md
    commands/skim.md
    bin/                      # build output target (gitignored)
  Makefile                    # `make build` -> plugin/bin/skim for host OS/arch
  docs/superpowers/specs/2026-09-07-skim-design.md
```

### 3.4 Binary subcommands

| Subcommand | Called by | Behaviour |
|---|---|---|
| `skim read-hook` | `PreToolUse` / `Read` | See 4.1 |
| `skim grep-hook` | `PreToolUse` / `Grep` | See 4.2 |
| `skim bash-hook` | `PreToolUse` / `Bash` | See 4.3 |
| `skim run -- <cmd>` | agent (via bash-hook substitute) | Execute `<cmd>`, capture full output to a log file, print summary + log path. |
| `skim cat <path>` | agent (force-full escape hatch) | Print the whole file, bypassing interception. |
| `skim summarize` | internal | Build prompt, invoke worker, emit digest JSON. |
| `skim doctor` | user / `/skim` | Environment + config diagnostics. |
| `skim stats` | user / `/skim` | Cumulative savings + cache hit rate. |
| `skim config [...]` | user / `/skim` | Show/set config, `--clear-cache`. |

## 4. Interception mechanism

The only lever used is **`PreToolUse` → deny → replacement text in
`permissionDecisionReason`**. No argument rewriting, no PostToolUse result
substitution.

### 4.0 Recursion guard (required)

Every hook subcommand checks the `SKIM_ACTIVE` env var first. If set, the hook
emits an immediate no-op (allow / no decision) and exits. The `worker` package
sets `SKIM_ACTIVE=1` in the environment of every nested `claude -p` process, so
the worker's own tool calls never re-enter the hooks.

### 4.1 `Read`

1. If the `Read` call already carries `offset` or `limit` → pass through
   (targeted read is intentional and cheap).
2. If the path matches a `passthrough_globs` entry → pass through.
3. `stat` the file; count lines. If `<= read_max_lines` **and**
   `<= read_max_bytes` → pass through.
4. Look up the digest cache (key in 5.2). On hit → go to step 6.
5. On miss → invoke the worker (4.4) to produce a **file map** digest (5.1).
   Store in cache.
6. Emit `permissionDecision: deny`. `permissionDecisionReason` = rendered digest
   (5.3) followed by:
   > For exact lines, `Read` again with `offset`/`limit` on the range you need.
   > To force the full file, run `skim cat <path>`.

### 4.2 `Grep`

1. The hook runs ripgrep itself with the requested pattern/scope and counts
   matches.
2. If `<= grep_max_matches` → pass through (let Claude Code run its own `Grep`;
   the duplicate ripgrep run is cheap).
3. Otherwise → invoke the worker to produce a **cluster** digest (5.1): matches
   grouped by directory and/or enclosing symbol, with counts and representative
   file paths. Deny, return the rendered clusters.

### 4.3 `Bash`

1. The hook matches the command string against `bash_noisy_patterns` (regex
   list, configurable).
2. No match → pass through.
3. Match → deny with reason:
   > Large-output command. Re-run as: `skim run -- <cmd>`
4. `skim run` executes `<cmd>`, streams full stdout+stderr to
   `~/.claude/skim/runs/<id>.log`, then prints a **run** digest (5.1): short
   summary, key lines (errors, failures, result counts), exit code, and the log
   path. Nothing is discarded.

### 4.4 Worker call

```
SKIM_ACTIVE=1 claude -p --model <config.model> \
  --output-format json --max-turns 1 < <payload>
```

- The worker is given no tools; it receives raw content plus a JSON-schema
  instruction.
- `--max-turns 1` bounds it to a single response.
- Parse the `.result` field as JSON against the expected schema (5.1).

### 4.5 Graceful degradation

If the worker exits non-zero, exceeds `worker_timeout_sec`, or returns output
that does not parse against the schema:

- The hook emits **allow / no decision**, so the original operation runs
  normally.
- The failure is appended to `~/.claude/skim/skim.log` with context.

The optimizer breaking must never block the user.

## 5. Digest schemas, rendering, caching

### 5.1 Schemas

**File map** (`Read`):

```json
{
  "summary": "3-5 sentence overview of purpose and shape",
  "map": [
    {"lines": "1-38",    "kind": "imports + package doc"},
    {"lines": "39-140",  "kind": "type defs: Config, Rule, Threshold"},
    {"lines": "141-320", "kind": "func Load + validation helpers"},
    {"lines": "321-410", "kind": "func Apply — main entry point"}
  ],
  "symbols": ["Config", "Rule", "Load", "Apply"],
  "notes": "line numbers approximate +/- 3"
}
```

**Clusters** (`Grep`):

```json
{
  "summary": "what the matches collectively represent",
  "clusters": [
    {"where": "internal/hook/", "matches": 23, "gist": "handler wiring for each tool"},
    {"where": "internal/detect/", "matches": 11, "gist": "threshold checks"}
  ],
  "total": 41,
  "representative_files": ["internal/hook/read.go", "internal/detect/size.go"]
}
```

**Run** (`Bash`):

```json
{
  "summary": "1-3 sentence outcome",
  "key_lines": ["FAIL TestApply/oversize", "2 failed, 44 passed"],
  "exit_code": 1,
  "log_path": "~/.claude/skim/runs/abc123.log"
}
```

### 5.2 Cache

- Location: `~/.claude/skim/cache/<sha>.json`
- Key: `sha256(abs_path + mtime_ns + size_bytes + config.model + schema_version)`
- Only `Read` digests are cached (grep/bash inputs are not stable).
- Startup sweep removes entries older than 7 days.
- `skim config --clear-cache` empties it.

### 5.3 Rendering

Each schema has a compact plain-text renderer with a golden-file test. The
rendered form is what goes into `permissionDecisionReason`. Target: under ~600
tokens for a typical file map.

### 5.4 Metrics

Every interception appends one line to `~/.claude/skim/metrics.jsonl`:

```json
{"ts":"2026-09-07T21:10:00Z","tool":"Read","orig_tokens_est":7400,
 "digest_tokens_est":480,"worker_tokens":5100,"cache_hit":false,"saved_est":6920}
```

`skim stats` aggregates: total estimated tokens kept out of main context, cache
hit rate, interception counts per tool. Token estimates use a bytes/4
approximation; they are indicative, not exact.

## 6. code-writer path

### 6.1 Agent (`agents/code-writer.md`)

- Frontmatter: `model: haiku`; `tools: Read, Write, Edit, Glob, Grep`.
- System prompt, in essence: "You generate boilerplate that matches existing
  project conventions. Read neighbouring files to learn the pattern. Implement
  exactly what was asked — do not make design decisions. Report the file paths
  and line ranges you wrote so they can be reviewed."

### 6.2 Skill (`skills/code-writer/SKILL.md`)

- Triggers on boilerplate-shaped requests: a new endpoint mirroring an existing
  one, test scaffolds, DTO/struct plumbing, config wiring.
- Instructs the main agent to delegate to the `code-writer` agent via `Task`,
  then **review the resulting diff** rather than writing the code itself.
- Explicitly excludes anything needing design judgement, concurrency reasoning,
  or non-obvious correctness — mirrors the known failure mode where a worker
  model missed a thread-safety bug.

## 7. Configuration

`~/.claude/skim/config.json`, created with defaults by `skim doctor`:

```json
{
  "disabled": false,
  "read_max_lines": 300,
  "read_max_bytes": 60000,
  "grep_max_matches": 60,
  "bash_noisy_patterns": [
    "\\bcat\\s", "\\bcurl\\s", "\\btail\\s+-n\\s+\\d{3,}",
    "npm\\s+(run\\s+)?test", "jest", "pytest", "go\\s+test"
  ],
  "passthrough_globs": ["**/*.md", "**/go.mod", ".claude/**"],
  "model": "claude-haiku-4-5-20251001",
  "worker_timeout_sec": 45
}
```

### Kill switches (precedence order)

1. `SKIM_DISABLED=1` in the environment.
2. `"disabled": true` in config (toggled by `/skim off` / `/skim on`).
3. `passthrough_globs` — per-path opt-out.

When disabled, every hook is a no-op pass-through.

### `skim doctor` reports

- Is `claude` on `PATH` and runnable.
- Is `plugin/bin/skim` built for this OS/arch.
- Config file present and valid.
- Last 5 errors from `skim.log`.
- Cache size and entry count.

## 8. Testing strategy

- **Go unit tests:** threshold detection; config load + precedence;
  `bash_noisy_patterns` matching; cache-key determinism; `SKIM_ACTIVE`
  recursion guard; hook JSON I/O contract (stdin bytes → stdout schema).
- **Fake `claude` stub:** a script placed on `PATH` during tests that echoes
  canned JSON, so the full hook round-trip (detect → worker → render deny
  reason) runs deterministically with zero real API calls.
- **Golden-file tests:** every digest renderer (file map / clusters / run).
- **Degradation tests:** stub returns malformed JSON / non-zero exit / sleeps
  past `worker_timeout_sec` → assert the hook emits allow/no-decision and writes
  a log line.
- **Manual smoke test:** real repo + real `claude`; run a task that reads
  several large files with the plugin on vs. off; compare `/context` growth and
  `skim stats` output.

## 9. Open questions / risks

- **Latency.** Every uncached large read adds one nested `claude -p` startup
  (~3-6 s). Mitigations: caching, thresholds tuned so only genuinely large
  files trigger, `/skim off`. If this proves too intrusive interactively, a
  follow-up option is an async pre-warm or a persistent worker process.
- **Claude Code hook-output contract.** The design assumes `PreToolUse` deny +
  `permissionDecisionReason` is delivered to the model as usable text on the
  installed Claude Code version. First implementation task is a spike to
  confirm this and the exact JSON envelope.
- **Nested `claude -p` and quota.** Worker calls still count against the Pro
  account. The design bets that Haiku-weighted worker tokens plus the
  no-re-send saving nets out positive; `skim stats` is what proves or disproves
  it in practice.
- **`Grep` double-run.** For under-threshold greps the hook runs ripgrep and
  then Claude Code runs its own. Cheap, but noted.
- **Binary distribution.** The plugin must ship a binary matching the user's
  OS/arch, or build on install. v1: `make build` for the host; multi-platform
  release artifacts are a follow-up.

## 10. Out of scope for v1 (possible follow-ups)

- Ollama / local-model worker backend.
- API-metered billing mode with real dollar accounting.
- `updatedInput`-based transparent argument rewriting (no deny round-trip).
- Persistent worker process to remove per-call startup latency.
- Multi-platform prebuilt release binaries.
