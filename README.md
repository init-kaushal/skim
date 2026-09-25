# skim

![Claude Code plugin](https://img.shields.io/badge/Claude%20Code-plugin-c2410c?style=flat-square)
![Go 1.23](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go&logoColor=white)
![Uses the PreToolUse hook](https://img.shields.io/badge/hook-PreToolUse-6b7280?style=flat-square)
![Worker model: Haiku 4.5](https://img.shields.io/badge/worker-haiku--4.5-8b5cf6?style=flat-square)
![Fails open](https://img.shields.io/badge/fails-open-067647?style=flat-square)
![No dependencies](https://img.shields.io/badge/deps-none-64748b?style=flat-square)
![MIT license](https://img.shields.io/badge/license-MIT-3b82f6?style=flat-square)

**[Documentation & landing page →](https://init-kaushal.github.io/skim/)**

A Claude Code plugin that intercepts oversized `Read`, wide `Grep`, and noisy
`Bash` tool calls before they run and substitutes a compact, Haiku-produced
digest — so the main session's context (and token bill, and 5-hour/weekly
usage limits) isn't spent absorbing raw file dumps, grep floods, and command
output that get re-sent on every subsequent turn.

> **Status:** v1 complete and merged to `main`. All 15 planned tasks are
> implemented and reviewed, followed by four post-implementation fix waves
> (see the handoff notes for the full ledger). Every fix wave has had either
> an independent review pass or a direct empirical check against a real built
> binary. Picking this up fresh? Read
> [`docs/superpowers/notes/2026-09-14-handoff.md`](docs/superpowers/notes/2026-09-14-handoff.md)
> first — it has the full history and every deferred follow-up.

## What it does

Claude Code agents spend a large share of their token budget on
non-reasoning work: reading big files, scanning wide grep results, and
absorbing noisy command output (test runs, `curl`, log dumps). That bulk
content lands in the main context and is then re-sent on every subsequent
turn, so the cost compounds. `skim` reproduces the useful core of Spotify's
"Portal + shunt" pattern locally, with no external platform: it hard-blocks
expensive `Read`/`Grep`/`Bash` operations from the expensive model and routes
them to a cheap worker model instead, handing back a structured digest (a
~500-token file map or match-cluster summary) rather than thousands of
tokens of raw content.

**skim is model-agnostic on the driving side.** It does not branch on which
model runs the main Claude Code session — Sonnet, Opus, Fable, or whatever a
future release adds. It hooks tool calls, not the model, so the detection and
substitution logic is identical for all of them. Only the worker paths are
pinned to a cheap tier: the interception worker defaults to
`claude-haiku-4-5-20251001` (configurable), and the separate `code-writer`
subagent is pinned to Haiku in its own frontmatter, independent of the
interception worker's model setting. The value of what's saved scales with
the session model's per-token weight — highest on Opus, lowest when the
session already runs on Haiku.

## How it works

The only lever `skim` uses is Claude Code's `PreToolUse` hook: **deny the
tool call, and put a replacement digest in `permissionDecisionReason`.**
There's no argument rewriting and no `PostToolUse` result substitution — deny
+ reason text is the one mechanism, and it's stable across Claude Code
versions. When a hook allows a call through (exit 0, empty stdout), the tool
runs completely normally.

Three interception paths, each registered as a `PreToolUse` hook in
`plugin/hooks/hooks.json`:

- **`Read`** — pass through if the call already has `offset`/`limit` (a
  targeted read is intentional), if the path matches a `passthrough_globs`
  entry, or if the file is under the size/line thresholds. Otherwise, look up
  a digest cache; on a miss, invoke the worker to produce a **file map**
  (summary + line-range map + symbol list), cache it, and deny with the
  rendered digest plus instructions for getting exact lines (`Read` with
  `offset`/`limit`, or `skim cat <path>` to force the full file).
- **`Grep`** — the hook runs `rg` itself and counts matches. Under
  `grep_max_matches`, it passes through (Claude Code's own `Grep` runs too —
  a cheap duplicate). Over the threshold, the worker produces a **clusters**
  digest (matches grouped by directory/symbol, with counts and representative
  files), and the call is denied with that digest.
- **`Bash`** — the command string is matched against `bash_noisy_patterns`
  (e.g. `cat`, `curl`, `tail -n <large>`, test runners). A match is denied
  with instructions to re-run as `skim run -- <cmd>`, which executes the
  command, streams the full stdout+stderr to a log file under
  `~/.claude/skim/runs/`, and returns a **run** digest (summary, key lines,
  exit code, log path) — nothing is discarded, it's just off to the side.

**Degrade-open guarantee.** If the worker exits non-zero, times out, or
returns output that doesn't parse against the expected schema, the hook
emits allow/no-decision so the original tool call proceeds exactly as if
`skim` weren't installed. The failure is logged to `~/.claude/skim/skim.log`.
The optimizer breaking must never block the user — every degradation path
fails toward the unmodified Claude Code behavior, never toward blocking.

A recursion guard (`SKIM_ACTIVE=1`, set on every nested worker process) makes
every hook a no-op the moment it detects it's running inside skim's own
worker call, so the worker's own tool use never re-enters the hooks.

## What the worker is shown

The Read hook ships at most **2000 lines** (with a 128KB backstop for minified
or single-line files) to the worker, deliberately matching Claude Code's own
Read cap. Digesting past that cannot save anything — the surplus was never going
to reach the session's context — it only adds worker cost. When a file is longer,
the digest says so explicitly and gives the line offset to continue from, rather
than passing a prefix off as a map of the whole file.

Tests use a **fake `claude` stub** placed on `PATH` during the test run: a
script that echoes canned JSON in the `claude -p --output-format json` shape,
so the full hook round-trip (detect → invoke worker → parse → render deny
reason) runs deterministically end-to-end with zero real API calls. This is
also how degradation is tested — the stub can return malformed JSON, exit
non-zero, or sleep past `worker_timeout_sec` to assert the hook falls back to
allow/no-decision and logs the failure. Digest renderers (file map, clusters,
run) are covered by golden-file tests.

## How the worker talks to the model

skim has two transports and prefers the cheaper one:

**No API key is required.** skim runs on whatever credentials Claude Code
already has. The key only selects a cheaper transport if you happen to have one.

| | when | fixed cost per call, before file content |
|---|---|---|
| **`claude -p` CLI** | the default — anything Claude Code can authenticate, including a subscription | ~1,300 tokens (a minimal system prompt replaces Claude Code's ~5,500-token default) |
| **direct API** | `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` happens to be set | ~0 |

The direct path posts once to `/v1/messages` with no system prompt, no
`cache_control` (the content is read once and never re-read, so a cache write at
1.25–2x input would be pure loss), no thinking, and no child process. It needs
no recursion guard either, because there is no nested Claude Code session to
re-enter skim's own hooks.

**On a subscription you get the CLI transport, and that is the supported path.**
Claude Code keeps its OAuth token in the OS keychain, and skim deliberately does
not read it: borrowing a session credential for out-of-band API calls is fragile
against refresh and expiry, is a credential-exfiltration pattern whatever the
intent, and a subscription is not an API entitlement. So the CLI transport is
what a plugin install actually uses, and it is tuned for that — it passes its
own minimal `--system-prompt`, which displaces Claude Code's default and saves
**4,210 input tokens per call** (measured: 10,864 → 6,654 cache-write tokens,
about $0.0084 an interception). That also stops the project's `CLAUDE.md` and
environment details leaking into a call that only has to describe one file.

`skim doctor` prints which transport is active.

One honest trade in the direct path: the Messages API reports tokens but not
money — `total_cost_usd` is a CLI convenience — so that transport prices its own
calls from a small table of worker-model rates. The CLI transport's cost figure
is authoritative and survives price changes; the direct one can go stale. An
unrecognised model records no cost rather than a guess, and `skim stats` says
how many calls reported one.

## Install

```bash
claude plugin marketplace add init-kaushal/skim      # or a local checkout path
claude plugin install skim@skim
```

Then, inside a Claude Code session:

```
/skim doctor
```

**No Go toolchain needed.** `plugin/bin/skim` is a small tracked launcher, not
the binary. On first use it downloads the prebuilt binary for your platform from
the matching GitHub release and **verifies it against the published
`SHA256SUMS` before running it** — a download whose hash does not match is
discarded, never executed. It is cached under `~/.claude/skim/bin/`, so this
happens once per version, not once per session.

Resolution order, first hit wins:

1. `plugin/bin/skim-bin` — a local `make build`, so a source checkout always
   beats a download and you never run a stale artifact while developing
2. an already-downloaded release binary in the cache
3. a fresh, checksum-verified download
4. `go build`, **only when the Go module is actually reachable** — which is
   true for a source checkout run with `--plugin-dir`, and *not* true for an
   installed plugin: `claude plugin install` copies only `plugin/` into
   `~/.claude/plugins/cache/`, with no `go.mod` above it

So for an installed plugin the download is the route that matters, and a
published release is required for one to work at all.

Prebuilt for `darwin/arm64`, `darwin/amd64`, `linux/amd64` and `linux/arm64`.
Windows is not published: the launcher is a POSIX shell script, so a Windows
binary would ship with nothing able to start it.

If every route fails, the hooks **exit 0 and do nothing** — your tool calls
behave exactly as if skim were not installed — and the reason is written to
`~/.claude/skim/skim.log`. `/skim doctor` says so out loud rather than staying
quiet. `SKIM_NO_DOWNLOAD=1` skips the download entirely.

`doctor` also confirms `claude` is on `PATH` and runnable, that ripgrep is
reachable, that the config file is present and valid, and reports the last few
errors from `skim.log` plus cache size.

Releases are cut by pushing a `v*` tag: CI runs the full suite, cross-compiles
every target, publishes them with their checksums, and refuses to release if
the version baked into the launcher does not match the tag.

## Try it

Two ways, depending on whether you want to install it.

**Without installing anything.** `--plugin-dir` loads a plugin for one session
only, so this touches no config:

```bash
make build
cd /some/project
claude --plugin-dir /path/to/skim/plugin
```

**Then, in the session:**

```
/skim demo
```

That writes a ~680-line sample into the current directory and tells you to
`Read` it. The Read gets intercepted and you get a ~25-line structural digest
back instead of the file — and the model can still answer questions about it.
To see the contrast, `/skim off`, read it again to get the full file, then
`/skim on`.

`/skim stats` shows what it saved and what the worker cost.

The sample is written into *your* directory rather than shipped inside the
plugin on purpose: an installed plugin lives under `~/.claude/plugins/`, which
matches the default `.claude/**` passthrough glob, so a bundled sample would be
passed through untouched and the demo would appear to do nothing. `skim demo`
refuses to write anywhere under `.claude/` for the same reason.

## Configuration

`~/.claude/skim/config.json`, created with defaults on first `skim doctor`
run:

| Key | Default | Meaning |
|---|---|---|
| `disabled` | `false` | Master kill switch — toggled by `/skim off` / `/skim on`. |
| `read_max_lines` | `300` | Files at or under this line count pass through `Read` uninterrupted. |
| `read_max_bytes` | `60000` | Files at or under this byte size pass through `Read` uninterrupted. |
| `grep_max_matches` | `60` | Grep results at or under this many *matching lines* pass through uninterrupted. Counted with `rg --count` (lines), matching what Grep returns in content mode — not `--count-matches` (occurrences), which over-counted any line containing more than one hit. |
| `bash_noisy_patterns` | `["\\bcat\\s", "\\bcurl\\s", "\\btail\\s+-n\\s+\\d{3,}", "npm\\s+(run\\s+)?test", "jest", "pytest", "go\\s+test"]` | Regexes matched against the `Bash` command string; a match redirects to `skim run --`. |
| `passthrough_globs` | `["**/*.md", "**/go.mod", ".claude/**"]` | Paths that always skip interception, regardless of size. |
| `model` | `claude-haiku-4-5-20251001` | Model passed explicitly (`--model`) to every worker call — never inherited from the session model. |
| `worker_timeout_sec` | `45` | Worker call timeout before the hook degrades open. Capped at 55s, under the 60s timeout the plugin declares for its hooks. |

### Kill switches (checked in this order)

1. **`SKIM_DISABLED=1`** in the environment — forces every hook to a no-op
   pass-through, checked before anything else.
2. **`"disabled": true`** in config, toggled by `/skim off` / `/skim on`.
3. **Invalid `config.model`** — empty, or a model this CLI cannot run.
   `config.model` is validated once at config load (not per call); an
   invalid value forces the same pass-through state as an explicit disable,
   so no interception ever pays for a doomed worker round-trip. `skim doctor`
   surfaces the bad value.
4. **`passthrough_globs`** — a per-path opt-out that applies even when skim
   is otherwise fully enabled.

When disabled by any of the above, every hook allows its tool call through
immediately with no nested `claude -p` attempt.

## Reading `skim stats`

```
  tool     intercepts   tokens saved     worker $
  Bash              1              0       0.0000
  Read              1            829       0.0425
  Run               1          14892       0.0425

  note    Bash only redirects; its savings and cost appear on the Run line.
  cache   0 hit / 1 miss (0% hit rate, Read only)

  assuming a opus-5 session (5.00 $/Mtok in), content surviving 10 more turns:
    saved    ~15721 tokens  =  $0.0786 on first read, $0.1572 with re-sends
    spent    $0.0850 actually billed by the worker (2 of 3 calls reported cost)
    net      $+0.0722  — ahead
```

Three things are deliberate here:

- **The verdict is in dollars, not tokens.** A worker call's input, output,
  cache-write and cache-read tokens bill at 1x, 5x, 2x and 0.1x of the input
  rate, and against a different model than the session — so subtracting a token
  sum from tokens-saved compares quantities that share no unit. The cost figure
  comes from the CLI's own `total_cost_usd` per call, not from a price table
  compiled into skim, so it stays right when prices change.
- **The session model is an assumption, and says so.** The `PreToolUse` payload
  does not carry it, so `stats` cannot detect it. Pass `--session-model` to match
  your setup; the verdict genuinely flips between Opus and a Haiku session.
- **`Bash` and `Run` are separate rows.** The Bash hook only redirects — it makes
  no worker call and saves nothing by itself. The saving and the cost both land
  on the `Run` row, when `skim run` actually executes and digests the command.
  A `Bash` row of zeroes is correct, not a bug.

The cache row counts only `Read`, the one path with a digest cache. Counting
Grep and Bash as misses is what previously made it read "0 hit / 4 miss (0% hit
rate)" on one Read plus three Bash interceptions.

## Limitations

- **Latency.** Every uncached large `Read`/`Grep`/noisy `Bash` interception
  adds one nested `claude -p` call. Measured at 6–13 seconds across file sizes
  from 17KB to 400KB — near-flat, because the cost is the call itself rather
  than the content. Caching absorbs repeat reads of the same file (keyed on
  path + mtime + size + model + schema version), and thresholds are meant to
  only catch genuinely large operations, but if this proves too intrusive
  interactively, `/skim off` is the immediate escape hatch.
- **Approximate line numbers.** The worker-produced file map's line ranges
  are approximate (nominally ±3 lines) — they come from a Haiku pass over the
  content, not an exact parse. When you need precise lines, `Read` again with
  `offset`/`limit` on the range of interest, or run `skim cat <path>` to
  force the full, unmodified file.
- **`skim run` changes what the Bash permission system matches on.** When a
  noisy command is redirected, the command Claude Code actually evaluates is
  `skim run -- <original>` (or `skim run -- sh -c '<original>'` when the
  command uses pipes, redirects or substitutions), not the original string. If
  you rely on narrow Bash allow/deny rules, they will see the `skim run …`
  form — write your rules accordingly, or run `/skim off` in sessions where
  exact Bash matching matters.
- **`Grep` interception needs ripgrep, but almost certainly already has it.**
  The hook shells out to ripgrep to count matches before deciding whether to
  intercept. It prefers a standalone `rg` on `PATH` and otherwise falls back to
  Claude Code's own binary, which *is* ripgrep 14.1.1 when invoked with
  `argv[0]` set to `rg`. That fallback is load-bearing: a normal Claude Code
  install has no `rg` executable at all — on some setups `rg` is a shell
  *function* that re-execs `claude`, and a shell function is invisible to an
  exec PATH lookup. Without the fallback, Grep interception silently never ran.
  `skim doctor` reports which route it resolved and probes it to confirm it
  really is ripgrep.
- **Whether it saves money depends on the session model, and it is close.**
  The worker is cheap per token but not free, and it reads content to compress
  it — so the trade is a ratio, not a given. Haiku input is 5x cheaper than
  Opus 5 input and 3x cheaper than Sonnet 5, which sets the margin. Measured
  with `make bench`: on Opus 5 a long file pays off on the very first read
  (2x), while a file just over the 300-line threshold needs to stay relevant
  for ~3 more turns before it breaks even. On a Haiku session it does not pay
  off at all. **Run `make bench FILE=<a file you actually read>` before
  trusting this on your workload**, and `skim stats` for the running total across
  a real session.
- **Savings shrink as the session model gets cheaper.** The whole mechanic
  moves bulk content, and its re-send cost, out of the session model's
  context — the value of that scales with the session model's per-token
  weight. It's largest when the session runs on an expensive model (e.g.
  Opus) and smallest — potentially negative once nested worker calls are
  counted — when the session already runs on Haiku. In that case, raising the
  thresholds or running `/skim off` is the right call; `skim stats` is what
  tells you which way the trade is going in practice.

## Development

```bash
make build   # go build -o plugin/bin/skim ./cmd/skim
make check   # build + vet + gofmt + test, no API calls
make smoke   # REAL API call: asserts interception actually works end to end
make bench FILE=<path> [MODEL=opus-5] [TURNS=10]
             # REAL API call: prices interception against letting the Read through
```

`make smoke` exists because the entire fake-`claude` suite was green while skim
silently did nothing in production: Haiku wraps its JSON in a markdown code
fence, so every digest failed to parse and every interception degraded open —
safely, invisibly, uselessly. Only a real call catches that class of bug. Run it
before any release.

`make bench` measures a single file in isolation: it makes one real worker call,
takes `total_cost_usd` as ground truth, and reports the turn count at which
interception breaks even. Use it to decide whether skim suits a given kind of
file. Use `skim stats` for the running total across a real session.

