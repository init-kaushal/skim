package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCat_DumpsWholeFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("line1\nline2\n"), 0o644)
	var out bytes.Buffer
	if err := Cat(&out, p); err != nil {
		t.Fatal(err)
	}
	if out.String() != "line1\nline2\n" {
		t.Fatalf("got %q", out.String())
	}
}

func TestCat_MissingFile_Errors(t *testing.T) {
	if err := Cat(&bytes.Buffer{}, "/no/such/file"); err == nil {
		t.Fatal("expected error")
	}
}
