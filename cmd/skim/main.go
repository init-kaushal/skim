package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kaushal/skim/internal/cache"
	"github.com/kaushal/skim/internal/cli"
	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/handler"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/paths"
	"github.com/kaushal/skim/internal/runner"
	"github.com/kaushal/skim/internal/worker"
)

func main() { os.Exit(runWithStdin(os.Stdin, os.Args[1:], os.Stdout, os.Stderr)) }

const usage = `usage: skim <command> [args]

hook handlers (invoked by Claude Code, read JSON on stdin):
  read-hook     intercept oversized Read calls
  grep-hook     intercept wide Grep calls
  bash-hook     intercept noisy Bash calls

commands:
  run -- <cmd>  execute a command, capture full output, print a digest
  cat <path>    print a whole file with no interception
  doctor        environment and config diagnostics
  stats         cumulative interception savings
  config        show or change configuration
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
		}
		if err := runner.Run(context.Background(), rest[1:], d); err != nil {
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
		if err := cli.Stats(stdout); err != nil {
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

	case "version":
		fmt.Fprintln(stdout, "skim (dev)")
		return 0

	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// hookFn is the shared shape of handler.ReadHook / GrepHook / BashHook.
type hookFn func(ctx context.Context, in hookio.Input, d handler.Deps) error

// hookMain runs one PreToolUse interception. It parses stdin, loads config
// (falling back to defaults on error), wires the production handler.Deps, and
// invokes fn. It ALWAYS returns 0 — every failure path degrades open.
func hookMain(stdin io.Reader, stdout io.Writer, fn hookFn) int {
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
		CountMatches: rgCountMatches,
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

// sampleCap bounds the rg context dump fed to the worker as the Grep sample.
const sampleCap = 32768

// rgCountMatches backs handler.Deps.CountMatches. It runs
//
//	rg --count-matches --no-heading --no-messages --color never [--glob G] [--type T] -- <pattern> [path]
//
// summing the per-file `path:N` counts, then a second
//
//	rg -n -C1 --no-heading --color never -- <pattern> [path]
//
// capped at 32 KB for the sample. rg exit code 1 (no matches) is not an error;
// any other failure (rg missing, bad regex) is returned so the hook degrades
// open.
func rgCountMatches(gi hookio.GrepInput) (int, string, error) {
	args := []string{"--count-matches", "--no-heading", "--no-messages", "--color", "never"}
	if gi.Glob != "" {
		args = append(args, "--glob", gi.Glob)
	}
	if gi.Type != "" {
		args = append(args, "--type", gi.Type)
	}
	args = append(args, "--", gi.Pattern)
	if gi.Path != "" {
		args = append(args, gi.Path)
	}

	out, err := exec.Command("rg", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return 0, "", nil // rg: no matches
		}
		return 0, "", err
	}

	total := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if i := strings.LastIndex(line, ":"); i >= 0 {
			n := 0
			if _, serr := fmt.Sscanf(line[i+1:], "%d", &n); serr == nil {
				total += n
			}
		}
	}

	sampleArgs := []string{"-n", "-C1", "--no-heading", "--color", "never", "--", gi.Pattern}
	if gi.Path != "" {
		sampleArgs = append(sampleArgs, gi.Path)
	}
	sout, _ := exec.Command("rg", sampleArgs...).Output()
	if len(sout) > sampleCap {
		sout = sout[:sampleCap]
	}
	return total, string(sout), nil
}
