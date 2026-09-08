package handler

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

func baseDeps(t *testing.T, out *bytes.Buffer) Deps {
	t.Helper()
	return Deps{
		Cfg:      config.Default(),
		Env:      func(string) string { return "" },
		CacheKey: func(p, m string) (string, error) { return "k", nil },
		CacheGet: func(string) (digest.FileMap, bool) { return digest.FileMap{}, false },
		CachePut: func(string, digest.FileMap) error { return nil },
		Record:   func(metrics.Entry) {},
		Logf:     func(string, ...any) {},
		Now:      func() time.Time { return time.Unix(0, 0) },
		Stdout:   out,
	}
}

func readInput(t *testing.T, path string, offset, limit int) hookio.Input {
	t.Helper()
	js := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Read","tool_input":` +
		`{"file_path":"` + path + `","offset":` + strconv.Itoa(offset) + `,"limit":` + strconv.Itoa(limit) + `}}`)
	in, err := hookio.Parse(bytes.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestReadHook_SmallFile_Allows(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "small.go")
	os.WriteFile(f, []byte("package main\n"), 0o644)

	var out bytes.Buffer
	d := baseDeps(t, &out)
	if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("small file should allow (no output), got %q", out.String())
	}
}

func TestReadHook_LargeFile_DeniesWithDigest(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)

	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, error) {
		return []byte(`{"summary":"big file","map":[{"lines":"1-5000","kind":"x lines"}]}`), nil
	}
	if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) {
		t.Fatalf("large file should deny, got %q", out.String())
	}
	if !strings.Contains(out.String(), "big file") {
		t.Fatal("deny reason should contain the digest summary")
	}
}

func TestReadHook_WorkerFails_DegradesOpen(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)

	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, error) {
		return nil, errors.New("boom")
	}
	if err := ReadHook(context.Background(), readInput(t, f, 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("worker failure must degrade open (no output), got %q", out.String())
	}
}

func TestReadHook_OffsetPresent_Allows(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.go")
	os.WriteFile(f, []byte(strings.Repeat("x\n", 5000)), 0o644)
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Summarize = func(_ context.Context, _ worker.Request) ([]byte, error) {
		t.Fatal("summarize must not be called when offset/limit present")
		return nil, nil
	}
	if err := ReadHook(context.Background(), readInput(t, f, 10, 50), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("targeted read should allow")
	}
}

func TestReadHook_RecursionGuard_Allows(t *testing.T) {
	var out bytes.Buffer
	d := baseDeps(t, &out)
	d.Env = func(k string) string {
		if k == "SKIM_ACTIVE" {
			return "1"
		}
		return ""
	}
	if err := ReadHook(context.Background(), readInput(t, "/whatever", 0, 0), d); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatal("recursion guard should allow")
	}
}
