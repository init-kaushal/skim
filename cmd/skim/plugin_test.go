package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot locates the module root from this test's package directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestShippedPluginCanRunItsHooks is the regression guard for the worst bug
// this project has had: plugin/bin/ was gitignored, so every installed copy
// shipped a hooks.json pointing at ${CLAUDE_PLUGIN_ROOT}/bin/skim with no such
// file. Every hook failed to exec, Claude Code let the tool call through, and
// skim did nothing at all on every install that was not the author's own build
// directory — silently, because failing open is indistinguishable from working
// when you cannot see the digest that never arrived.
//
// The invariant: every command in hooks.json must resolve to a file that is
// present and executable in a *clean checkout*, before anything is built.
func TestShippedPluginCanRunItsHooks(t *testing.T) {
	root := repoRoot(t)
	pluginDir := filepath.Join(root, "plugin")

	raw, err := os.ReadFile(filepath.Join(pluginDir, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("hooks.json does not parse: %v", err)
	}

	seen := 0
	for event, entries := range doc.Hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				seen++
				// The command is "<path> <subcommand>"; the path is field one.
				fields := strings.Fields(h.Command)
				if len(fields) == 0 {
					t.Errorf("%s/%s: empty command", event, e.Matcher)
					continue
				}
				resolved := strings.ReplaceAll(fields[0], "${CLAUDE_PLUGIN_ROOT}", pluginDir)

				info, err := os.Stat(resolved)
				if err != nil {
					t.Errorf("%s/%s: hook command %q resolves to %s, which does not exist — "+
						"an installed plugin would fail to exec it and silently never intercept",
						event, e.Matcher, h.Command, resolved)
					continue
				}
				if info.Mode()&0o111 == 0 {
					t.Errorf("%s/%s: %s is not executable (mode %v)",
						event, e.Matcher, resolved, info.Mode())
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("hooks.json declared no hooks at all")
	}
}

// TestHookEntrypointIsTrackedAndExecutable checks the same invariant against
// git rather than the working tree. A file that exists only because a local
// build produced it, or that is tracked without its executable bit, would pass
// the test above on the author's machine and fail for every user.
func TestHookEntrypointIsTrackedAndExecutable(t *testing.T) {
	root := repoRoot(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	out, err := exec.Command("git", "-C", root, "ls-files", "-s", "plugin/bin/skim").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		t.Fatal("plugin/bin/skim is not tracked by git — it will not reach any user, " +
			"and the hooks point at it")
	}
	if !strings.HasPrefix(line, "100755") {
		t.Errorf("plugin/bin/skim is tracked with mode %q, want 100755; "+
			"without the executable bit the hooks cannot run it",
			strings.Fields(line)[0])
	}

	// And the ignore rules must not quietly re-swallow it.
	ck := exec.Command("git", "-C", root, "check-ignore", "-q", "plugin/bin/skim")
	if err := ck.Run(); err == nil {
		t.Error("plugin/bin/skim is matched by .gitignore")
	}
}

// TestLauncherDegradesOpenWithoutABinary pins the launcher's most important
// property. If it cannot produce a binary, a hook subcommand must still exit 0
// so the tool call proceeds — a broken optimiser must never block the user —
// while a user-facing subcommand must say what is wrong rather than exit
// silently, which is what made the original bug invisible.
func TestLauncherDegradesOpenWithoutABinary(t *testing.T) {
	root := repoRoot(t)
	launcher, err := os.ReadFile(filepath.Join(root, "plugin", "bin", "skim"))
	if err != nil {
		t.Fatal(err)
	}

	// Stage a plugin tree with the launcher but no module to build from, so the
	// build attempt cannot succeed.
	dir := t.TempDir()
	binDir := filepath.Join(dir, "plugin", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(binDir, "skim")
	if err := os.WriteFile(path, launcher, 0o755); err != nil {
		t.Fatal(err)
	}

	home := filepath.Join(dir, "skimhome")
	run := func(arg string) (int, string) {
		cmd := exec.Command(path, arg)
		cmd.Env = append(os.Environ(), "SKIM_HOME="+home)
		var sb strings.Builder
		cmd.Stdout, cmd.Stderr = &sb, &sb
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("running launcher: %v", err)
		}
		return code, sb.String()
	}

	for _, hook := range []string{"read-hook", "grep-hook", "bash-hook"} {
		code, out := run(hook)
		if code != 0 {
			t.Errorf("%s exited %d with no binary; hooks must degrade open (exit 0). output: %s",
				hook, code, out)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("%s wrote %q to stdout/stderr; a hook must stay silent so Claude Code "+
				"does not read it as a decision", hook, out)
		}
	}

	code, out := run("doctor")
	if code == 0 {
		t.Error("doctor should fail loudly when there is no binary, not exit 0")
	}
	if !strings.Contains(out, "make build") {
		t.Errorf("the error should tell the user how to fix it, got: %q", out)
	}

	// The failure has to be diagnosable after the fact.
	log, err := os.ReadFile(filepath.Join(home, "skim.log"))
	if err != nil || !strings.Contains(string(log), "skim-launcher") {
		t.Errorf("launcher should leave a note in skim.log; got err=%v log=%q", err, log)
	}
}
