package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		cmd.Env = append(os.Environ(), "SKIM_HOME="+home, "SKIM_NO_DOWNLOAD=1")
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

// TestLauncherVersionMatchesManifest keeps the two places a version is written
// in step. The launcher downloads skim-<SKIM_VERSION>-<os>-<arch>, so if it
// drifts from the plugin manifest — and therefore from the tag that was
// released — every install 404s on first use and silently falls back to
// building from source, or to nothing at all.
func TestLauncherVersionMatchesManifest(t *testing.T) {
	root := repoRoot(t)

	launcher, err := os.ReadFile(filepath.Join(root, "plugin", "bin", "skim"))
	if err != nil {
		t.Fatal(err)
	}
	var declared string
	for _, line := range strings.Split(string(launcher), "\n") {
		if strings.HasPrefix(line, "SKIM_VERSION=") {
			declared = strings.Trim(strings.TrimPrefix(line, "SKIM_VERSION="), `"`)
			break
		}
	}
	if declared == "" {
		t.Fatal("launcher declares no SKIM_VERSION")
	}

	raw, err := os.ReadFile(filepath.Join(root, "plugin", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}

	if declared != manifest.Version {
		t.Errorf("launcher SKIM_VERSION=%q but plugin.json version=%q; "+
			"the launcher would download an asset the release does not contain",
			declared, manifest.Version)
	}
}

// launcherIn stages the real launcher in a throwaway plugin tree with no module
// beside it, so source builds cannot mask the download path under test.
func launcherIn(t *testing.T) (launcher, home string) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "plugin", "bin", "skim"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "plugin", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(binDir, "skim")
	if err := os.WriteFile(p, src, 0o755); err != nil {
		t.Fatal(err)
	}
	return p, filepath.Join(dir, "home")
}

func runLauncher(t *testing.T, launcher, home, base, arg string) (int, string) {
	t.Helper()
	cmd := exec.Command(launcher, arg)
	cmd.Env = append(os.Environ(), "SKIM_HOME="+home, "SKIM_DOWNLOAD_BASE="+base)
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

// TestLauncherRefusesATamperedDownload is the security property. The launcher
// runs unattended on every tool call, so a downloaded artifact it cannot verify
// against the published checksums is a remote code execution path, not an
// inconvenience. It must refuse, discard, and say so.
func TestLauncherRefusesATamperedDownload(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		if _, err := exec.LookPath("wget"); err != nil {
			t.Skip("no curl or wget to download with")
		}
	}

	// A release whose checksum file does not describe its binary.
	dist := t.TempDir()
	payload := "#!/bin/sh\necho PWNED\n"
	name := "skim-" + launcherVersion(t) + "-" + hostOS() + "-" + hostArch()
	if err := os.WriteFile(filepath.Join(dist, name), []byte(payload), 0o755); err != nil {
		t.Fatal(err)
	}
	sums := "0000000000000000000000000000000000000000000000000000000000000000  " + name + "\n"
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(sums), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dist)))
	defer srv.Close()

	launcher, home := launcherIn(t)

	code, out := runLauncher(t, launcher, home, srv.URL, "version")
	if strings.Contains(out, "PWNED") {
		t.Fatal("the launcher EXECUTED a binary whose checksum did not match")
	}
	if code == 0 {
		t.Errorf("expected a non-zero exit for a user-facing command, got 0: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "bin", name)); err == nil {
		t.Error("an unverified download must not be cached")
	}
	log, _ := os.ReadFile(filepath.Join(home, "skim.log"))
	if !strings.Contains(string(log), "checksum mismatch") {
		t.Errorf("the rejection should be diagnosable from the log, got: %s", log)
	}

	// And a hook must still not break the tool call.
	code, out = runLauncher(t, launcher, home, srv.URL, "read-hook")
	if code != 0 || strings.TrimSpace(out) != "" {
		t.Errorf("hook must degrade open silently; exit=%d out=%q", code, out)
	}
}

// TestLauncherRunsAVerifiedDownload is the happy path: a correctly published
// release is downloaded, verified, cached and executed, with no Go toolchain
// involved.
func TestLauncherRunsAVerifiedDownload(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dist := t.TempDir()
	name := "skim-" + launcherVersion(t) + "-" + hostOS() + "-" + hostArch()
	// Stand-in for the real binary; the launcher only has to fetch, verify and
	// exec it, and a shell script proves that end to end without a 6MB build.
	payload := "#!/bin/sh\necho launched-ok\n"
	bin := filepath.Join(dist, name)
	if err := os.WriteFile(bin, []byte(payload), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256File(t, bin)
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"),
		[]byte(sum+"  "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dist)))
	defer srv.Close()

	launcher, home := launcherIn(t)
	code, out := runLauncher(t, launcher, home, srv.URL, "version")
	if code != 0 {
		t.Fatalf("exit=%d out=%q", code, out)
	}
	if !strings.Contains(out, "launched-ok") {
		t.Errorf("verified binary was not executed, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(home, "bin", name)); err != nil {
		t.Errorf("verified download should be cached for next time: %v", err)
	}
}

func launcherVersion(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "plugin", "bin", "skim"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "SKIM_VERSION=") {
			return strings.Trim(strings.TrimPrefix(line, "SKIM_VERSION="), `"`)
		}
	}
	t.Fatal("no SKIM_VERSION in launcher")
	return ""
}

func hostOS() string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		return runtime.GOOS
	}
	return "unsupported"
}

func hostArch() string {
	switch runtime.GOARCH {
	case "arm64", "amd64":
		return runtime.GOARCH
	}
	return "unsupported"
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestManifestDoesNotRedeclareAutoloadedPaths guards a failure that
// `claude plugin validate` does not catch and that only appears at install
// time — where it takes the whole plugin down, not just one feature:
//
//	Status: ✘ failed to load
//	Error: Hook load failed: Duplicate hooks file detected: ./hooks/hooks.json
//	resolves to already-loaded file … The standard hooks/hooks.json is loaded
//	automatically, so manifest.hooks should only reference additional hook files.
//
// The standard directories and files are discovered on their own. Naming one
// in plugin.json asks for it to be loaded twice. The manifest fields exist for
// *extra* paths only — superpowers, for instance, uses `hooks` to point at a
// non-standard hooks-cursor.json.
func TestManifestDoesNotRedeclareAutoloadedPaths(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "plugin", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}

	// field in plugin.json -> the path it must not point at
	standard := map[string]string{
		"hooks":    "hooks/hooks.json",
		"commands": "commands",
		"agents":   "agents",
		"skills":   "skills",
	}
	for field, stdPath := range standard {
		v, present := manifest[field]
		if !present {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue // a non-string form is not this mistake
		}
		norm := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "./"), "/")
		if norm == stdPath {
			t.Errorf("plugin.json declares %q: %q, which is the auto-loaded standard path. "+
				"Claude Code loads it anyway and then refuses the plugin with a duplicate-load "+
				"error, so the whole plugin fails to load. Remove the field, or point it at an "+
				"additional non-standard file.", field, s)
		}
	}

	// The directories themselves must still be there to be discovered.
	for _, must := range []string{"hooks/hooks.json", "commands", "agents"} {
		if _, err := os.Stat(filepath.Join(root, "plugin", must)); err != nil {
			t.Errorf("plugin/%s is missing, so there is nothing to auto-load: %v", must, err)
		}
	}
}
