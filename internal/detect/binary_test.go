package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLooksBinary_ByExtension(t *testing.T) {
	dir := t.TempDir()
	// Contents are deliberately plain text: the extension alone must decide,
	// so a PNG is never shipped to the worker even if it somehow sniffs clean.
	for _, name := range []string{"a.png", "b.JPG", "c.pdf", "d.ipynb", "e.woff2", "f.MP4", "g.svg"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("not really binary"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := LooksBinary(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !got {
			t.Errorf("LooksBinary(%s) = false, want true (extension allowlist)", name)
		}
	}
}

func TestLooksBinary_SniffsNulByte(t *testing.T) {
	dir := t.TempDir()

	textFile := filepath.Join(dir, "big.go")
	if err := os.WriteFile(textFile, []byte(strings.Repeat("package main\n", 500)), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LooksBinary(textFile)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("a large Go file must not be treated as binary")
	}

	blobFile := filepath.Join(dir, "mystery")
	if err := os.WriteFile(blobFile, []byte("ELF\x00\x01\x02 padding"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = LooksBinary(blobFile)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("a NUL byte in the first 512 bytes should read as binary")
	}
}

// TestLooksBinary_NulPastSniffWindow documents the heuristic's deliberate
// limit: only the first 512 bytes are inspected, so a NUL further in is missed
// and the file takes the normal interception path.
func TestLooksBinary_NulPastSniffWindow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "late")
	body := append([]byte(strings.Repeat("a", sniffBytes+10)), 0x00)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LooksBinary(p)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("a NUL past the sniff window should not be detected")
	}
}

func TestLooksBinary_EmptyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LooksBinary(p)
	if err != nil {
		t.Fatalf("empty file should not error: %v", err)
	}
	if got {
		t.Error("an empty file is not binary")
	}
}

// TestLooksBinary_MissingFile checks the contract callers rely on: an error
// reports "could not tell" and must come back with false, never a positive
// detection that would let an unreadable path skip interception.
func TestLooksBinary_MissingFile(t *testing.T) {
	got, err := LooksBinary(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if got {
		t.Error("a sniff error must report false, not binary")
	}
}
