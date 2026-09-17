package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/paths"
	"github.com/kaushal/skim/internal/rg"
	"github.com/kaushal/skim/internal/worker"
)

// Doctor runs environment and configuration checks, writing a human-readable
// report to w. lookPath is injected (normally exec.LookPath) so callers and
// tests can control PATH resolution. It returns 0 when no FAIL lines were
// emitted, else 1.
func Doctor(w io.Writer, lookPath func(string) (string, error)) int {
	failures := 0
	fail := func(format string, a ...any) { failures++; fmt.Fprintf(w, "FAIL  "+format+"\n", a...) }
	ok := func(format string, a ...any) { fmt.Fprintf(w, "ok    "+format+"\n", a...) }
	warn := func(format string, a ...any) { fmt.Fprintf(w, "warn  "+format+"\n", a...) }
	info := func(format string, a ...any) { fmt.Fprintf(w, "      "+format+"\n", a...) }

	if p, err := lookPath("claude"); err != nil {
		fail("`claude` not found on PATH — the worker cannot run; every interception will pass through")
	} else {
		ok("claude: %s", p)
	}
	// Reported through rg.Path, not a bare lookPath("rg"), because a missing
	// standalone ripgrep is not the same thing as no ripgrep. Claude Code ships
	// ripgrep inside its own binary, and skim falls back to it — so checking
	// only for an `rg` executable printed a warning about Grep interception
	// passing through on machines where it works fine.
	if p, argv0, rerr := rg.Path(); rerr != nil {
		warn("ripgrep unavailable (%v) — Grep interception will pass through", rerr)
	} else {
		via := p
		if argv0 == "rg" && p != "rg" {
			via = p + " (via the claude binary; no standalone rg on PATH)"
		}
		// Probe it, because the fallback assumes a binary named `claude` is
		// Claude Code and therefore ripgrep. If it is not, match counts come
		// back quietly wrong rather than failing.
		if ver, verr := rg.Verify(); verr != nil {
			warn("ripgrep at %s is not usable: %v", via, verr)
		} else {
			ok("ripgrep: %s — %s", via, ver)
		}
	}

	// Which transport the worker will use is worth stating plainly: the CLI
	// fallback pays for Claude Code's own ~5.5K-token system prompt on every
	// interception, which dominates the bill on small files, and nothing else
	// surfaces the difference.
	if tr := worker.ActiveTransport(); tr.Direct {
		ok("worker transport: direct API (%s)", tr.Why)
	} else {
		info("worker transport: claude CLI — %s", tr.Why)
	}

	c, err := config.Load()
	if err != nil {
		fail("config: %v", err)
	} else if verr := c.Validate(); verr != nil {
		fail("config invalid: %v", verr)
	} else {
		ok("config: %s", paths.Config())
		if reason := c.NotActiveReason(os.Getenv); reason != "" {
			info("skim is currently INACTIVE: %s", reason)
		}
	}

	if entries, err := os.ReadDir(paths.CacheDir()); err == nil {
		var total int64
		for _, e := range entries {
			if fi, err := e.Info(); err == nil {
				total += fi.Size()
			}
		}
		info("cache: %d entries, %d KB", len(entries), total/1024)
	}

	if f, err := os.Open(paths.Log()); err == nil {
		defer f.Close()
		var last []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			last = append(last, sc.Text())
			if len(last) > 5 {
				last = last[1:]
			}
		}
		if len(last) > 0 {
			info("last %d log line(s):", len(last))
			for _, l := range last {
				info("  %s", l)
			}
		}
	}

	if failures == 0 {
		fmt.Fprintln(w, "\nskim: OK")
		return 0
	}
	fmt.Fprintf(w, "\nskim: %d problem(s)\n", failures)
	return 1
}
