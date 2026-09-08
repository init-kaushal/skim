# Claude Code PreToolUse hook contract — findings (Task 1 spike)

**Date:** 2026-09-08
**Claude Code version:** 2.1.263
**Method:** Controller research against Claude Code's documented hook behavior for
this version. Not a live-session observation (a subagent cannot drive an
interactive session); see "Residual uncertainty" and the plan's review loop as
the correction path.

## 1. stdin JSON shape (what the hook reads)

A `PreToolUse` hook receives a single JSON object on stdin:

```json
{
  "session_id": "abc123",
  "transcript_path": "/Users/x/.claude/projects/.../transcript.jsonl",
  "cwd": "/Users/x/proj",
  "hook_event_name": "PreToolUse",
  "tool_name": "Read",
  "tool_input": { "file_path": "/Users/x/proj/big.go", "offset": 10, "limit": 100 }
}
```

Fields the handlers use:
- `tool_name` — the tool about to run (`"Read"`, `"Grep"`, `"Bash"`, …).
- `tool_input` — an object mirroring that tool's parameters **exactly as the
  model supplied them**.
- `cwd` — session working directory (available if needed; handlers currently
  resolve paths as given).

Per-tool `tool_input` keys:
- **Read:** `file_path` (always present, absolute); `offset`, `limit` (present
  only when the model supplied them — they are **omitted**, not zero-filled,
  when absent).
- **Bash:** `command` (the full command string); also `description`, `timeout`
  (unused by skim).
- **Grep:** `pattern` (always); `path`, `glob`, `type`, `output_mode`, `-n`,
  `-i`, `-A`/`-B`/`-C`, `head_limit` (all optional).

**Implication for `hookio.ReadInput`:** decoding into `struct{ Offset int;
Limit int }` yields `0` for an omitted key. skim treats `Offset != 0 || Limit
!= 0` as "targeted read → pass through". An omitted offset *and* an explicit
`offset: 0` both read as 0, which is the desired behavior (offset 0 with no
limit is a full read). No change needed.

## 2. Allow (let the tool run)

Exit code **0** with **no stdout** → the tool call proceeds through the normal
permission flow. This is skim's "allow / degrade-open" path.

(An explicit `{"hookSpecificOutput":{"hookEventName":"PreToolUse",
"permissionDecision":"allow"}}` also exists — it *short-circuits* remaining
permission checks. skim does **not** want that; empty stdout is correct so
normal permissioning still applies.)

## 3. Deny with a reason the model sees (skim's core lever)

Write this object to stdout, exit code **0**:

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<text>"}}
```

- The tool call is **blocked** (does not execute).
- `permissionDecisionReason` is **delivered to the model** as the feedback for
  the blocked call — this is its documented purpose. The model reads it and can
  act on it (e.g. issue a narrower `Read` with `offset`/`limit`).

Older equivalent still supported: `{"decision":"block","reason":"<text>"}`, and
exit code **2** with the message on **stderr**. The `hookSpecificOutput` /
`permissionDecision` form is the current, preferred one and is what
`hookio.Deny` emits.

**Decision hinges on this:** the reason text reaching the model is confirmed as
the designed behavior of `permissionDecision: "deny"`. → **PROCEED.**

## 4. `${CLAUDE_PLUGIN_ROOT}` in plugin `hooks.json`

`${CLAUDE_PLUGIN_ROOT}` expands to the installed plugin's root directory inside
a plugin's `hooks.json` `command` strings. So:

```json
{ "type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/bin/skim read-hook" }
```

resolves to the `bin/skim` shipped in the plugin. This is the mechanism Task 15
relies on.

## 5. Residual uncertainty

- Whether `permissionDecisionReason` is surfaced to the model as a `tool_result`
  vs. a system-style note — irrelevant to skim, since in every documented case
  the text is received and actionable.
- Exact matcher semantics for `"Bash"` (substring vs. exact) — Task 15 uses the
  bare tool name `"Bash"`, which is the documented exact-match form.
- If any of §1's field names differ in practice, only `internal/hookio`
  (Task 5) and `plugin/hooks.json` (Task 15) need adjustment; the review loop
  is the safety net.

## Decision

**PROCEED** — `PreToolUse` deny + `permissionDecisionReason` delivers text to
the model; that is the whole basis of skim's substitution approach and it is
confirmed for Claude Code 2.1.263.
