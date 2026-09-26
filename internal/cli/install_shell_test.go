package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallShell_DryRun(t *testing.T) {
	var out bytes.Buffer
	if err := InstallShell(&out, "/tmp/fake/bin", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/tmp/fake/bin") {
		t.Errorf("dry-run output should mention the dir, got: %s", out.String())
	}
}

func TestInstallShell_WritesExport(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")

	// Override shellRC by writing to a temp file via env manipulation;
	// instead, call the underlying write logic directly.
	f, err := os.Create(rc)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	binDir := filepath.Join(dir, "bin")

	var out bytes.Buffer
	if err := writeExportToFile(rc, &out, binDir); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), binDir) {
		t.Errorf("rc file should contain binDir %s, got:\n%s", binDir, data)
	}
}

func TestInstallShell_Idempotent(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	binDir := filepath.Join(dir, "bin")

	var out bytes.Buffer
	// Write once
	if err := writeExportToFile(rc, &out, binDir); err != nil {
		t.Fatal(err)
	}
	// Write again
	if err := writeExportToFile(rc, &out, binDir); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(rc)
	if count := strings.Count(string(data), binDir); count != 1 {
		t.Errorf("rc file should contain binDir exactly once, got %d times", count)
	}
}
