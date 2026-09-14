#!/usr/bin/env python3
"""Measure whether skim's Read interception actually saves money on a given file.

skim stats reports a token count. Tokens are not the billing unit: the worker's
input, output, cache-write and cache-read tokens are each priced differently,
and the worker runs on a different model than the session. This script does the
comparison in dollars instead, using the real `claude -p` cost envelope.

    python3 scripts/bench-digest.py <file> [--session-model opus-5] [--turns 10]

It makes ONE real API call (the digest the Read hook would have made) and prices
the counterfactual — what the session model would have paid to read the file
directly — from the same token counts. Nothing is estimated except the
counterfactual's token count, which is taken from the worker's own measurement
of the file.
"""
import argparse, json, os, subprocess, sys

# Per-million-token prices, Anthropic first-party. SOURCE OF TRUTH: the
# `claude-api` skill's model table. Re-check when models or prices change —
# every conclusion below is a ratio of these numbers.
PRICES = {
    "opus-5":    {"in": 5.00, "out": 25.00},
    "sonnet-5":  {"in": 3.00, "out": 15.00},
    "haiku-4.5": {"in": 1.00, "out":  5.00},
}
# Cache multipliers on the *input* rate. Measured against a real Haiku call:
# a 5,509-token 1h cache write billed $0.011018 = 5509 * 2.0 / 1e6.
CACHE_WRITE_1H = 2.00
CACHE_WRITE_5M = 1.25
CACHE_READ     = 0.10

WORKER_MODEL_ID = "claude-haiku-4-5-20251001"
WORKER_PRICE_KEY = "haiku-4.5"
MAX_WORKER_INPUT = 400 * 1024   # must match handler.maxWorkerInputBytes

PROMPT = '''You are a code-reading assistant. Below is the full contents of the file {path}.
Return ONLY a JSON object, no prose, with this shape:
{{"summary": "<3-5 sentences on purpose and shape>",
 "map": [{{"lines": "<start>-<end>", "kind": "<what lives there>"}}, ...],
 "symbols": ["<top-level names>"],
 "notes": "line numbers approximate +/- 3"}}
The map must cover the file top to bottom with no gaps.

FILE CONTENTS:
{content}'''


def worker_call(path):
    """Run the exact digest call skim's Read hook would run. Returns the envelope."""
    raw = open(path, encoding="utf-8", errors="replace").read()
    truncated = len(raw.encode()) > MAX_WORKER_INPUT
    content = raw[:MAX_WORKER_INPUT]
    prompt = PROMPT.format(path=os.path.abspath(path), content=content)

    env = dict(os.environ, SKIM_DISABLED="1")   # never let skim intercept its own benchmark
    p = subprocess.run(
        ["claude", "-p", "--model", WORKER_MODEL_ID,
         "--output-format", "json", "--max-turns", "1", "--tools", ""],
        input=prompt, capture_output=True, text=True, env=env, timeout=600,
    )
    if p.returncode != 0:
        sys.exit(f"worker call failed (exit {p.returncode}): {p.stderr[:400]}")
    return json.loads(p.stdout), truncated


def price(tokens, rate_per_m, mult=1.0):
    return tokens * rate_per_m * mult / 1e6


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("file")
    ap.add_argument("--session-model", default="opus-5", choices=sorted(PRICES))
    ap.add_argument("--turns", type=int, default=10,
                    help="turns the content would have ridden along for (re-send effect)")
    a = ap.parse_args()

    nbytes = os.path.getsize(a.file)
    nlines = sum(1 for _ in open(a.file, encoding="utf-8", errors="replace"))
    env, truncated = worker_call(a.file)
    u = env["usage"]

    # --- what the worker actually cost (ground truth from the CLI) ---
    worker_usd = env["total_cost_usd"]
    flat_sum = (u["input_tokens"] + u["output_tokens"]
                + u["cache_creation_input_tokens"] + u["cache_read_input_tokens"])

    # --- how many tokens the file itself is, per the worker's own count ---
    # The worker's prompt is system + file. cache_creation covers the bulk of it;
    # subtract the ~5.5K system prompt measured for `--tools ""`.
    SYSTEM_PROMPT_TOKENS = 5509
    file_tokens = max(1, u["cache_creation_input_tokens"] + u["cache_read_input_tokens"]
                      + u["input_tokens"] - SYSTEM_PROMPT_TOKENS)

    digest_chars = len(env["result"])
    digest_tokens = max(1, digest_chars // 4)

    sm = PRICES[a.session_model]
    # Counterfactual: session model ingests the file. Claude Code caches the
    # conversation prefix, so a re-send bills at the cache-read rate.
    def without(turns):
        return price(file_tokens, sm["in"]) + price(file_tokens, sm["in"], CACHE_READ) * turns

    def with_skim(turns):
        return worker_usd + price(digest_tokens, sm["in"]) + price(digest_tokens, sm["in"], CACHE_READ) * turns

    print(f"file            {a.file}")
    print(f"                {nbytes:,} bytes / {nlines:,} lines"
          + ("   ** TRUNCATED to 400KB before the worker saw it **" if truncated else ""))
    print(f"session model   {a.session_model}  (${sm['in']}/M in)")
    print(f"worker model    {WORKER_PRICE_KEY}  (${PRICES[WORKER_PRICE_KEY]['in']}/M in)")
    print()
    print("--- the real worker call ---")
    print(f"  wall time              {env['duration_ms']/1000:>10.1f}s"
          + ("   ** would EXCEED a 45s worker_timeout_sec **" if env["duration_ms"] > 45000 else ""))
    print(f"  cost (CLI ground truth) ${worker_usd:>9.6f}")
    print(f"  input                  {u['input_tokens']:>10,}")
    print(f"  cache write            {u['cache_creation_input_tokens']:>10,}"
          f"   (billed {CACHE_WRITE_1H}x input = ${price(u['cache_creation_input_tokens'], PRICES[WORKER_PRICE_KEY]['in'], CACHE_WRITE_1H):.6f})")
    print(f"  cache read             {u['cache_read_input_tokens']:>10,}"
          f"   (billed {CACHE_READ}x input)")
    th = u.get("output_tokens_details", {}).get("thinking_tokens", 0)
    print(f"  output                 {u['output_tokens']:>10,}   (of which thinking: {th:,}"
          f" = ${price(th, PRICES[WORKER_PRICE_KEY]['out']):.6f} of pure overhead)")
    print(f"  skim records this as   {flat_sum:>10,} 'worker_tokens' — a flat sum of four")
    print( "                                    differently-priced quantities, so the")
    print( "                                    number in `skim stats` is not a cost.")
    print()
    print("--- dollars: intercept vs let it through ---")
    print(f"  file is ~{file_tokens:,} tokens; digest is ~{digest_tokens:,} tokens"
          f" ({100*digest_tokens/file_tokens:.1f}% of it)")
    print()
    print(f"  {'turns':>6} {'without skim':>14} {'with skim':>12} {'saved':>11} {'verdict':>9}")
    for t in sorted({0, 5, a.turns, 25, 50}):
        w, s = without(t), with_skim(t)
        print(f"  {t:>6} {w:>13.6f}$ {s:>11.6f}$ {w-s:>+10.6f}$ {'WIN' if w > s else 'LOSS':>9}")
    print()
    # Break-even: smallest turn count where skim wins.
    be = next((t for t in range(0, 501) if without(t) > with_skim(t)), None)
    if be is None:
        print("  VERDICT  skim never pays off on this file at this session model.")
    elif be == 0:
        print("  VERDICT  skim pays off immediately, on the first read.")
    else:
        print(f"  VERDICT  skim pays off only if the content would have survived "
              f"{be}+ further turns.")
    print()
    print("  Note: the counterfactual assumes the session model ingests the whole file.")
    print("  Claude Code's Read tool truncates at ~2000 lines, so for files longer than")
    print("  that the real 'without skim' cost is lower than shown and skim's margin is")
    print("  correspondingly thinner.")


if __name__ == "__main__":
    main()
