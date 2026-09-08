package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaushal/skim/internal/digest"
)

func TestKey_ChangesWithMtimeAndModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKIM_HOME", dir)
	f := filepath.Join(dir, "src.go")
	os.WriteFile(f, []byte("a\nb\n"), 0o644)

	k1, err := Key(f, "haiku")
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := Key(f, "sonnet")
	if k1 == k2 {
		t.Fatal("key should depend on model")
	}

	future := time.Now().Add(2 * time.Hour)
	os.Chtimes(f, future, future)
	k3, _ := Key(f, "haiku")
	if k1 == k3 {
		t.Fatal("key should depend on mtime")
	}
}

func TestPutGet_RoundTrip(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	fm := digest.FileMap{Summary: "s", Map: []digest.MapEntry{{Lines: "1-2", Kind: "x"}}}
	if err := Put("k1", fm); err != nil {
		t.Fatal(err)
	}
	got, ok := Get("k1")
	if !ok || got.Summary != "s" {
		t.Fatalf("Get = %+v, ok=%v", got, ok)
	}
	if _, ok := Get("missing"); ok {
		t.Fatal("missing key should be a miss")
	}
}

func TestSweep_RemovesOldEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SKIM_HOME", home)
	Put("fresh", digest.FileMap{Summary: "s", Map: []digest.MapEntry{{Lines: "1", Kind: "x"}}})
	Put("stale", digest.FileMap{Summary: "s", Map: []digest.MapEntry{{Lines: "1", Kind: "x"}}})
	old := time.Now().Add(-8 * 24 * time.Hour)
	os.Chtimes(filepath.Join(home, "cache", "stale.json"), old, old)

	removed, err := Sweep(7 * 24 * time.Hour)
	if err != nil || removed != 1 {
		t.Fatalf("removed=%d err=%v, want 1 / nil", removed, err)
	}
	if _, ok := Get("fresh"); !ok {
		t.Fatal("fresh entry should survive")
	}
}
