---
name: code-writer
description: Use when the task is mechanical boilerplate that mirrors an existing pattern in this repo — a new endpoint/handler/DTO/test file that copies a sibling. Delegates the writing to a cheap Haiku subagent and has you review the diff. Do NOT use for code needing design judgement, concurrency reasoning, or non-obvious correctness.
---

# Delegating boilerplate to the code-writer agent

When the current task is boilerplate that closely mirrors code already in the repo:

1. Identify the exact sibling file(s) the new code should mirror, and the exact output path(s).
2. Dispatch the `code-writer` agent via the Task tool with:
   - the sibling file paths to copy conventions from,
   - the precise thing to produce,
   - any names/signatures that are already fixed.
3. When it returns, **read the diff it produced and review it yourself** — check names, error handling, edge cases, and that it made no unrequested changes. You are accountable for the result.
4. If the change needed design judgement, concurrency reasoning, or non-obvious correctness, do it yourself instead — the worker model is not reliable for those.
