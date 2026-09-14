#!/bin/sh
# Real-API smoke test for skim's Read interception.
#
# Why this exists: the entire fake-`claude` unit suite was green while skim
# silently did nothing in production. Haiku wraps its JSON response in a
# markdown code fence despite being told "no prose", every digest failed to
# parse, and every interception degraded open — safely, invisibly, uselessly.
# No simulated test could catch that. This one makes a real call and asserts
# the hook actually emits a deny carrying a real digest.
#
# Costs a few cents. Draws on your API key or subscription limits.
set -eu

BIN=plugin/bin/skim
[ -x "$BIN" ] || { echo "smoke: $BIN not built — run 'make build'"; exit 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
export SKIM_HOME="$TMP/home"
mkdir -p "$SKIM_HOME"

# A file comfortably over the default 300-line / 60KB thresholds, with real
# structure so the digest has something to describe.
TARGET="$TMP/sample.go"
{
  echo "package sample"
  echo
  i=0
  while [ "$i" -lt 400 ]; do
    echo "// Widget$i models the ${i}th widget in the sample catalogue."
    echo "type Widget$i struct { ID int; Name string; Tags []string }"
    echo
    echo "func (w *Widget$i) Describe() string { return w.Name }"
    echo
    i=$((i + 1))
  done
} > "$TARGET"

echo "smoke: target $(wc -c < "$TARGET" | tr -d ' ') bytes, $(wc -l < "$TARGET" | tr -d ' ') lines"
echo "smoke: calling the real worker (this takes ~20-40s)..."

OUT="$TMP/out.json"
printf '{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"file_path":"%s","offset":0,"limit":0}}' \
  "$TARGET" | "$BIN" read-hook > "$OUT" 2> "$TMP/err.txt" || true

if [ ! -s "$OUT" ]; then
  echo "smoke: FAIL — hook allowed the call (empty stdout), i.e. it degraded open."
  echo "smoke: this is the fence-bug signature. Check the log:"
  [ -f "$SKIM_HOME/skim.log" ] && tail -20 "$SKIM_HOME/skim.log"
  exit 1
fi

if ! grep -q '"permissionDecision":"deny"' "$OUT"; then
  echo "smoke: FAIL — expected a deny decision, got:"
  cat "$OUT"
  exit 1
fi

# The digest must carry real model-written prose, not just skim's own framing.
python3 - "$OUT" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))["hookSpecificOutput"]
reason = d["permissionDecisionReason"]
problems = []
if "Structure:" not in reason:
    problems.append("no Structure: section")
if "Widget" not in reason and "widget" not in reason:
    problems.append("digest never mentions the file's actual subject")
body = [l for l in reason.splitlines() if l.strip()]
if len(body) < 6:
    problems.append(f"digest suspiciously short ({len(body)} lines)")
if problems:
    print("smoke: FAIL —", "; ".join(problems))
    print("--- reason ---"); print(reason)
    sys.exit(1)
print("smoke: PASS — real digest came back and parsed")
print("--- digest ---"); print(reason)
PY

echo "smoke: metrics recorded:"
cat "$SKIM_HOME/metrics.jsonl" 2>/dev/null || echo "  (none — metrics.Record failed)"
