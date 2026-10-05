# skim v2 — Architecture & Roadmap

**Status:** Phase 1 merged to `v0.2-phase1` (current)  
**Repo:** https://github.com/init-kaushal/skim

---

## Why we're doing this

skim v0.1.x intercepts oversized Claude Code tool outputs and replaces them with
Haiku-produced digests. It saves tokens, but the decision logic is entirely
threshold-based (150 lines / 60 KB). That means:

- Files that are cheap to keep in context still get intercepted (wasted worker cost).
- Files that would clearly pay off on a Haiku call might be just under the threshold
  (missed savings).
- There's no trace of *why* a file was or wasn't intercepted.
- Bash output (git diffs, test results) goes straight to Haiku even when a
  deterministic parser would do the same job for free.

**The goal for v2** is to minimize total expensive-model work and context consumption
while preserving coding-agent correctness and quality. Not just "summarize big outputs."

---

## Core design principle

> Use the cheapest mechanism capable of completing a task.

Routing hierarchy (cheapest first):

```
1. Passthrough    — below minimum threshold; no action
2. Deterministic  — structural parser/compressor; no LLM needed
3. Cache hit      — previously digested content; no LLM needed
4. Cheap worker   — Haiku; justified by expected_savings > worker_cost × margin
5. (Phase 3) Sonnet
6. (Phase 3) Opus / escalation
```

**Route based on cost math, not input size alone.**

```
expected_savings = removed_tokens × session_model_price
worker_cost      = worker_input_tokens × worker_input_price
                 + worker_output_tokens × worker_output_price

intercept = expected_savings > worker_cost × safety_margin
         && expected_savings − worker_cost > min_savings_usd
```

---

## What changed in Phase 1 (v0.2)

### New packages

| Package | Role |
|---|---|
| `internal/cost/` | Pricing table for all models + `Evaluate()` decision function |
| `internal/compress/` | Deterministic compressors: go test, git status/diff/log, build errors |
| `internal/decision/` | `Engine.PlanRead/PlanGrep/PlanRun` — combines cost + compress into one routing decision |
| `internal/observe/` | Decision log (`~/.claude/skim/decisions.jsonl`) + `skim explain` formatter |

### Modified packages

- **`internal/config/`** — 4 new fields: `session_model`, `safety_margin`,
  `min_savings_usd`, `dry_run`. All backward-compatible (new fields default to
  sensible values; existing `config.json` files load without change).
- **`internal/cache/`** — Added `KeyFromContent()`: content-hash cache key that
  survives git touches, renames, and identical files at different paths.
  Old `Key()` (mtime-based) is kept for backward compatibility; existing cache
  entries expire normally via Sweep.
- **`internal/handler/read.go`** — Wired `Engine.PlanRead()` for cost-aware
  routing. Uses `KeyFromContent()` instead of `Key()`. Falls back to legacy
  threshold path when no engine is wired (preserves all existing tests).
- **`internal/handler/grep.go`** — Wired `Engine.PlanGrep()`: Grep calls now
  respect cost math before paying for a Haiku call.
- **`internal/handler/deps.go`** — Added `Engine *decision.Engine` and
  `ObserveRecord func(observe.Entry)` fields.
- **`internal/metrics/`** — Added `Strategy` field to `Entry` (omitempty,
  backward-compatible).
- **`internal/cli/`** — Added `explain` command (`skim explain`).
- **`cmd/skim/main.go`** — Wires `Engine` and `ObserveRecord` into production Deps.

### New CLI command

```
skim explain              # last 10 decisions
skim explain --last 25    # last N decisions
skim explain --tool Read  # filter by tool
skim explain --file auth  # filter by target path substring
```

Example output:
```
Operation:  Read internal/worker/worker.go
Strategy:   CHEAP_WORKER
Direct cost est:  $0.0041
Worker cost est:  $0.0006
Net savings est:  $0.0035
Reason:     savings ($0.0035) justify worker cost ($0.0006)
```

### New config fields

```json
{
  "session_model":   "claude-opus-5-5",
  "safety_margin":   1.5,
  "min_savings_usd": 0.001,
  "dry_run":         false
}
```

Set `dry_run: true` to run the full decision engine without intercepting — useful
for understanding what skim would do in a new repository before enabling it.

### Deterministic compressors

The `internal/compress/` package intercepts these output types before any LLM call:

| Compressor | Trigger | complete? |
|---|---|---|
| `go_test` | `--- PASS/FAIL` + tab-separated output | yes |
| `git_status` | `On branch` + modified/untracked markers | yes |
| `git_diff` | `diff --git` | no (LLM adds context) |
| `git_log` | `commit ` + `Author:` + `Date:` | yes |
| `build` | file:line compiler error patterns | yes |
| `generic_test` | pytest/jest/mocha PASS/FAIL patterns | yes |

`complete=true` means the compressed output is routed as `DETERMINISTIC` — no
LLM worker is invoked.

---

## Backward compatibility

- Existing `config.json` files: load without change. New fields default to safe values.
- Existing cache entries: coexist with content-hash entries. Expire normally.
- Existing metrics JSONL: new `strategy` field is `omitempty`; readers skip it.
- Existing CLI commands: all unchanged. `explain` is additive.
- Hook contract: fail-open preserved unconditionally. Any error in the new packages
  returns `Allow` — a broken optimizer never blocks a tool call.

---

## Phase 2 (next): Semantic repository index

Goals:
- `skim index` — builds an AST-based symbol map (Go: `go/ast`, TS: tree-sitter)
- Lets the expensive model locate code without grepping hundreds of files
- File maps: compact per-file representations (purpose, symbols, line ranges)
- Context state tracker: tracks session goals, decisions, relevant files

New packages: `internal/index/`, `internal/filemap/`, `internal/context/`

## Phase 3: Model routing & escalation

Goals:
- DETERMINISTIC → CHEAP → NORMAL → DEEP classification
- Workers return `SUCCESS | NEEDS_ESCALATION | FAILED`
- Haiku → Sonnet → Opus escalation chain
- No silent decisions outside cheap worker scope

New packages: `internal/router/`, extend `internal/worker/`

## Phase 4: Adaptive & quality

Goals:
- Reuse prediction from session history
- Quality signals correlated with test outcomes
- Benchmark framework (reproducible before/after measurements)
- Cross-agent adapter architecture

New packages: `internal/predict/`, `internal/quality/`, `internal/bench/`

---

## Safety rules (all phases)

These never change regardless of optimization decisions:

- Credentials, secrets, security-sensitive content → never intercepted
- Exact error messages, stack traces → always preserved
- Binary files, patches being reviewed → always passthrough
- Worker failure → degrade open (Allow), never block
- Sensitive content → local processing only, never sent to external workers
  unless the user explicitly configures it

---

*skim v2 architecture — written 2026-10-05*  
*Current: v0.1.4 (main) · Phase 1: v0.2-phase1*
