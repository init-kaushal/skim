package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// InstallShell writes a PATH export line for the skim launcher to the user's
// shell rc file, so `skim doctor` / `skim stats` work from the terminal without
// a full plugin re-install. Passing dryRun=true prints what would be written
// without touching any files.
func InstallShell(w io.Writer, selfDir string, dryRun bool) error {
	if dryRun {
		export := fmt.Sprintf(`export PATH="%s:$PATH"`, selfDir)
		fmt.Fprintf(w, "Would add to your shell rc file:\n\n  %s\n\nRun without --dry-run to apply.\n", export)
		return nil
	}

	if isOnPath(selfDir) {
		fmt.Fprintf(w, "skim: %s is already on PATH — nothing to do.\n", selfDir)
		return nil
	}

	rc, err := shellRC()
	if err != nil {
		return fmt.Errorf("could not determine shell rc file: %w", err)
	}

	return writeExportToFile(rc, w, selfDir)
}

// writeExportToFile appends an export line for dir to rcPath, idempotently.
// Extracted so tests can supply an arbitrary rc path.
func writeExportToFile(rcPath string, w io.Writer, dir string) error {
	if alreadyInFile(rcPath, dir) {
		fmt.Fprintf(w, "skim: %s is already in %s — nothing to do.\n", dir, rcPath)
		return nil
	}

	export := fmt.Sprintf(`export PATH="%s:$PATH"`, dir)
	line := "\n# Added by skim install-shell\n" + export + "\n"

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("could not write to %s: %w", rcPath, err)
	}
	defer f.Close()
	if _, err := fmt.Fprint(f, line); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	fmt.Fprintf(w, "skim: added to %s\n\nRestart your terminal or run:\n\n  source %s\n\nThen `skim doctor` will work from any directory.\n", rcPath, rcPath)
	return nil
}

// shellRC returns the path to the user's primary interactive shell rc file.
func shellRC() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		// macOS bash uses .bash_profile for login shells; .bashrc for interactive.
		// .bash_profile is the safer default on macOS.
		if _, err := os.Stat(filepath.Join(home, ".bashrc")); err == nil {
			return filepath.Join(home, ".bashrc"), nil
		}
		return filepath.Join(home, ".bash_profile"), nil
	case "fish":
		return filepath.Join(home, ".config/fish/config.fish"), nil
	default:
		return filepath.Join(home, ".profile"), nil
	}
}

func isOnPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}

func alreadyInFile(path, dir string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), dir)
}
