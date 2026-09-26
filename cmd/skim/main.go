package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaushal/skim/internal/cache"
	"github.com/kaushal/skim/internal/cli"
	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/handler"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/paths"
	"github.com/kaushal/skim/internal/rg"
	"github.com/kaushal/skim/internal/runner"
	"github.com/kaushal/skim/internal/worker"
)

// version is stamped at build time by scripts/build-release.sh via
// -ldflags "-X main.version=...". It stays "dev" for a plain `go build`, which
// is how you can tell a local build from a released artifact.
var version = "dev"

func main() { os.Exit(runWithStdin(os.Stdin, os.Args[1:], os.Stdout, os.Stderr)) }

const usage = `usage: skim <command> [args]

hook handlers (invoked by Claude Code, read JSON on stdin):
  read-hook     intercept oversized Read calls
  grep-hook     intercept wide Grep calls
  bash-hook     intercept noisy Bash calls

commands:
  run -- <cmd>  execute a command, capture full output, print a digest
  cat <path>    print a whole file with no interception
  demo [dir]    write a sample file big enough to trigger interception
  doctor        environment and config diagnostics
  stats         cumulative interception savings
  config        show or change configuration
  install-shell add skim to PATH in your shell rc file (--dry-run to preview)
  version       print version
`

// run keeps Task 2's signature intact for cmd/skim/main_test.go. It threads an
// empty stdin into the real dispatch.
func run(args []string, stdout, stderr io.Writer) int {
	return runWithStdin(strings.NewReader(""), args, stdout, stderr)
}

// runWithStdin is the real entry point: it dispatches the subcommand, wiring the
// production collaborators from every internal package. Hook subcommands always
// return 0 — a broken optimiser must never block a tool call.
func runWithStdin(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "read-hook":
		return hookMain(stdin, stdout, handler.ReadHook)
	case "grep-hook":
		return hookMain(stdin, stdout, handler.GrepHook)
	case "bash-hook":
		return hookMain(stdin, stdout, handler.BashHook)

	case "run":
		rest := args[1:]
		if len(rest) < 2 || rest[0] != "--" {
			fmt.Fprintln(stderr, "usage: skim run -- <command> [args...]")
			return 2
		}
		cfg := loadConfigOrDefault(nil)
		d := runner.Deps{
			Summarize:  worker.Run,
			Model:      cfg.Model,
			TimeoutSec: cfg.WorkerTimeoutSec,
			Now:        time.Now,
			Stdout:     stdout,
			RunsDir:    paths.RunsDir(),
			Record:     func(e metrics.Entry) { _ = metrics.Record(e) },
		}
		if err := runner.Run(context.Background(), rest[1:], d); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	case "demo":
		dir := ""
		if len(args) > 1 {
			dir = args[1]
		}
		if err := cli.Demo(stdout, dir); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	case "cat":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: skim cat <path>")
			return 2
		}
		if err := cli.Cat(stdout, args[1]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	case "doctor":
		return cli.Doctor(stdout, exec.LookPath)

	case "stats":
		if err := cli.Stats(stdout, args[1:]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	case "config":
		if err := cli.Config(stdout, args[1:]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	// `/skim off` / `/skim on` is the advertised kill switch in
	// plugin/commands/skim.md, which passes $ARGUMENTS through verbatim — so
	// the bare forms must dispatch, not just `skim config off`/`on`.
	case "off", "on":
		if err := cli.Config(stdout, args[0:1]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	case "install-shell":
		dryRun := len(args) > 1 && args[1] == "--dry-run"
		selfDir, err := selfBinDir()
		if err != nil {
			fmt.Fprintln(stderr, "skim: could not resolve own directory:", err)
			return 1
		}
		if err := cli.InstallShell(stdout, selfDir, dryRun); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	case "version":
		fmt.Fprintf(stdout, "skim %s\n", version)
		return 0

	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// selfBinDir returns the directory containing the running binary, following
// any symlinks. Used by install-shell to put the right path on PATH.
func selfBinDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

// hookFn is the shared shape of handler.ReadHook / GrepHook / BashHook.
type hookFn func(ctx context.Context, in hookio.Input, d handler.Deps) error

// hookMain runs one PreToolUse interception. It parses stdin, loads config
// (falling back to defaults on error), wires the production handler.Deps, and
// invokes fn. It ALWAYS returns 0 — every failure path degrades open.
func hookMain(stdin io.Reader, stdout io.Writer, fn hookFn) int {
	// Spec §4.0: the recursion guard comes first. Both of these are pure env
	// vars needing no config, so answer them before paying for a config read
	// (which may write a file) and a cache directory sweep on every tool call.
	// The config-file `disabled` flag and Validate() still run inside the
	// handler via NotActiveReason — precedence is unchanged, just short-circuited.
	if os.Getenv("SKIM_ACTIVE") == "1" || os.Getenv("SKIM_DISABLED") == "1" {
		return 0
	}

	in, err := hookio.Parse(stdin)
	if err != nil {
		return 0 // malformed input: allow the tool call
	}

	logf := func(format string, a ...any) {
		f, e := os.OpenFile(paths.Log(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if e != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, time.Now().UTC().Format(time.RFC3339)+" "+format+"\n", a...)
	}

	cfg := loadConfigOrDefault(logf)

	// Best-effort GC of stale digest cache entries; failures are non-fatal.
	_, _ = cache.Sweep(7 * 24 * time.Hour)

	d := handler.Deps{
		Cfg:          cfg,
		Env:          os.Getenv,
		Summarize:    worker.Run,
		CacheKey:     cache.Key,
		CacheGet:     cache.Get,
		CachePut:     cache.Put,
		Record:       func(e metrics.Entry) { _ = metrics.Record(e) },
		Logf:         logf,
		Now:          time.Now,
		Stdout:       stdout,
		CountMatches: rg.Count,
		Sample:       rg.Sample,
	}
	_ = fn(context.Background(), in, d)
	return 0
}

// loadConfigOrDefault returns config.Load()'s result, or config.Default() if the
// load fails. When logf is non-nil, a load failure is logged.
func loadConfigOrDefault(logf func(string, ...any)) config.Config {
	cfg, err := config.Load()
	if err != nil {
		if logf != nil {
			logf("config load: %v (using defaults)", err)
		}
		return config.Default()
	}
	return cfg
}
